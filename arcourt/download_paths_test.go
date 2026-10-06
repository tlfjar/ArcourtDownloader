package arcourt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalSymlinkDestinations(t *testing.T) {
	for _, kind := range []string{"output", "ancestor", "case", "manifest", "lock", "pdf"} {
		t.Run(kind, func(t *testing.T) {
			s, _, req := localFixture(t, 1)
			outside := t.TempDir()
			var link, target string
			switch kind {
			case "output":
				link = filepath.Join(req.OutputDirectory, "link")
				target = outside
				req.OutputDirectory = link
			case "ancestor":
				link = filepath.Join(req.OutputDirectory, "link")
				target = outside
				if err := os.Mkdir(filepath.Join(outside, "chosen"), 0700); err != nil {
					t.Fatal(err)
				}
				req.OutputDirectory = filepath.Join(link, "chosen")
			case "case":
				link = filepath.Join(req.OutputDirectory, "case-60cv-2026-1")
				target = outside
			default:
				r, err := s.Download(context.Background(), req, nil)
				if err != nil {
					t.Fatal(err)
				}
				name := ManifestFilename
				if kind == "lock" {
					name = ".arcourt.lock"
				}
				if kind == "pdf" {
					name = r.Documents[0].Filename
				}
				link = filepath.Join(r.Directory, name)
				target = filepath.Join(outside, "target")
				data, err := os.ReadFile(link)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlink creation unavailable: %v; Windows junction gate runs separately", err)
			}
			defer os.Remove(link)
			before, _ := os.ReadFile(target)
			r, err := s.Download(context.Background(), req, nil)
			if kind == "pdf" {
				if err != nil || r.Documents[0].Status != DocumentSucceeded || strings.EqualFold(filepath.Base(link), r.Documents[0].Filename) {
					t.Fatalf("symlink PDF was trusted: %+v %v", r, err)
				}
			} else if !errors.Is(err, ErrUnsafeDestination) && !errors.Is(err, ErrManifest) {
				t.Fatalf("symlink accepted: %v", err)
			}
			after, _ := os.ReadFile(target)
			if string(before) != string(after) {
				t.Fatal("symlink target modified")
			}
		})
	}
}

func TestLocalPublishCollisionAndOwnedCleanup(t *testing.T) {
	s, _, req := localFixture(t, 1)
	dir := filepath.Join(req.OutputDirectory, "case-60cv-2026-1")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, ".arcourt-pdf-not-owned.tmp")
	if err := os.WriteFile(foreign, []byte("foreign partial"), 0600); err != nil {
		t.Fatal(err)
	}
	collided := ""
	s.diskBefore = func(op, name string) error {
		if op == "publish.link" && collided == "" {
			collided = name
			return os.WriteFile(filepath.Join(dir, name), []byte("concurrent writer"), 0600)
		}
		return nil
	}
	r, err := s.Download(context.Background(), req, nil)
	if err != nil || r.Documents[0].Filename == collided {
		t.Fatalf("no-clobber collision: %+v %v", r, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, collided))
	if string(data) != "concurrent writer" {
		t.Fatal("concurrent file overwritten")
	}
	data, _ = os.ReadFile(foreign)
	if string(data) != "foreign partial" {
		t.Fatal("another run's temporary file removed")
	}
}

func TestLocalUnrelatedRecoveryTargetNotAdopted(t *testing.T) {
	s, f, req := localFixture(t, 1)
	s.diskBefore = func(op, name string) error {
		if op == "manifest.rename" {
			return errors.New("injected")
		}
		return nil
	}
	r, _ := s.Download(context.Background(), req, nil)
	path := filepath.Join(r.Directory, r.Documents[0].Filename)
	// Identical contents at the recorded name are insufficient: a replacement
	// has a different file identity from the retained temporary hard link.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fixturePDF), 0600); err != nil {
		t.Fatal(err)
	}
	s.diskBefore = nil
	again, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, again, DownloadCounts{Selected: 1, Succeeded: 1})
	if len(f.calls) != 2 || again.Documents[0].Filename == r.Documents[0].Filename {
		t.Fatal("unowned final file adopted")
	}
}
