package arcourt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// ErrBrowserFetcherClosed is returned after application shutdown begins.
var ErrBrowserFetcherClosed = errors.New("browser fetcher is closed")

// A launcher must return a stop function even on failure if it acquired resources.
// stop waits for the application's browser process to exit before returning.
type browserLauncher func(context.Context, BrowserInfo, bool, string) (context.Context, func(), error)

func newBrowserFetcher(cfg BrowserFetcherConfig, resolver browserResolver, launch browserLauncher) (*BrowserFetcher, error) {
	info, err := resolver.resolve(cfg.ExecutablePath)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &BrowserFetcher{cfg: cfg, browser: info, launch: launch, shutdown: ctx, cancelShutdown: cancel}, nil
}

// BrowserInfo returns the selected executable without launching it.
func (f *BrowserFetcher) BrowserInfo() BrowserInfo { return f.browser }

// Track the entire operation so Close also cancels/waits for HTTP and writers
// after the short-lived browser discovery session has closed.
func (f *BrowserFetcher) beginOperation(parent context.Context) (context.Context, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, nil, ErrBrowserFetcherClosed
	}
	f.active.Add(1)
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(f.shutdown, cancel)
	return ctx, func() { stop(); cancel(); f.active.Done() }, nil
}

// Close cancels and waits for all application-owned sessions and profile cleanup.
// Call it during application shutdown, before exiting the process. It is idempotent.
func (f *BrowserFetcher) Close() error {
	f.mu.Lock()
	f.closed = true
	f.cancelShutdown()
	f.mu.Unlock()
	f.active.Wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cleanupErr
}

// browserStartupError deliberately excludes raw browser stderr, which can include
// profile contents or URLs. Unwrap retains cancellation/deadline classification.
type browserStartupError struct {
	message string
	cause   error
}

func (e *browserStartupError) Error() string { return e.message }
func (e *browserStartupError) Unwrap() error { return e.cause }

func (f *BrowserFetcher) startupError(stage string, timeout time.Duration, cause error) error {
	reason := "check executable permissions, browser installation, and writable system temporary storage"
	if errors.Is(cause, context.Canceled) {
		reason = "operation canceled"
	} else if errors.Is(cause, context.DeadlineExceeded) {
		reason = "startup deadline exceeded; check the browser installation or increase PageTimeout"
	}
	return &browserStartupError{
		message: fmt.Sprintf("browser %s failed: %s (%q, source=%s, headless=%t, timeout=%s, sandbox=enabled, TLS=verified, profile=isolated temporary); %s",
			stage, f.browser.Name, f.browser.ExecutablePath, f.browser.Source, f.cfg.Headless, timeout, reason),
		cause: cause,
	}
}

func (f *BrowserFetcher) openBrowser(parent context.Context, timeout time.Duration) (context.Context, func(), error) {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil, nil, ErrBrowserFetcherClosed
	}
	f.active.Add(1)
	f.mu.Unlock()

	ctx, cancel := context.WithTimeout(parent, timeout)
	stopShutdown := context.AfterFunc(f.shutdown, cancel)
	var stopBrowser func()
	var profile string
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			defer f.active.Done()
			stopShutdown()
			cancel()
			if stopBrowser != nil {
				stopBrowser()
			}
			if profile != "" {
				if err := removeBrowserProfile(profile); err != nil {
					f.mu.Lock()
					f.cleanupErr = errors.Join(f.cleanupErr, errors.New("could not remove an application temporary browser profile; check temporary-directory permissions and file locks"))
					f.mu.Unlock()
				}
			}
		})
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, err
	}
	var err error
	profile, err = os.MkdirTemp("", "arcourt-browser-")
	if err != nil {
		cleanup()
		return nil, nil, f.startupError("profile creation", timeout, err)
	}
	browserCtx, stop, err := f.launch(ctx, f.browser, f.cfg.Headless, profile)
	stopBrowser = stop
	if ctx.Err() != nil {
		// A canceled process can report EOF or a startup failure instead of the
		// context error. Preserve cancellation/deadline semantics for callers.
		err = ctx.Err()
	}
	if err != nil {
		cleanup()
		return nil, nil, f.startupError("launch", timeout, err)
	}
	// Also clean up if the caller cancels or the browser exits before the operation
	// returns. Start only after launch has finished publishing its stop function.
	go func() {
		select {
		case <-ctx.Done():
		case <-browserCtx.Done():
		}
		cleanup()
	}()
	return browserCtx, cleanup, nil
}

func removeBrowserProfile(profile string) error {
	// Only receives the fresh directory returned by MkdirTemp above. Windows
	// browser subprocesses can briefly retain file handles after the parent exits.
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		if err = os.RemoveAll(profile); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

func browserAllocatorOptions(info BrowserInfo, headless bool, profile string) []chromedp.ExecAllocatorOption {
	// Use only the required flags rather than defaults that disable site isolation
	// and phishing protection. Explicit false also prevents chromedp's root-user
	// fallback from silently disabling the sandbox on Linux.
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(info.ExecutablePath),
		chromedp.UserDataDir(profile),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", headless),
		chromedp.Flag("no-sandbox", false),
		chromedp.Flag("remote-debugging-address", "127.0.0.1"),
		chromedp.Flag("remote-debugging-port", "0"),
	}
	if edgeNeedsCompatLayerClear(info.ExecutablePath) {
		// Edge may relaunch itself when it inherits an application compatibility
		// layer such as RunAsInvoker. chromedp needs the original process to stay
		// attached while it reads the DevTools endpoint. Clear the layer only in
		// the Edge child; leave the application's environment unchanged.
		opts = append(opts, chromedp.Env("__COMPAT_LAYER="))
	}
	return opts
}

func edgeNeedsCompatLayerClear(executablePath string) bool {
	return runtime.GOOS == "windows" && strings.EqualFold(filepath.Base(executablePath), "msedge.exe") && os.Getenv("__COMPAT_LAYER") != ""
}

func launchChromedp(ctx context.Context, info BrowserInfo, headless bool, profile string) (context.Context, func(), error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, browserAllocatorOptions(info, headless, profile)...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	stop := func() {
		cancelBrowser()
		cancelAlloc()
	}
	// Separate process startup from navigation to keep startup diagnostics free
	// of court URLs, cookies, and page data.
	err := chromedp.Run(browserCtx)
	return browserCtx, stop, err
}
