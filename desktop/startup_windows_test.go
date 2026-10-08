package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestDesktopStartup(t *testing.T) {
	if os.Getenv("ARCOURT_DESKTOP_SMOKE") != "1" {
		t.Skip("set ARCOURT_DESKTOP_SMOKE=1 after building the normal executable")
	}
	exe, err := filepath.Abs("build/bin/ArcourtDownloader.exe")
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	// WebView2 can finish writing its cache just after the app exits. Remove
	// this test-owned profile with a bounded retry before TempDir's final
	// cleanup, so a transient cache write does not fail the startup fixture.
	t.Cleanup(func() {
		deadline := time.Now().Add(10 * time.Second)
		for {
			err := os.RemoveAll(base)
			if err == nil {
				time.Sleep(200 * time.Millisecond)
				_, statErr := os.Stat(base)
				if os.IsNotExist(statErr) {
					return
				}
				if statErr != nil {
					err = statErr
				} else {
					err = fmt.Errorf("profile directory was recreated")
				}
			}
			if time.Now().After(deadline) {
				t.Errorf("WebView2 test profile cleanup did not settle: %v", err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	for _, dir := range []string{"Roaming", "Local"} {
		if err := os.Mkdir(filepath.Join(base, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "APPDATA="+filepath.Join(base, "Roaming"), "LOCALAPPDATA="+filepath.Join(base, "Local"))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false
	defer func() {
		if !exited {
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	for _, action := range []string{"startup", "close"} {
		check := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", "../scripts/desktop-native-smoke.ps1", "-Action", action, "-AppProcessId", fmt.Sprint(cmd.Process.Pid))
		check.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("normal executable %s: %v %s", action, err, output)
		} else if len(output) > 0 {
			t.Log(string(output))
		}
	}
	select {
	case err := <-done:
		exited = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("normal executable failed to close")
	}
}
