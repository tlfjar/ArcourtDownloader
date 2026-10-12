package arcourt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func fakeBrowserFetcher(t *testing.T, launch browserLauncher) *BrowserFetcher {
	t.Helper()
	r := testResolver("linux", nil, map[string]os.FileMode{"/browser": 0755}, nil)
	f, err := newBrowserFetcher(BrowserFetcherConfig{
		ExecutablePath: "/browser", Headless: true, PageTimeout: time.Second,
		CaseURLTemplate: "https://fixture.invalid/case/{case_number}",
	}, r, launch)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	return f
}

func requireProfileRemoved(t *testing.T, profile string) {
	t.Helper()
	if profile == "" {
		t.Fatal("launcher did not receive a profile")
	}
	if _, err := os.Stat(profile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("profile still exists: %v", err)
	}
}

func TestBrowserSessionCleanup(t *testing.T) {
	for _, mode := range []string{"success", "launch failure", "page failure", "caller cancellation", "deadline", "app exit"} {
		t.Run(mode, func(t *testing.T) {
			var profile string
			var stops atomic.Int32
			f := fakeBrowserFetcher(t, func(ctx context.Context, info BrowserInfo, headless bool, dir string) (context.Context, func(), error) {
				profile = dir
				if info.ExecutablePath != "/browser" || !headless {
					t.Error("launch configuration lost")
				}
				if stat, err := os.Stat(dir); err != nil || !stat.IsDir() {
					t.Fatal("missing isolated profile")
				}
				stop := func() {
					if _, err := os.Stat(dir); err != nil {
						t.Error("profile removed before browser stopped")
					}
					stops.Add(1)
				}
				if mode == "launch failure" {
					return ctx, stop, errors.New("stderr cookie=SECRET https://signed.invalid/?token=SECRET")
				}
				return ctx, stop, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			timeout := 5 * time.Second
			if mode == "deadline" {
				timeout = 50 * time.Millisecond
			}
			var cleanup func()
			var err error
			if mode == "page failure" {
				_, cleanup, err = f.openCasePageOnce(ctx, "60CV-2026-1", timeout)
			} else {
				_, cleanup, err = f.openBrowser(ctx, timeout)
			}
			if mode == "launch failure" || mode == "page failure" {
				if err == nil {
					t.Fatal("expected failure")
				}
				if mode == "launch failure" {
					for _, want := range []string{"/browser", "headless=true", "sandbox=enabled", "TLS=verified", "check executable"} {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("diagnostic missing %q: %v", want, err)
						}
					}
					if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), profile) {
						t.Fatal("sensitive startup diagnostics")
					}
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "success":
					cleanup()
					cleanup()
				case "caller cancellation":
					cancel()
				case "app exit":
					if err := f.Close(); err != nil {
						t.Fatal(err)
					}
				}
				// Wait for automatic cleanup without explicitly calling it first.
				done := make(chan struct{})
				go func() { f.active.Wait(); close(done) }()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Fatal("cleanup did not finish")
				}
				cleanup()
			}
			requireProfileRemoved(t, profile)
			if stops.Load() != 1 {
				t.Fatalf("stop called %d times", stops.Load())
			}
		})
	}
}

func TestCancelAtLaunchBoundary(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "caller", true: "app exit"}[shutdown], func(t *testing.T) {
			entered := make(chan string, 1)
			var stopped atomic.Bool
			f := fakeBrowserFetcher(t, func(ctx context.Context, _ BrowserInfo, _ bool, profile string) (context.Context, func(), error) {
				entered <- profile
				<-ctx.Done()
				return ctx, func() { stopped.Store(true) }, errors.New("browser stdout closed during launch")
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, _, err := f.openBrowser(ctx, 5*time.Second); result <- err }()
			profile := <-entered
			if shutdown {
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("got %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("launch cancellation blocked")
			}
			if !stopped.Load() {
				t.Fatal("launcher not stopped")
			}
			requireProfileRemoved(t, profile)
		})
	}
}

func TestBrowserDoesNotLaunchAfterCancellationOrClose(t *testing.T) {
	f := fakeBrowserFetcher(t, func(context.Context, BrowserInfo, bool, string) (context.Context, func(), error) {
		t.Fatal("unexpected launch")
		return nil, nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := f.openBrowser(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.openBrowser(context.Background(), time.Second); !errors.Is(err, ErrBrowserFetcherClosed) {
		t.Fatalf("got %v", err)
	}
}

func TestNewBrowserFetcherInvalidOverride(t *testing.T) {
	_, err := NewBrowserFetcher(BrowserFetcherConfig{CaseURLTemplate: "https://fixture.invalid/{case_number}", ExecutablePath: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("got %v", err)
	}
}

func TestBrowserAllocatorFlags(t *testing.T) {
	for _, headless := range []bool{true, false} {
		t.Run(map[bool]string{true: "headless", false: "headful"}[headless], func(t *testing.T) {
			root := t.TempDir()
			profile := filepath.Join(root, "profile with spaces")
			executable := filepath.Join(root, "missing browser.exe")
			opts := browserAllocatorOptions(BrowserInfo{ExecutablePath: executable}, headless, profile)
			var args []string
			opts = append(opts, chromedp.ModifyCmdFunc(func(cmd *exec.Cmd) {
				args = append([]string(nil), cmd.Args...)
				if cmd.Path != executable {
					t.Errorf("executable = %q", cmd.Path)
				}
			}))
			allocCtx, stopAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
			defer stopAlloc()
			ctx, stopBrowser := chromedp.NewContext(allocCtx)
			defer stopBrowser()
			// The executable deliberately does not exist: inspect the actual command
			// construction without launching a browser or relying on its installation.
			if err := chromedp.Run(ctx); err == nil {
				t.Fatal("expected missing executable")
			}
			flags := make(map[string]bool)
			for _, arg := range args {
				flags[arg] = true
			}
			if flags["--headless"] != headless || !flags["--user-data-dir="+profile] || !flags["--remote-debugging-port=0"] {
				t.Fatalf("incorrect launch arguments: %v", args)
			}
			for _, unsafe := range []string{"--no-sandbox", "--ignore-certificate-errors", "--disable-web-security"} {
				if flags[unsafe] {
					t.Errorf("unexpected flag %s", unsafe)
				}
			}
		})
	}
}

func TestEdgeCompatibilityLayerScope(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows Edge compatibility layer behavior")
	}
	t.Setenv("__COMPAT_LAYER", "RunAsInvoker")
	for _, tc := range []struct {
		path string
		want bool
	}{
		{`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, true},
		{`C:\Program Files (x86)\Microsoft\Edge\Application\MSEDGE.EXE`, true},
		{`C:\Program Files\Google\Chrome\Application\chrome.exe`, false},
	} {
		if got := edgeNeedsCompatLayerClear(tc.path); got != tc.want {
			t.Errorf("edgeNeedsCompatLayerClear(%q) = %t, want %t", tc.path, got, tc.want)
		}
	}
	t.Setenv("__COMPAT_LAYER", "")
	if edgeNeedsCompatLayerClear(`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`) {
		t.Fatal("no compatibility layer needs to be cleared")
	}
}
