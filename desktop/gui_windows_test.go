package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
	"github.com/tlfjar/ArcourtDownloader/desktop/internal/app"
)

// Opt-in actual Windows Wails/WebView2 UI. The named fixture executable must be
// built first. The authenticated local fixture bridge executes DOM interactions
// in the real WebView2 through Wails; production has no test-control server.
func TestDesktopGUI(t *testing.T) {
	if os.Getenv("ARCOURT_DESKTOP_SMOKE") != "1" {
		t.Skip("set ARCOURT_DESKTOP_SMOKE=1 after scripts/build-desktop.ps1 -Fixture")
	}
	exe, err := filepath.Abs("build/bin/ArcourtDownloader-fixture.exe")
	if err != nil {
		t.Fatal(err)
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(base, "PDF output with spaces")
	profiles := filepath.Join(base, "automation profiles")
	for _, dir := range []string{out, profiles} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	var tokenBytes [24]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(tokenBytes[:])
	cmd := exec.Command(exe)
	logFile, err := os.Create(filepath.Join(base, "application.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Env = append(os.Environ(), "ARCOURT_DESKTOP_FIXTURE_DIR="+base, "ARCOURT_DESKTOP_FIXTURE_TOKEN="+token, "TEMP="+profiles, "TMP="+profiles)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	processDone := make(chan error, 1)
	go func() { processDone <- cmd.Wait() }()
	exited := false
	native := func(action, folder string) error {
		c := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", "../scripts/desktop-native-smoke.ps1", "-Action", action, "-AppProcessId", fmt.Sprint(cmd.Process.Pid), "-Folder", folder)
		c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		output, err := c.CombinedOutput()
		if err != nil {
			return fmt.Errorf("native %s: %w: %s", action, err, output)
		}
		if strings.TrimSpace(string(output)) != "" {
			t.Log(strings.TrimSpace(string(output)))
		}
		return nil
	}
	defer func() {
		if !exited {
			_ = native("close", "")
			select {
			case <-processDone:
			case <-time.After(10 * time.Second):
				_ = cmd.Process.Kill()
				<-processDone
			}
		}
	}()
	client := &http.Client{Timeout: 12 * time.Second}
	address := ""
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(base, "ui-address")); err == nil {
			address = string(data)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if address == "" {
		_ = native("inspect", "")
		if data, err := os.ReadFile(logFile.Name()); err == nil {
			t.Log(string(data))
		}
		t.Fatal("fixture application did not start")
	}
	tryEval := func(js string, out any) error {
		b, _ := json.Marshal(map[string]string{"script": js})
		req, err := http.NewRequest("POST", address+"/__fixture/eval", bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("X-Fixture-Token", token)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("fixture bridge: HTTP %d", resp.StatusCode)
		}
		var result struct {
			Value json.RawMessage `json:"value"`
			Error string          `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return err
		}
		if result.Error != "" {
			return fmt.Errorf("fixture script: %s", result.Error)
		}
		if out != nil {
			return json.Unmarshal(result.Value, out)
		}
		return nil
	}
	eval := func(js string, out any) {
		t.Helper()
		if err := tryEval(js, out); err != nil {
			t.Fatal(err)
		}
	}
	poll := func(js string) {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		var lastErr error
		for time.Now().Before(deadline) {
			var ok bool
			lastErr = tryEval(js, &ok)
			if lastErr == nil && ok {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		var body string
		_ = tryEval("document.body.innerText", &body)
		_ = native("inspect", "")
		t.Fatalf("poll %s: %v\n%s", js, lastErr, body)
	}
	click := func(id string) {
		t.Helper()
		poll(fmt.Sprintf("!document.getElementById(%q).disabled", id))
		eval(fmt.Sprintf("document.getElementById(%q).click()", id), nil)
	}
	set := func(id, value string) {
		t.Helper()
		b, _ := json.Marshal(value)
		eval(fmt.Sprintf("(()=>{const e=document.getElementById(%q);e.value=%s;e.dispatchEvent(new Event('input',{bubbles:true}));})()", id, b), nil)
	}
	snapshot := func() app.State { t.Helper(); var s app.State; eval("window.go.main.Desktop.Snapshot()", &s); return s }
	idle := func() {
		t.Helper()
		poll("!document.getElementById('preview').disabled && !document.getElementById('status').textContent.includes('Loading')")
	}
	preview := func(number string) {
		t.Helper()
		set("case-number", number)
		click("preview")
		poll("!document.getElementById('preview-area').hidden && document.getElementById('header-title').textContent.includes(" + fmt.Sprintf("%q", number) + ")")
		idle()
	}
	selectAll := func() {
		t.Helper()
		click("verified")
		click("select-all")
		poll("document.getElementById('selection-count').textContent.startsWith('2 of')")
	}
	download := func() { t.Helper(); click("download"); poll("!document.getElementById('results').hidden"); idle() }
	poll("typeof window.go?.main?.Desktop !== 'undefined' && document.getElementById('browser-info').textContent.includes('Microsoft')")
	poll("document.getElementById('about-version').textContent.includes('development, unsigned') && document.getElementById('about-version').textContent.includes('source ')")
	t.Log("Native Windows application and WebView2 started.")
	click("choose-folder")
	if err := native("choose", out); err != nil {
		t.Fatal(err)
	}
	poll("document.getElementById('output-path').textContent.includes('PDF output with spaces')")
	set("case-number", "60CV-2026-5")
	click("preview")
	poll("document.getElementById('status').dataset.phase === 'error'")
	idle()
	if s := snapshot(); s.Preview != nil || s.Verified || s.Selected != 0 || s.Phase != "error" {
		t.Fatalf("mismatched case was accepted: %+v", s)
	}
	poll("document.getElementById('download').disabled")
	t.Log("Mismatched case header rejected before selection or download.")
	preview("60CV-2026-1")
	click("verified")
	poll("!document.querySelector('#documents input').disabled")
	eval("document.querySelector('#documents input').click()", nil)
	poll("document.getElementById('selection-count').textContent.startsWith('1 of')")
	download()
	s := snapshot()
	if s.Phase != "success" || s.Result.Counts.Succeeded != 1 {
		t.Fatalf("success: %+v", s)
	}
	doc := s.Result.Documents[0]
	data, err := os.ReadFile(filepath.Join(s.Result.Directory, doc.Filename))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != doc.SHA256 || !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatal("PDF/hash mismatch")
	}
	manifest, err := os.ReadFile(filepath.Join(s.Result.Directory, arcourt.ManifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), doc.DocumentID) || strings.Contains(string(manifest), "DO_NOT_DISPLAY") {
		t.Fatal("manifest identity/redaction")
	}
	var body string
	eval("document.body.innerText", &body)
	if strings.Contains(body, "DO_NOT_DISPLAY") {
		t.Fatal("signed URL exposed")
	}
	click("open-folder")
	if err := native("open", s.Result.Directory); err != nil {
		t.Fatal(err)
	}
	download()
	s = snapshot()
	if s.Phase != "success" || s.Result.Counts.Skipped != 1 || s.Result.Documents[0].SkipReason != arcourt.SkipVerified {
		t.Fatalf("repeat: %+v", s)
	}
	click("select-all")
	download()
	s = snapshot()
	if s.Phase != "success" || s.Result.Counts.Succeeded != 1 || s.Result.Counts.Skipped != 1 || s.Result.Partial {
		t.Fatalf("all coverage: %+v", s)
	}
	if !strings.HasPrefix(s.Message, "Download complete: 1 saved; 1 already downloaded and verified.") || !strings.Contains(s.Message, "Select All included every document in this preview.") || len(s.Preview.Warnings) == 0 {
		t.Fatalf("successful Select All lost coverage explanation: %+v", s)
	}
	poll("document.getElementById('status').dataset.phase === 'success' && document.getElementById('status').textContent.includes('Download complete')")
	t.Log("Individual download, exact PDF/hash/manifest, native open-folder, verified repeat, and successful Select All with coverage notes passed.")
	if capture := os.Getenv("ARCOURT_DESKTOP_CAPTURE"); capture != "" {
		if err := native("capture", capture); err != nil {
			t.Fatal(err)
		}
	}
	preview("60CV-2026-2")
	selectAll()
	download()
	s = snapshot()
	if s.Phase != "partial" || s.Result.Counts.Succeeded != 1 || s.Result.Counts.Unavailable != 1 {
		t.Fatalf("unavailable: %+v", s)
	}
	preview("60CV-2026-3")
	selectAll()
	click("download")
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(base, "slow-started")); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(base, "slow-started")); err != nil {
		t.Fatal("slow transfer never began")
	}
	click("cancel")
	poll("document.getElementById('status').dataset.phase === 'canceled'")
	idle()
	s = snapshot()
	if s.Result.Counts.Succeeded != 1 || s.Result.Counts.Canceled != 1 {
		t.Fatalf("cancel: %+v", s)
	}
	t.Log("Unavailable document and cancellation retained partial PDFs and accurate totals.")
	denied := filepath.Join(base, "unwritable output")
	if err := os.Mkdir(denied, 0700); err != nil {
		t.Fatal(err)
	}
	if err := native("deny", denied); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := native("restore", denied); err != nil {
			t.Error(err)
		}
	}()
	click("settings-toggle")
	set("output-setting", denied)
	click("save-settings")
	idle()
	preview("60CV-2026-4")
	selectAll()
	download()
	s = snapshot()
	if s.Phase == "success" || s.Result.Counts.Failed != 2 {
		t.Fatalf("unwritable: %+v", s)
	}
	set("browser", filepath.Join(base, "missing browser.exe"))
	click("save-settings")
	idle()
	click("preview")
	poll("document.getElementById('status').dataset.phase === 'error'")
	idle()
	s = snapshot()
	if s.Preview != nil || !strings.Contains(s.Message, "Browser setup") {
		t.Fatalf("missing browser: %+v", s)
	}
	t.Log("Native ACL write denial and missing-browser setup failed with actionable UI errors.")
	set("browser", "")
	set("output-setting", out)
	click("save-settings")
	idle()
	preview("60CV-2026-3")
	selectAll()
	if err := os.Remove(filepath.Join(base, "slow-started")); err != nil {
		t.Fatal(err)
	}
	click("download")
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(base, "slow-started")); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(base, "slow-started")); err != nil {
		t.Fatal("close test transfer never began")
	}
	if err := native("close", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-processDone:
		exited = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("window close did not finish cleanup")
	}
	if err := native("cleanup", profiles); err != nil {
		t.Fatal(err)
	}
	t.Log("Native close during download canceled work, exited, and removed owned automation browsers/profiles.")
}
