package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
)

type Service interface {
	Preview(context.Context, string) (*arcourt.CasePreview, error)
	Download(context.Context, arcourt.DownloadRequest, chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error)
}
type Factory func(Preferences) (Service, func() error, string, error)

func BrowserService(p Preferences) (Service, func() error, string, error) {
	if p.CaseURLTemplate == "" {
		return nil, nil, "", errors.New("Set the case page template in Settings before previewing.")
	}
	f, err := arcourt.NewBrowserFetcher(arcourt.BrowserFetcherConfig{CaseURLTemplate: p.CaseURLTemplate, ExecutablePath: p.BrowserOverride, Headless: true})
	if err != nil {
		return nil, nil, "", errors.New("Browser unavailable. Install Edge or Chrome, or correct the browser override in Settings.")
	}
	s, err := arcourt.NewDownloadService(f)
	if err != nil {
		_ = f.Close()
		return nil, nil, "", err
	}
	b := f.BrowserInfo()
	return s, f.Close, fmt.Sprintf("%s · %s · %s", b.Name, b.Source, b.ExecutablePath), nil
}

// Controller owns UI state. Jobs retain the busy slot until progress is drained
// and browser Close has returned. Generation checks reject obsolete commands and
// late results; revisions let the webview reject out-of-order snapshots.
type Controller struct {
	mu       sync.Mutex
	state    State
	preview  *arcourt.CasePreview
	selected map[string]bool
	all      bool
	factory  Factory
	store    PreferenceStore
	secrets  SecretStore
	cancel   context.CancelFunc
	done     chan struct{}
	closed   bool
}

func New(factory Factory, store PreferenceStore) *Controller {
	return NewWithSecrets(factory, store, DefaultSecretStore())
}

func NewWithSecrets(factory Factory, store PreferenceStore, secrets SecretStore) *Controller {
	p, err := store.Load()
	c := &Controller{factory: factory, store: store, secrets: secrets, selected: map[string]bool{}, state: State{Phase: "idle", Preferences: p, Revision: 1}}
	if err != nil {
		c.state.Diagnostic = "Settings could not be read. Save Settings to repair the local preferences file."
	}
	if err := validatePreferences(p); err != nil {
		c.state.Preferences = Preferences{}
		c.state.Diagnostic = "Stored settings are invalid. Configure and save Settings again."
	}
	c.refreshNamingCredential()
	c.detectBrowser()
	return c
}

// Caller holds mu. A credential is only read into server memory and its value
// is never copied into State or Preferences.
func (c *Controller) refreshNamingCredential() {
	c.state.NamingCredentialStatus = "missing"
	provider := c.state.Preferences.NamingProvider
	if provider == "" {
		return
	}
	if c.secrets == nil {
		c.state.NamingCredentialStatus = "unavailable"
		return
	}
	configured, err := c.secrets.Status(provider)
	switch {
	case err == nil && configured:
		c.state.NamingCredentialStatus = "configured"
	case err == nil || errors.Is(err, ErrCredentialMissing):
		c.state.NamingCredentialStatus = "missing"
	default:
		c.state.NamingCredentialStatus = "unavailable"
	}
}

func (c *Controller) detectBrowser() {
	// Detection uses the same resolver without launching a browser. A placeholder
	// is used only to satisfy construction and is never navigated.
	p := c.state.Preferences
	if p.CaseURLTemplate == "" {
		p.CaseURLTemplate = "https://example.invalid/{case_number}"
	}
	_, closeService, browser, err := c.factory(p)
	if err != nil {
		c.state.Browser = "No browser detected. Install Edge or Chrome, or correct the override."
		return
	}
	c.state.Browser = browser
	if err := closeService(); err != nil {
		c.state.Diagnostic = "Browser cleanup failed. Check temporary-folder permissions and file locks."
	}
}

func (c *Controller) Snapshot() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Return an independent snapshot: Wails serializes after releasing this lock.
	b, _ := json.Marshal(c.state)
	var s State
	_ = json.Unmarshal(b, &s)
	return s
}

func (c *Controller) touch() { c.state.Revision++ }

func (c *Controller) invalidate() {
	c.state.Generation++
	c.preview, c.state.Preview, c.state.Result = nil, nil, nil
	c.selected = map[string]bool{}
	c.all, c.state.Verified, c.state.Selected = false, false, 0
	c.state.All = false
	c.state.Progress, c.state.Bytes = arcourt.DownloadCounts{}, 0
	c.state.Message, c.state.Phase, c.state.NamingNotice = "", "idle", ""
}

func (c *Controller) SetCase(number string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("Application is closing.")
	}
	number = strings.ToUpper(strings.TrimSpace(number))
	if number == c.state.CaseNumber {
		return nil
	}
	if c.cancel != nil {
		c.cancel()
		c.state.Canceling = true
	}
	c.invalidate()
	c.state.CaseNumber = number
	c.touch()
	return nil
}

