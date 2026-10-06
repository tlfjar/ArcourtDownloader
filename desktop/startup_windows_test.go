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
