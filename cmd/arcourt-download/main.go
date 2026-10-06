package main

import (
	"context"
	"os"
	"os/signal"
)

func main() {
	// Exit only after run has drained progress and closed the owned fetcher.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr, browserService)
	stop()
	os.Exit(code)
}
