package app

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Preferences struct {
	OutputDirectory string `json:"outputDirectory"`
	BrowserOverride string `json:"browserOverride"`
	CaseURLTemplate string `json:"caseURLTemplate"`
	NamingEnabled   bool   `json:"namingEnabled"`
	NamingProvider  string `json:"namingProvider"`
	NamingModel     string `json:"namingModel"`
	// NamingConsentRecipient is the exact direct endpoint approved in Settings.
	// Changing a provider endpoint requires fresh consent.
	NamingConsentRecipient string `json:"namingConsentRecipient"`
}

var namingRecipients = map[string]string{
	"openai":    "https://api.openai.com/v1/responses",
	"xai":       "https://api.x.ai/v1/chat/completions",
	"anthropic": "https://api.anthropic.com/v1/messages",
	"google":    "https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent",
}

// Keep this in step with arcourt's local NamingRequest model validation. A
// rejected model must be caught when Settings are saved, before any download.
var namingModelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func namingRecipient(provider string) string { return namingRecipients[provider] }

type PreferenceStore struct{ Path string }

func DefaultStore() (PreferenceStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return PreferenceStore{}, err
	}
	return PreferenceStore{Path: filepath.Join(dir, "ArcourtDownloader", "settings.json")}, nil
}

func (s PreferenceStore) Load() (Preferences, error) {
	var p Preferences
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(b, &p)
	if err != nil {
		return Preferences{}, err
	}
	return p, nil
}

func (s PreferenceStore) Save(p Preferences) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.Path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), s.Path)
}

// A settings form is configuration, not a credential store. Actual document URL
// validation remains exclusively in the shared service.
func validatePreferences(p Preferences) error {
	if p.NamingProvider != "" && namingRecipient(p.NamingProvider) == "" {
		return errors.New("Choose OpenAI, xAI, Anthropic, or Google for AI naming.")
	}
	if p.NamingModel != "" && !namingModelID.MatchString(p.NamingModel) {
		return errors.New("Enter a model ID of at most 128 letters, digits, dots, underscores, colons, or hyphens, starting with a letter or digit.")
	}
	if p.NamingEnabled && (p.NamingProvider == "" || strings.TrimSpace(p.NamingModel) == "") {
		return errors.New("Choose a provider and model before enabling AI naming.")
	}
	if p.NamingConsentRecipient != "" && p.NamingConsentRecipient != namingRecipient(p.NamingProvider) {
		return errors.New("Review and consent to the selected AI recipient again.")
	}
	if p.CaseURLTemplate != "" {
		u, err := url.Parse(strings.ReplaceAll(p.CaseURLTemplate, "{case_number}", "60CV-2026-1"))
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || !strings.Contains(p.CaseURLTemplate, "{case_number}") {
			return errors.New("Use an absolute HTTP(S) case page template containing {case_number}.")
		}
		if u.User != nil || u.Fragment != "" {
			return errors.New("Use a case page template without credentials or a fragment.")
		}
		for key := range u.Query() {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "auth") || strings.Contains(lower, "signature") || strings.Contains(lower, "credential") || strings.Contains(lower, "password") {
				return errors.New("Use a public case page template without secrets or signed URL parameters.")
			}
		}
	}
	if p.OutputDirectory != "" && !filepath.IsAbs(p.OutputDirectory) {
		return errors.New("Choose an absolute output folder path.")
	}
	if p.BrowserOverride != "" && !filepath.IsAbs(p.BrowserOverride) {
		return errors.New("Use the absolute, unquoted path to Edge or Chrome, or leave it blank for detection.")
	}
	return nil
}
