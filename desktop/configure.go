//go:build !desktopfixture || bindings

package main

import (
	"context"
	"github.com/tlfjar/ArcourtDownloader/desktop/internal/app"
	"path/filepath"
)

func fixtureReady(context.Context) {}

func configure(store app.PreferenceStore) (app.Factory, app.PreferenceStore, func()) {
	return app.BrowserService, store, func() {}
}

func webviewDirectory(cache string, _ app.PreferenceStore) string {
	return filepath.Join(cache, "ArcourtDownloader", "WebView2")
}
