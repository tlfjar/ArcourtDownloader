package arcourt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BrowserInfo identifies the executable selected before any browser is launched.
type BrowserInfo struct {
	Name           string
	ExecutablePath string
	Source         string // explicit, installation, or PATH
}

// browserResolver keeps all host probes injectable for portable discovery tests.
type browserResolver struct {
	goos     string
	getenv   func(string) string
	home     func() (string, error)
	stat     func(string) (os.FileInfo, error)
	lookPath func(string) (string, error)
	abs      func(string) (string, error)
}

func hostBrowserResolver() browserResolver {
	return browserResolver{runtime.GOOS, os.Getenv, os.UserHomeDir, os.Stat, exec.LookPath, filepath.Abs}
}

func (r browserResolver) executable(path string) (string, error) {
	path, err := r.abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve executable path")
	}
	info, err := r.stat(path)
	if err != nil {
		return "", fmt.Errorf("executable is missing or inaccessible")
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("executable path is not a regular file")
	}
	if r.goos == "windows" {
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return "", fmt.Errorf("executable must be an .exe file")
		}
	} else if info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("file has no executable permission")
	}
	return path, nil
}

func (r browserResolver) resolve(explicit string) (BrowserInfo, error) {
	if explicit != "" {
		path, err := r.executable(explicit)
		if err != nil {
			return BrowserInfo{}, fmt.Errorf("invalid BrowserFetcherConfig.ExecutablePath: %w; provide the unquoted path to an installed browser executable", err)
		}
		return BrowserInfo{"Explicit browser", path, "explicit"}, nil
	}

	type candidate struct {
		name     string
		paths    []string
		commands []string
	}
	var candidates []candidate
	switch r.goos {
	case "windows":
		// Do not turn absent environment roots into relative working-directory probes.
		for _, browser := range []struct{ name, directory, exe string }{
			{"Microsoft Edge", "Microsoft/Edge/Application", "msedge.exe"},
			{"Google Chrome", "Google/Chrome/Application", "chrome.exe"},
		} {
			c := candidate{name: browser.name, commands: []string{browser.exe}}
			for _, variable := range []string{"ProgramFiles(x86)", "ProgramW6432", "ProgramFiles", "LOCALAPPDATA"} {
				if root := r.getenv(variable); root != "" && filepath.IsAbs(root) {
					c.paths = append(c.paths, filepath.Join(root, browser.directory, browser.exe))
				}
			}
			candidates = append(candidates, c)
		}
	case "darwin":
		for _, browser := range []struct{ name, bundle, exe string }{
			{"Microsoft Edge", "Microsoft Edge.app", "Microsoft Edge"},
			{"Google Chrome", "Google Chrome.app", "Google Chrome"},
			{"Chromium", "Chromium.app", "Chromium"},
		} {
			rel := filepath.Join(browser.bundle, "Contents", "MacOS", browser.exe)
			c := candidate{name: browser.name, paths: []string{filepath.Join("/Applications", rel)}}
			if home, err := r.home(); err == nil && home != "" {
				c.paths = append(c.paths, filepath.Join(home, "Applications", rel))
			}
			candidates = append(candidates, c)
		}
		fallthrough
	default:
		candidates = append(candidates,
			candidate{name: "Google Chrome", commands: []string{"google-chrome", "google-chrome-stable"}},
			candidate{name: "Chromium", commands: []string{"chromium", "chromium-browser"}},
			candidate{name: "Microsoft Edge", commands: []string{"microsoft-edge", "microsoft-edge-stable"}},
		)
	}
	for _, c := range candidates {
		for _, path := range c.paths {
			if executable, err := r.executable(path); err == nil {
				return BrowserInfo{c.name, executable, "installation"}, nil
			}
		}
		for _, command := range c.commands {
			if path, err := r.lookPath(command); err == nil {
				if executable, err := r.executable(path); err == nil {
					return BrowserInfo{c.name, executable, "PATH"}, nil
				}
			}
		}
	}
	return BrowserInfo{}, fmt.Errorf("no supported browser found on %s; install Microsoft Edge or Google Chrome, or set BrowserFetcherConfig.ExecutablePath to its executable", r.goos)
}
