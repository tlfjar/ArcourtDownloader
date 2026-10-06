package main

import (
	"context"
	"embed"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/tlfjar/ArcourtDownloader/buildinfo"
	"github.com/tlfjar/ArcourtDownloader/desktop/internal/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

type Desktop struct {
	controller *app.Controller
	ctx        context.Context
	closeOnce  sync.Once
	mayClose   atomic.Bool
}

func (d *Desktop) Snapshot() app.State     { return d.controller.Snapshot() }
func (d *Desktop) BuildInfo() buildinfo.Info { return buildinfo.Current() }
func (d *Desktop) SetCase(s string) error  { return d.controller.SetCase(s) }
func (d *Desktop) PreviewCase() error      { return d.controller.PreviewCase() }
func (d *Desktop) Download(g uint64) error { return d.controller.Download(g) }
func (d *Desktop) Cancel()                 { d.controller.Cancel() }
func (d *Desktop) SetSelection(g uint64, ids []string, verified, all bool) error {
	return d.controller.SetSelection(g, ids, verified, all)
}
func (d *Desktop) SavePreferences(p app.Preferences) error { return d.controller.SavePreferences(p) }

func (d *Desktop) ChooseFolder() (string, error) {
	s := d.Snapshot()
	if s.Busy || s.Closing {
		return "", errors.New("Wait for the operation to finish before choosing a folder.")
	}
	initial := s.Preferences.OutputDirectory
	if info, err := os.Stat(initial); err != nil || !info.IsDir() {
		initial = ""
	}
	folder, err := wruntime.OpenDirectoryDialog(d.ctx, wruntime.OpenDialogOptions{Title: "Choose PDF output folder", DefaultDirectory: initial, CanCreateDirectories: true})
	if err != nil {
		return "", errors.New("Folder picker could not open. Enter an existing absolute folder path in Settings.")
	}
	if folder == "" {
		return "", nil
	}
	s.Preferences.OutputDirectory = folder
	if err := d.SavePreferences(s.Preferences); err != nil {
		return "", err
	}
	return folder, nil
}

func (d *Desktop) OpenOutputFolder() error {
	s := d.Snapshot()
	folder := s.Preferences.OutputDirectory
	if s.Result != nil && s.Result.Directory != "" {
		folder = s.Result.Directory
	}
	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() || !filepath.IsAbs(folder) {
		return errors.New("Output folder is missing. Choose an existing local folder.")
	}
	if err := openFolder(folder); err != nil {
		return errors.New("Windows could not open the output folder. Open the displayed path in File Explorer.")
	}
	return nil
}

func (d *Desktop) beforeClose(ctx context.Context) bool {
	if d.mayClose.Load() {
		return false
	}
	d.closeOnce.Do(func() {
		go func() {
			d.controller.Close()
			if message := d.Snapshot().Diagnostic; message != "" {
				_, _ = wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{Type: wruntime.WarningDialog, Title: "Arcourt Downloader", Message: message})
			}
			d.mayClose.Store(true)
			wruntime.Quit(ctx)
		}()
	})
	return true
}

func main() {
	store, err := app.DefaultStore()
	if err != nil {
		startupError("Could not locate your Windows settings folder.")
		return
	}
	factory, store, finish := configure(store)
	defer finish()
	d := &Desktop{controller: app.New(factory, store)}
	cache, err := os.UserCacheDir()
	if err != nil {
		startupError("Could not locate your Windows local application data folder.")
		return
	}
	err = wails.Run(&options.App{
		Title: "Arcourt Downloader", Width: 1120, Height: 850, MinWidth: 800, MinHeight: 640,
		BackgroundColour: options.NewRGB(244, 246, 250),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        func(ctx context.Context) { d.ctx = ctx },
		OnDomReady:       fixtureReady,
		OnBeforeClose:    d.beforeClose,
		OnShutdown:       func(context.Context) { d.controller.Close() },
		Bind:             []interface{}{d},
		Windows:          &windows.Options{WebviewUserDataPath: webviewDirectory(cache, store)},
	})
	if err != nil {
		startupError("Arcourt Downloader could not start. Install Microsoft Edge WebView2 Runtime and check local application data permissions. Edge or Chrome is also required for court downloads.")
	}
}
