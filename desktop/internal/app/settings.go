package app

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Preferences struct {
	OutputDirectory string `json:"outputDirectory"`
	BrowserOverride string `json:"browserOverride"`
	CaseURLTemplate string `json:"caseURLTemplate"`
}

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
