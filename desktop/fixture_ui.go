//go:build desktopfixture && !bindings

package main

// Local, authenticated test control uses the shell's normal script executor.
// It is compiled out of production; it neither changes WebView2 settings nor
// enables an external debugging port.
import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var fixtureUI struct {
	sync.Mutex
	ctx context.Context
}
var fixtureEval sync.Mutex

func fixtureReady(ctx context.Context) { fixtureUI.Lock(); fixtureUI.ctx = ctx; fixtureUI.Unlock() }

func serveFixtureUI(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/__fixture/eval" {
		return false
	}
	token := os.Getenv("ARCOURT_DESKTOP_FIXTURE_TOKEN")
	if token == "" || r.Header.Get("X-Fixture-Token") != token || r.Header.Get("Origin") != "" || r.Method != "POST" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return true
	}
	fixtureUI.Lock()
	ctx := fixtureUI.ctx
	fixtureUI.Unlock()
	if ctx == nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return true
	}
	fixtureEval.Lock()
	defer fixtureEval.Unlock()
	var input struct {
		Script string `json:"script"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&input); err != nil {
		http.Error(w, "bad request", 400)
		return true
	}
	result := make(chan string, 1)
	unsub := wruntime.EventsOnce(ctx, "fixture-result", func(data ...interface{}) {
		if len(data) > 0 {
			if s, ok := data[0].(string); ok {
				select {
				case result <- s:
				default:
				}
			}
		}
	})
	defer unsub()
	wruntime.WindowExecJS(ctx, `(async()=>{try{const value=await (`+input.Script+`);window.runtime.EventsEmit('fixture-result',JSON.stringify({value:value??null}));}catch(e){window.runtime.EventsEmit('fixture-result',JSON.stringify({error:String(e)}));}})()`)
	select {
	case data := <-result:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(data))
	case <-r.Context().Done():
	case <-time.After(10 * time.Second):
		http.Error(w, "script timed out", 504)
	}
	return true
}
