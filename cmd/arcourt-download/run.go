package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
	"github.com/tlfjar/ArcourtDownloader/buildinfo"
)

const (
	exitSuccess  = 0
	exitFailure  = 1
	exitInvalid  = 2
	exitCanceled = 130
)

// Only the shared application's existing contract is needed by this frontend.
type service interface {
	Preview(context.Context, string) (*arcourt.CasePreview, error)
	Download(context.Context, arcourt.DownloadRequest, chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error)
}

type serviceFactory func(arcourt.BrowserFetcherConfig) (service, func() error, error)

func browserService(cfg arcourt.BrowserFetcherConfig) (service, func() error, error) {
	fetcher, err := arcourt.NewBrowserFetcher(cfg)
	if err != nil {
		return nil, nil, err
	}
	s, err := arcourt.NewDownloadService(fetcher)
	if err != nil {
		_ = fetcher.Close()
		return nil, nil, err
	}
	return s, fetcher.Close, nil
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, factory serviceFactory) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		fmt.Fprintln(stdout, buildinfo.Current().String())
		return exitSuccess
	}
	o, err := parseOptions(args, getenv)
	r := response{Command: o.command}
	code := exitSuccess
	switch {
	case errors.Is(err, flag.ErrHelp):
		r.Help = usage
	case err != nil:
		code, r.Error = exitInvalid, err.Error()
	default:
		r, code = execute(ctx, o, stderr, factory)
	}
	r.ExitCode = code
	r.Status = map[int]string{exitSuccess: "success", exitFailure: "partial_or_failure", exitInvalid: "invalid_invocation", exitCanceled: "canceled"}[code]
	if r.Error != "" {
		fmt.Fprintln(stderr, "Error:", r.Error)
	}
	if err := writeResponse(stdout, o.json, r); err != nil {
		fmt.Fprintln(stderr, "Error: could not write command result to stdout")
		if code == exitSuccess {
			return exitFailure
		}
	}
	return code
}

func execute(parent context.Context, o options, stderr io.Writer, factory serviceFactory) (r response, code int) {
	r.Command = o.command
	ctx, cancel := context.WithTimeout(parent, o.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		r.Error = operationError(err)
		return r, errorCode(err)
	}
	s, closeService, err := factory(o.browser)
	if err != nil {
		// Browser construction only resolves configuration/executables; its errors
		// contain no fetched pages or request URLs.
		r.Error = "browser setup failed: " + err.Error()
		return r, exitFailure
	}
	defer func() {
		if err := closeService(); err != nil {
			message := "browser cleanup failed; check temporary-directory permissions and file locks"
			if r.Error == "" {
				r.Error = message
			} else {
				r.Error += "; " + message
			}
			if code == exitSuccess {
				code = exitFailure
			}
		}
	}()
	fmt.Fprintf(stderr, "Discovering case %s...\n", o.caseNumber)
	p, err := s.Preview(ctx, o.caseNumber)
	if err != nil {
		r.Error = operationError(err)
		return r, errorCode(err)
	}
	// The real service validates identity. Keep a defensive check at this boundary
	// so an injected service can never turn a mismatched preview into a selection.
	if p == nil || strings.ToUpper(strings.TrimSpace(p.CaseInfo.CaseNumber)) != o.caseNumber {
		r.Error = operationError(arcourt.ErrCaseNumberMismatch)
		return r, exitFailure
	}
	r.Preview = publicPreview(p)
	if !p.Discovery.Complete || p.Discovery.Truncated {
		r.Warnings = append(r.Warnings, "Discovery is incomplete: only the refreshed preview's discovered documents can be selected; whole-docket completeness is not established.")
	}
	for _, warning := range r.Warnings {
		fmt.Fprintln(stderr, "Warning:", warning)
	}
	if o.command == "preview" {
		return r, exitSuccess
	}
	selected, err := selectDocuments(p, o)
	if err != nil {
		r.Error = err.Error()
		return r, exitInvalid
	}
	// Own/drain the channel through the Download return, even on cancellation.
	// Its best-effort events never replace the authoritative returned result.
	events := make(chan arcourt.DownloadEvent, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for event := range events {
			switch event.Kind {
			case arcourt.DownloadStarted:
				fmt.Fprintf(stderr, "Downloading %s...\n", event.DocumentID)
			case arcourt.DownloadDocumentDone:
				fmt.Fprintf(stderr, "%s: %s\n", event.DocumentID, event.Document.Status)
			}
		}
	}()
	result, downloadErr := s.Download(ctx, arcourt.DownloadRequest{
		CaseNumber: o.caseNumber, Selection: selected, OutputDirectory: o.output,
	}, events)
	close(events)
	<-done
	if result != nil {
		r.Download = publicDownload(result)
	} else if downloadErr == nil {
		downloadErr = errors.New("missing download result")
	}
	if ctx.Err() != nil {
		downloadErr = errors.Join(downloadErr, ctx.Err())
	}
	if o.all && len(r.Warnings) != 0 {
		downloadErr = errors.Join(downloadErr, arcourt.ErrIncompleteDiscovery)
	}
	// The shared service already reports these as errors. Defend against a caller
	// accidentally suppressing that error when adapting the service interface.
	if result != nil {
		for _, doc := range result.Documents {
			if doc.Status != arcourt.DocumentSucceeded && !(doc.Status == arcourt.DocumentSkipped && doc.SkipReason == arcourt.SkipVerified) {
				downloadErr = errors.Join(downloadErr, arcourt.ErrDocumentsIncomplete)
			}
		}
	}
	if downloadErr != nil {
		r.Error = operationError(downloadErr)
		return r, errorCode(downloadErr)
	}
	return r, exitSuccess
}

