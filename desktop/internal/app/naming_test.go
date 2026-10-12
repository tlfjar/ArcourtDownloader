package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
)

type fakeSecretStore struct {
	keys                             map[string]string
	reads, statuses, writes, deletes int
	broken                           bool
}

func (s *fakeSecretStore) Status(provider string) (bool, error) {
	s.statuses++
	if s.broken {
		return false, errCredentialUnavailable
	}
	return s.keys[provider] != "", nil
}

func (s *fakeSecretStore) Read(provider string) (string, error) {
	s.reads++
	if s.broken {
		return "", errCredentialUnavailable
	}
	if key := s.keys[provider]; key != "" {
		return key, nil
	}
	return "", ErrCredentialMissing
}
func (s *fakeSecretStore) Write(provider, key string) error {
	s.writes++
	if s.broken {
		return errCredentialUnavailable
	}
	if s.keys == nil {
		s.keys = map[string]string{}
	}
	s.keys[provider] = key
	return nil
}
func (s *fakeSecretStore) Delete(provider string) error {
	s.deletes++
	if s.broken {
		return errCredentialUnavailable
	}
	delete(s.keys, provider)
	return nil
}

func namingController(t *testing.T, p Preferences, secrets SecretStore, service Service) *Controller {
	t.Helper()
	store := PreferenceStore{Path: filepath.Join(t.TempDir(), "settings.json")}
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	c := NewWithSecrets(func(Preferences) (Service, func() error, string, error) {
		return service, func() error { return nil }, "Synthetic browser", nil
	}, store, secrets)
	t.Cleanup(c.Close)
	if err := c.SetCase(caseNumber); err != nil {
		t.Fatal(err)
	}
	return c
}

func selectForNaming(t *testing.T, c *Controller) State {
	t.Helper()
	s := ready(t, c)
	if err := c.SetSelection(s.Generation, []string{s.Preview.Documents[0].ID}, true, false); err != nil {
		t.Fatal(err)
	}
	return c.Snapshot()
}

func TestNamingRequestRequiresSavedRecipientConsentAndKey(t *testing.T) {
	for _, tc := range []struct {
		name, consent, key  string
		enabled, wantNaming bool
	}{
		{name: "disabled", consent: namingRecipient("openai"), key: "test-secret", wantNaming: false},
		{name: "no consent", key: "test-secret", enabled: true},
		{name: "no key", consent: namingRecipient("openai"), enabled: true},
		{name: "enabled", consent: namingRecipient("openai"), key: "test-secret", enabled: true, wantNaming: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan arcourt.DownloadRequest, 1)
			secrets := &fakeSecretStore{keys: map[string]string{"openai": tc.key}}
			p := Preferences{OutputDirectory: t.TempDir(), NamingEnabled: tc.enabled, NamingProvider: "openai", NamingModel: "fixture-model", NamingConsentRecipient: tc.consent}
			c := namingController(t, p, secrets, fakeService{download: func(_ context.Context, req arcourt.DownloadRequest, _ chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
				requests <- req
				return &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Succeeded: 1}}, nil
			}})
			beforePreviewReads := secrets.reads
			s := selectForNaming(t, c)
			if secrets.reads != beforePreviewReads {
				t.Fatal("preview read a naming credential")
			}
			if err := c.Download(s.Generation); err != nil {
				t.Fatal(err)
			}
			result := waitIdle(t, c)
			req := <-requests
			if (req.Naming != nil) != tc.wantNaming {
				t.Fatalf("unexpected naming request: %+v", req.Naming)
			}
			if tc.wantNaming && (req.Naming.Provider != "openai" || req.Naming.Model != "fixture-model" || req.Naming.APIKey != tc.key) {
				t.Fatal("download did not receive frozen naming configuration")
			}
			if tc.enabled && !tc.wantNaming && !strings.Contains(result.NamingNotice, "Standard filenames") {
				t.Fatalf("missing fallback notice: %q", result.NamingNotice)
			}
			b, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), tc.key) && tc.key != "" {
				t.Fatal("key leaked into snapshot")
			}
		})
	}
}