func (c *Controller) SavePreferences(p Preferences) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Busy || c.closed {
		return errors.New("Wait for the current operation before changing settings.")
	}
	p.CaseURLTemplate, p.BrowserOverride, p.OutputDirectory = strings.TrimSpace(p.CaseURLTemplate), strings.TrimSpace(p.BrowserOverride), strings.TrimSpace(p.OutputDirectory)
	p.NamingProvider, p.NamingModel, p.NamingConsentRecipient = strings.TrimSpace(p.NamingProvider), strings.TrimSpace(p.NamingModel), strings.TrimSpace(p.NamingConsentRecipient)
	if err := validatePreferences(p); err != nil {
		return err
	}
	if err := c.store.Save(p); err != nil {
		return errors.New("Settings could not be saved. Check your user configuration folder permissions.")
	}
	if p.CaseURLTemplate != c.state.Preferences.CaseURLTemplate || p.BrowserOverride != c.state.Preferences.BrowserOverride {
		c.invalidate()
	}
	c.state.Preferences = p
	c.refreshNamingCredential()
	c.state.NamingNotice = ""
	c.state.Diagnostic = ""
	c.detectBrowser()
	c.touch()
	return nil
}

// SaveNamingCredential is the only Wails-facing secret write path. Keys remain
// in Windows Credential Manager, never in settings JSON or a state snapshot.
func (c *Controller) SaveNamingCredential(provider, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Busy || c.closed {
		return errors.New("Wait for the current operation before changing credentials.")
	}
	if provider == "" || provider != c.state.Preferences.NamingProvider {
		return errors.New("Save the selected AI provider in Settings first.")
	}
	key, err := validateCredential(provider, key)
	if err != nil {
		return err
	}
	if c.secrets == nil || c.secrets.Write(provider, key) != nil {
		return errors.New("Windows Credential Manager could not save the API key.")
	}
	c.refreshNamingCredential()
	c.touch()
	return nil
}

func (c *Controller) RemoveNamingCredential(provider string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Busy || c.closed {
		return errors.New("Wait for the current operation before changing credentials.")
	}
	if provider == "" || provider != c.state.Preferences.NamingProvider {
		return errors.New("Save the selected AI provider in Settings first.")
	}
	if c.secrets == nil || c.secrets.Delete(provider) != nil {
		return errors.New("Windows Credential Manager could not remove the API key.")
	}
	c.refreshNamingCredential()
	c.touch()
	return nil
}

func (c *Controller) SetSelection(generation uint64, ids []string, verified, all bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Busy || c.closed || generation != c.state.Generation || c.preview == nil {
		return errors.New("Selection expired or an operation is active. Preview the case again after it finishes.")
	}
	known := map[string]bool{}
	for _, d := range c.state.Preview.Documents {
		known[d.ID] = true
	}
	selected := map[string]bool{}
	for _, id := range ids {
		if !known[id] {
			return errors.New("A selected document is not in this preview. Preview again.")
		}
		selected[id] = true
	}
	c.selected, c.all, c.state.Verified = selected, all && len(selected) == len(known), verified
	c.state.All = c.all
	for i := range c.state.Preview.Documents {
		d := &c.state.Preview.Documents[i]
		d.Selected = selected[d.ID]
	}
	c.state.Selected = len(selected)
	c.touch()
	return nil
}

