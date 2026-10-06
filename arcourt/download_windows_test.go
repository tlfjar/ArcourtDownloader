//go:build windows

package arcourt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestLocalWindowsJunctionDestinations(t *testing.T) {
	for _, kind := range []string{"output", "ancestor", "case"} {
		t.Run(kind, func(t *testing.T) {
			s, _, req := localFixture(t, 1)
			outside := t.TempDir()
			link := filepath.Join(req.OutputDirectory, "junction")
			if kind == "case" {
				link = filepath.Join(req.OutputDirectory, "case-60cv-2026-1")
			}
			// Environment operands avoid shell interpolation of temporary paths.
			cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference = 'Stop'; New-Item -ItemType Junction -Path $env:ARCOURT_TEST_JUNCTION -Target $env:ARCOURT_TEST_TARGET | Out-Null`)
			cmd.Env = append(os.Environ(), "ARCOURT_TEST_JUNCTION="+link, "ARCOURT_TEST_TARGET="+outside)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("create native Windows junction: %v %s", err, output)
			}
			defer os.Remove(link)
			if kind == "output" {
				req.OutputDirectory = link
			}
			if kind == "ancestor" {
				if err := os.Mkdir(filepath.Join(outside, "chosen"), 0700); err != nil {
					t.Fatal(err)
				}
				req.OutputDirectory = filepath.Join(link, "chosen")
			}
			if _, err := s.Download(context.Background(), req, nil); !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("junction accepted: %v", err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if kind == "ancestor" {
				want = 1
			}
			if len(entries) != want {
				t.Fatal("wrote through junction")
			}
		})
	}
}

func TestLocalWindowsOpenManifestRenameFailure(t *testing.T) {
	s, _, req := localFixture(t, 1)
	var held *os.File
	defer func() {
		if held != nil {
			held.Close()
		}
	}()
	s.diskBefore = func(op, name string) error {
		if op == "manifest.rename" && held == nil {
			var err error
			held, err = os.Open(filepath.Join(req.OutputDirectory, "case-60cv-2026-1", ManifestFilename))
			return err
		}
		return nil
	}
	r, err := s.Download(context.Background(), req, nil)
	if !errors.Is(err, ErrManifest) || !r.Documents[0].Saved {
		t.Fatalf("Windows open-handle rename: %+v %v", r, err)
	}
	held.Close()
	held = nil
	s.diskBefore = nil
	again, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, again, DownloadCounts{Selected: 1, Skipped: 1})
}