func TestNamingCredentialActionsStayOutsidePreferences(t *testing.T) {
	secrets := &fakeSecretStore{}
	c := namingController(t, Preferences{NamingProvider: "anthropic", NamingModel: "fixture-model"}, secrets, fakeService{})
	if err := c.SaveNamingCredential("openai", "secret"); err == nil || secrets.writes != 0 {
		t.Fatal("credential written for unsaved provider")
	}
	if err := c.SaveNamingCredential("anthropic", "line\nbreak"); err == nil || secrets.writes != 0 {
		t.Fatal("control character accepted")
	}
	if err := c.SaveNamingCredential("anthropic", " synthetic-api-secret "); err != nil {
		t.Fatal(err)
	}
	if secrets.keys["anthropic"] != "synthetic-api-secret" || c.Snapshot().NamingCredentialStatus != "configured" {
		t.Fatal("credential not saved")
	}
	b, err := os.ReadFile(c.store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "synthetic-api-secret") {
		t.Fatal("key leaked into preferences")
	}
	snapshot, err := json.Marshal(c.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(snapshot), "synthetic-api-secret") {
		t.Fatal("key leaked into snapshot")
	}
	if err := c.RemoveNamingCredential("anthropic"); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().NamingCredentialStatus != "missing" || secrets.deletes != 1 {
		t.Fatal("key was not removed")
	}
	secrets.broken = true
	if err := c.SaveNamingCredential("anthropic", "another-secret"); err == nil || !strings.Contains(err.Error(), "Credential Manager") {
		t.Fatal("unavailable secure store did not fail closed")
	}
}

func TestNamingSettingsRequireCurrentRecipient(t *testing.T) {
	c := namingController(t, Preferences{}, &fakeSecretStore{}, fakeService{})
	p := Preferences{NamingEnabled: true, NamingProvider: "xai", NamingModel: "grok-test", NamingConsentRecipient: namingRecipient("openai")}
	if err := c.SavePreferences(p); err == nil {
		t.Fatal("stale consent accepted")
	}
	p.NamingConsentRecipient = namingRecipient("xai")
	if err := c.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	p.NamingProvider = "google"
	if err := c.SavePreferences(p); err == nil {
		t.Fatal("consent followed provider change")
	}
	p.NamingConsentRecipient = ""
	if err := c.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Preferences.NamingConsentRecipient != "" {
		t.Fatal("consent was inherited")
	}
}

func TestNamingSettingsRejectModelIDsCoreCannotUse(t *testing.T) {
	c := namingController(t, Preferences{}, &fakeSecretStore{}, fakeService{})
	p := Preferences{NamingEnabled: true, NamingProvider: "openai", NamingModel: "gpt-4o:latest", NamingConsentRecipient: namingRecipient("openai")}
	if err := c.SavePreferences(p); err != nil {
		t.Fatalf("valid model rejected: %v", err)
	}
	for _, model := range []string{"-leading", "claude/test", "bad model", "line\nbreak", strings.Repeat("x", 129)} {
		p.NamingModel = model
		if err := c.SavePreferences(p); err == nil {
			t.Errorf("unusable model %q saved", model)
		}
		if got := c.Snapshot().Preferences.NamingModel; got != "gpt-4o:latest" {
			t.Fatalf("rejected model replaced settings: %q", got)
		}
	}
}

func TestNamingFallbackRemainsSuccessfulInDesktopView(t *testing.T) {
	r := &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Succeeded: 1}, Documents: []arcourt.LocalDocumentResult{{Status: arcourt.DocumentSucceeded, Saved: true, Filename: "fallback.pdf", Naming: &arcourt.NamingOutcome{Source: "deterministic", Reason: "image_only"}}}}
	view := resultView(r)
	if phase, _ := outcome(r, nil, nil); phase != "success" {
		t.Fatal("naming fallback changed download outcome")
	}
	if view.Documents[0].Naming == nil || view.Documents[0].Naming.Reason != "image_only" || view.Documents[0].Status != "succeeded" {
		t.Fatalf("fallback not shown: %+v", view.Documents[0])
	}
}
