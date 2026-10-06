package arcourt

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type probeFileInfo struct{ mode os.FileMode }

func (i probeFileInfo) Name() string       { return "browser" }
func (i probeFileInfo) Size() int64        { return 1 }
func (i probeFileInfo) Mode() os.FileMode  { return i.mode }
func (i probeFileInfo) ModTime() time.Time { return time.Time{} }
func (i probeFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i probeFileInfo) Sys() any           { return nil }

func testResolver(goos string, env map[string]string, files map[string]os.FileMode, commands map[string]string) browserResolver {
	return browserResolver{
		goos:   goos,
		getenv: func(k string) string { return env[k] },
		home:   func() (string, error) { return "/users/test", nil },
		stat: func(p string) (os.FileInfo, error) {
			if mode, ok := files[p]; ok {
				return probeFileInfo{mode}, nil
			}
			return nil, os.ErrNotExist
		},
		lookPath: func(p string) (string, error) {
			if found, ok := commands[p]; ok {
				return found, nil
			}
			return "", os.ErrNotExist
		},
		abs: func(p string) (string, error) { return p, nil },
	}
}

func TestBrowserResolver(t *testing.T) {
	root := t.TempDir()
	pf := filepath.Join(root, "Program Files")
	pf86 := filepath.Join(root, "Program Files (x86)")
	local := filepath.Join(root, "User With Spaces", "AppData", "Local")
	edge := filepath.Join(pf86, "Microsoft/Edge/Application/msedge.exe")
	edge64 := filepath.Join(pf, "Microsoft/Edge/Application/msedge.exe")
	chrome := filepath.Join(pf, "Google/Chrome/Application/chrome.exe")
	userEdge := filepath.Join(local, "Microsoft/Edge/Application/msedge.exe")
	userChrome := filepath.Join(local, "Google/Chrome/Application/chrome.exe")
	override := filepath.Join(root, "Custom Browser", "browser.exe")
	macChrome := filepath.Join("/Applications", "Google Chrome.app/Contents/MacOS/Google Chrome")
	macUserEdge := filepath.Join("/users/test", "Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge")
	env := map[string]string{"ProgramFiles(x86)": pf86, "ProgramFiles": pf, "LOCALAPPDATA": local}
	tests := []struct {
		name, goos, explicit  string
		env                   map[string]string
		files                 map[string]os.FileMode
		commands              map[string]string
		want, source, errText string
	}{
		{name: "override wins", explicit: override, env: env, files: map[string]os.FileMode{override: 0600, edge: 0600}, want: override, source: "explicit"},
		{name: "invalid override never falls back", explicit: override, env: env, files: map[string]os.FileMode{edge: 0600}, errText: "invalid BrowserFetcherConfig.ExecutablePath"},
		{name: "directory override", explicit: override, files: map[string]os.FileMode{override: os.ModeDir}, errText: "not a regular file"},
		{name: "non exe override", explicit: "browser.txt", files: map[string]os.FileMode{"browser.txt": 0600}, errText: ".exe"},
		{name: "edge only", env: env, files: map[string]os.FileMode{edge: 0600}, want: edge, source: "installation"},
		{name: "chrome only", env: env, files: map[string]os.FileMode{chrome: 0600}, want: chrome, source: "installation"},
		{name: "both installed", env: env, files: map[string]os.FileMode{chrome: 0600, edge: 0600}, want: edge, source: "installation"},
		{name: "native program files", env: map[string]string{"ProgramW6432": pf}, files: map[string]os.FileMode{edge64: 0600}, want: edge64, source: "installation"},
		{name: "per user edge with spaces", env: env, files: map[string]os.FileMode{userEdge: 0600, chrome: 0600}, want: userEdge, source: "installation"},
		{name: "per user chrome with spaces", env: env, files: map[string]os.FileMode{userChrome: 0600}, want: userChrome, source: "installation"},
		{name: "directory skipped", env: env, files: map[string]os.FileMode{edge: os.ModeDir, chrome: 0600}, want: chrome, source: "installation"},
		{name: "special file skipped", env: env, files: map[string]os.FileMode{edge: os.ModeNamedPipe, chrome: 0600}, want: chrome, source: "installation"},
		{name: "missing environment PATH", files: map[string]os.FileMode{override: 0600}, commands: map[string]string{"msedge.exe": override}, want: override, source: "PATH"},
		{name: "chrome PATH", files: map[string]os.FileMode{override: 0600}, commands: map[string]string{"chrome.exe": override}, want: override, source: "PATH"},
		{name: "edge PATH precedes installed chrome", env: env, files: map[string]os.FileMode{override: 0600, chrome: 0600}, commands: map[string]string{"msedge.exe": override}, want: override, source: "PATH"},
		{name: "missing browser", errText: "install Microsoft Edge or Google Chrome"},
		{name: "missing roots never probe cwd", files: map[string]os.FileMode{filepath.Join("Microsoft/Edge/Application/msedge.exe"): 0600}, errText: "no supported browser"},
		{name: "relative environment ignored", env: map[string]string{"ProgramFiles": "relative"}, files: map[string]os.FileMode{filepath.Join("relative/Microsoft/Edge/Application/msedge.exe"): 0600}, errText: "no supported browser"},
		{name: "PATH directory rejected", files: map[string]os.FileMode{override: os.ModeDir}, commands: map[string]string{"msedge.exe": override}, errText: "no supported browser"},
		{name: "linux fallback", goos: "linux", files: map[string]os.FileMode{"/usr/bin/chromium": 0755}, commands: map[string]string{"chromium": "/usr/bin/chromium"}, want: "/usr/bin/chromium", source: "PATH"},
		{name: "linux non executable", goos: "linux", explicit: "/browser", files: map[string]os.FileMode{"/browser": 0644}, errText: "no executable permission"},
		{name: "mac system app", goos: "darwin", files: map[string]os.FileMode{macChrome: 0755}, want: macChrome, source: "installation"},
		{name: "mac user app", goos: "darwin", files: map[string]os.FileMode{macUserEdge: 0755}, want: macUserEdge, source: "installation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.goos == "" {
				tt.goos = "windows"
			}
			r := testResolver(tt.goos, tt.env, tt.files, tt.commands)
			got, err := r.resolve(tt.explicit)
			if tt.errText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errText) {
					t.Fatalf("got %v; want %q", err, tt.errText)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ExecutablePath != tt.want || got.Source != tt.source {
				t.Fatalf("got %+v, want %q (%s)", got, tt.want, tt.source)
			}
		})
	}
}

func TestResolverProbeFailures(t *testing.T) {
	r := testResolver("darwin", nil, nil, nil)
	r.home = func() (string, error) { return "", errors.New("no home") }
	if _, err := r.resolve(""); err == nil {
		t.Fatal("expected missing browser")
	}
	r.abs = func(string) (string, error) { return "", errors.New("bad path") }
	if _, err := r.resolve("override"); err == nil || !strings.Contains(err.Error(), "cannot resolve") {
		t.Fatalf("got %v", err)
	}
}