func (c *Controller) begin(phase string) (context.Context, uint64, Preferences, error) {
	if c.state.Busy || c.closed {
		return nil, 0, Preferences{}, errors.New("Wait for the active operation to finish or cancel it.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	c.cancel, c.done = cancel, make(chan struct{})
	c.state.Busy, c.state.Canceling = true, false
	c.state.Phase, c.state.Message, c.state.Diagnostic = phase, "", ""
	c.state.Progress, c.state.Bytes, c.state.Result = arcourt.DownloadCounts{}, 0, nil
	c.state.NamingNotice = ""
	c.touch()
	return ctx, c.state.Generation, c.state.Preferences, nil
}

func (c *Controller) PreviewCase() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Busy || c.closed {
		return errors.New("Wait for the active operation to finish or cancel it.")
	}
	c.invalidate()
	ctx, gen, prefs, err := c.begin("previewing")
	if err != nil {
		return err
	}
	number := c.state.CaseNumber
	go c.run(ctx, gen, prefs, func(s Service) (*arcourt.CasePreview, *arcourt.LocalDownloadResult, error) {
		p, err := s.Preview(ctx, number)
		if err == nil && (p == nil || !strings.EqualFold(strings.TrimSpace(p.CaseInfo.CaseNumber), number)) {
			return nil, nil, arcourt.ErrCaseNumberMismatch
		}
		return p, nil, err
	}, nil)
	return nil
}

func (c *Controller) Download(generation uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.state.Generation || c.preview == nil || !c.state.Verified {
		return errors.New("Preview and verify the case header before downloading.")
	}
	if len(c.selected) == 0 {
		return errors.New("Select at least one document before downloading.")
	}
	if c.state.Preferences.OutputDirectory == "" {
		return errors.New("Choose an output folder before downloading.")
	}
	var selection []arcourt.DocketEntry
	for _, d := range c.preview.DocketEntries {
		if c.selected[arcourt.DocumentID(d.SourceURL)] {
			selection = append(selection, d)
		}
	}
	ctx, gen, prefs, err := c.begin("downloading")
	if err != nil {
		return err
	}
	c.state.Progress.Selected = len(c.selected)
	req := arcourt.DownloadRequest{CaseNumber: c.state.CaseNumber, Selection: selection, OutputDirectory: prefs.OutputDirectory}
	if prefs.NamingEnabled {
		switch {
		case prefs.NamingConsentRecipient != namingRecipient(prefs.NamingProvider):
			c.state.NamingNotice = "AI naming skipped: review and consent to the selected provider in Settings. Standard filenames will be used."
		case c.secrets == nil:
			c.state.NamingNotice = "AI naming skipped: secure credential storage is unavailable. Standard filenames will be used."
		default:
			key, keyErr := c.secrets.Read(prefs.NamingProvider)
			if keyErr != nil || key == "" {
				if errors.Is(keyErr, ErrCredentialMissing) || key == "" && keyErr == nil {
					c.state.NamingCredentialStatus = "missing"
				} else {
					c.state.NamingCredentialStatus = "unavailable"
				}
				c.state.NamingNotice = "AI naming skipped: provider API key is missing or unavailable. Standard filenames will be used."
			} else {
				c.state.NamingCredentialStatus = "configured"
				req.Naming = &arcourt.NamingRequest{Provider: prefs.NamingProvider, Model: prefs.NamingModel, APIKey: key}
				c.state.NamingNotice = "AI naming enabled for newly saved documents. Per-document results appear below."
			}
		}
	}
	c.touch()
	var allCoverage *arcourt.DiscoveryCoverage
	if c.all {
		coverage := c.preview.Discovery
		allCoverage = &coverage
	}
	go c.run(ctx, gen, prefs, func(s Service) (*arcourt.CasePreview, *arcourt.LocalDownloadResult, error) {
		events := make(chan arcourt.DownloadEvent, 32)
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			for e := range events {
				c.progress(gen, e)
			}
		}()
		r, err := s.Download(ctx, req, events)
		close(events)
		<-drained
		return nil, r, err
	}, allCoverage)
	return nil
}

func (c *Controller) progress(gen uint64, e arcourt.DownloadEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.state.Generation || !c.state.Busy || c.state.Phase != "downloading" {
		return
	}
	if e.Kind == arcourt.DownloadTransferring {
		c.state.Bytes = e.Bytes
	} else {
		c.state.Progress = e.Counts
	}
	c.touch()
}

func (c *Controller) run(ctx context.Context, gen uint64, prefs Preferences, work func(Service) (*arcourt.CasePreview, *arcourt.LocalDownloadResult, error), allCoverage *arcourt.DiscoveryCoverage) {
	s, closeService, browser, err := c.factory(prefs)
	setupFailed := err != nil
	var p *arcourt.CasePreview
	var r *arcourt.LocalDownloadResult
	var cleanupErr error
	if err == nil {
		p, r, err = work(s)
		cleanupErr = closeService()
	}
	if ctx.Err() != nil {
		err = errors.Join(err, ctx.Err())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancel()
	c.cancel = nil
	c.state.Busy, c.state.Canceling = false, false
	if gen == c.state.Generation {
		if browser != "" {
			c.state.Browser = browser
		}
		switch {
		case r != nil:
			c.state.Result = resultView(r)
			c.state.Progress = r.Counts
			c.state.Phase, c.state.Message = outcome(r, err, allCoverage)
		case err != nil:
			c.state.Phase, c.state.Message = "error", operationError(err)
			if setupFailed {
				c.state.Message = "Browser setup failed. Set a case page template, install Edge or Chrome, and check the browser override in Settings."
			}
			if errors.Is(err, context.Canceled) {
				c.state.Phase = "canceled"
			}
		case p != nil:
			c.preview, c.state.Preview, c.state.Phase = p, previewView(p), "ready"
		}
	}
	// Cleanup failures remain relevant even when a case was changed mid-operation.
	if cleanupErr != nil {
		c.state.Diagnostic = "Browser cleanup failed. Check temporary-folder permissions and file locks before closing."
		c.state.Phase, c.state.Message = "error", c.state.Diagnostic
	}
	c.touch()
	close(c.done)
}

func (c *Controller) Cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
		c.state.Canceling = true
		c.state.Message = "Canceling; waiting for browser and file cleanup…"
		c.touch()
	}
}

// Close is called off the native UI thread. The window stays responsive while
// the active job unwinds; callers wait before permitting process exit.
func (c *Controller) Close() {
	c.mu.Lock()
	c.closed, c.state.Closing = true, true
	if c.cancel != nil {
		c.cancel()
		c.state.Canceling = true
	}
	done := c.done
	c.touch()
	c.mu.Unlock()
	if done != nil {
		<-done
	}
}