func selectDocuments(p *arcourt.CasePreview, o options) ([]arcourt.DocketEntry, error) {
	entries := map[string]arcourt.DocketEntry{}
	var all []arcourt.DocketEntry
	for _, entry := range p.DocketEntries {
		if strings.TrimSpace(entry.SourceURL) == "" {
			continue
		}
		id := arcourt.DocumentID(entry.SourceURL)
		if _, exists := entries[id]; !exists {
			entries[id] = entry
			all = append(all, entry)
		}
	}
	selected := all
	if !o.all {
		selected = nil
		for _, id := range o.ids {
			entry, ok := entries[id]
			if !ok {
				return nil, fmt.Errorf("document ID %s is absent from fresh discovery; run preview again and check --max-documents", id)
			}
			selected = append(selected, entry)
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("fresh discovery contains no selectable documents; nothing was downloaded")
	}
	if len(selected) > o.browser.MaxDocuments {
		return nil, errors.New("selection exceeds --max-documents")
	}
	return selected, nil
}

func errorCode(err error) int {
	switch {
	case errors.Is(err, context.Canceled):
		return exitCanceled
	case errors.Is(err, arcourt.ErrInvalidCaseNumber):
		return exitInvalid
	default:
		return exitFailure
	}
}

// Never print underlying transport errors: they can contain signed request URLs.
func operationError(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "operation canceled; any completed PDFs remain saved"
	case errors.Is(err, context.DeadlineExceeded):
		return "operation timed out; inspect saved results and the configured timeouts"
	case errors.Is(err, arcourt.ErrInvalidCaseNumber):
		return "invalid case number"
	case errors.Is(err, arcourt.ErrCaseNumberMismatch):
		return "fresh discovery did not match the requested case; check the case number and URL template"
	case errors.Is(err, arcourt.ErrUnsafeDestination):
		return "unsafe output directory; use a local directory without symlinks or reparse points"
	case errors.Is(err, arcourt.ErrOutputBusy), errors.Is(err, arcourt.ErrDownloadBusy):
		return "another operation owns this service or case directory; retry after it finishes"
	case errors.Is(err, arcourt.ErrManifest):
		return "manifest could not be read or saved; inspect the result and existing output before retrying"
	case errors.Is(err, arcourt.ErrDocumentsIncomplete):
		return "one or more selected documents did not succeed; inspect document outcomes"
	case errors.Is(err, arcourt.ErrIncompleteDiscovery):
		return "--all saved the discovered selection, but discovery does not establish a complete docket"
	case errors.Is(err, arcourt.ErrBackendUnavailable):
		return "court backend is unavailable; try again later"
	case errors.Is(err, arcourt.ErrCaseAccessDenied):
		return "court denied case access (401/403); open the court page to check access or try again later"
	case errors.Is(err, arcourt.ErrCaseLoadFailed):
		return "case page did not load usable content; check the case number and URL template"
	default:
		return "operation failed; check browser installation, URL template, connection, output permissions, and document outcomes"
	}
}
