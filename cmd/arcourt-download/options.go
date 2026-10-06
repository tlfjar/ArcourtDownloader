package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
)

const usage = `Usage:
  arcourt-download --version
  arcourt-download preview --case CASE --case-url-template TEMPLATE [options]
  arcourt-download download --case CASE --output DIRECTORY (--id ID ... | --all) [options]

Options (after the command; use --flag=value for boolean values):
  --case                  Required case number, for example 60CV-2026-1
  --case-url-template     Required HTTP(S) template containing {case_number}
  --output                Existing local output directory (download only)
  --id                    Full document_id from preview; repeat to select several
  --all                   Select every document in the refreshed, bounded preview
  --browser               Installed Edge/Chrome executable; default auto-discovery
  --headless=true          Set --headless=false to show the isolated browser
  --timeout=10m            Whole command deadline, greater than 0 and at most 24h
  --page-timeout=90s       Each browser load deadline, greater than 0 and at most 5m
  --document-timeout=45s   Each HTTP attempt deadline, greater than 0 and at most 5m
  --max-documents=200      Maximum preview/selection size, 1..10000
  --max-pdf-bytes=67108864 PDF byte limit, 1..1073741824
  --max-attempts=3         HTTP attempts per document, 1..5
  --json                  One JSON result on stdout; progress/diagnostics on stderr
  --help                  Show this help
  --version               Show version, source commit, and development status

Configuration: explicit flags override ARCOURT_CASE_URL_TEMPLATE,
ARCOURT_BROWSER, and ARCOURT_OUTPUT for the corresponding options, then defaults.
No other environment variables or config files configure the CLI. There is no
default court URL. Empty explicit values override the environment too.

Exit codes: 0 success; 1 partial/failure (including timeouts and incomplete --all);
2 invalid invocation/selection; 130 cancellation (Ctrl+C).
See docs/command-line-workflow.md for PowerShell examples and JSON fields.
`

type idsFlag []string

func (f *idsFlag) String() string { return strings.Join(*f, ",") }
func (f *idsFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

type options struct {
	command, caseNumber, output string
	ids                         idsFlag
	all, json                   bool
	timeout                     time.Duration
	browser                     arcourt.BrowserFetcherConfig
}

// Keep the CLI's early syntax check aligned with the public fetch contract.
// The shared service independently validates both requested and rendered identity.
var casePattern = regexp.MustCompile(`^\d{2}[A-Z]{0,4}-\d{1,6}-\d{1,6}$`)
var idPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func parseOptions(args []string, getenv func(string) string) (options, error) {
	o := options{}
	// Detect JSON even if parsing stops at an earlier invalid option.
	for _, arg := range args {
		switch arg {
		case "--json", "-json", "--json=true", "-json=true":
			o.json = true
		case "--json=false", "-json=false":
			o.json = false
		}
	}
	if len(args) == 0 {
		return o, errors.New("expected preview or download; use --help")
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		return o, flag.ErrHelp
	}
	o.command = args[0]
	if o.command != "preview" && o.command != "download" {
		return o, errors.New("expected preview or download; use --help")
	}
	f := flag.NewFlagSet(o.command, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.caseNumber, "case", "", "case number")
	f.StringVar(&o.output, "output", getenv("ARCOURT_OUTPUT"), "output directory")
	f.StringVar(&o.browser.CaseURLTemplate, "case-url-template", getenv("ARCOURT_CASE_URL_TEMPLATE"), "case URL template")
	f.StringVar(&o.browser.ExecutablePath, "browser", getenv("ARCOURT_BROWSER"), "browser executable")
	f.BoolVar(&o.browser.Headless, "headless", true, "headless mode")
	f.BoolVar(&o.json, "json", o.json, "JSON output")
	f.BoolVar(&o.all, "all", false, "select all preview entries")
	f.Var(&o.ids, "id", "select document ID (repeatable)")
	f.DurationVar(&o.timeout, "timeout", 10*time.Minute, "whole command timeout")
	f.DurationVar(&o.browser.PageTimeout, "page-timeout", 90*time.Second, "browser page timeout")
	f.DurationVar(&o.browser.DocumentHTTP.AttemptTimeout, "document-timeout", 45*time.Second, "HTTP attempt timeout")
	f.IntVar(&o.browser.MaxDocuments, "max-documents", 200, "document limit")
	f.Int64Var(&o.browser.DocumentHTTP.MaxPDFBytes, "max-pdf-bytes", 64<<20, "PDF byte limit")
	f.IntVar(&o.browser.DocumentHTTP.MaxAttempts, "max-attempts", 3, "HTTP attempts")
	if err := f.Parse(args[1:]); err != nil {
		return o, err
	}
	if f.NArg() != 0 {
		return o, errors.New("unexpected positional argument; all options must follow the command (use --headless=false for headful mode)")
	}
	o.caseNumber = strings.ToUpper(strings.TrimSpace(o.caseNumber))
	if !casePattern.MatchString(o.caseNumber) {
		return o, errors.New("--case requires a supported case number, for example 60CV-2026-1")
	}
	if err := validateTemplate(o.browser.CaseURLTemplate); err != nil {
		return o, err
	}
	for _, limit := range []struct {
		name       string
		value, max time.Duration
	}{
		{"timeout", o.timeout, 24 * time.Hour},
		{"page-timeout", o.browser.PageTimeout, 5 * time.Minute},
		{"document-timeout", o.browser.DocumentHTTP.AttemptTimeout, 5 * time.Minute},
	} {
		if limit.value <= 0 || limit.value > limit.max {
			return o, fmt.Errorf("--%s must be greater than 0 and at most %s", limit.name, limit.max)
		}
	}
	if o.browser.MaxDocuments < 1 || o.browser.MaxDocuments > 10000 {
		return o, errors.New("--max-documents must be between 1 and 10000")
	}
	if o.browser.DocumentHTTP.MaxPDFBytes < 1 || o.browser.DocumentHTTP.MaxPDFBytes > 1<<30 {
		return o, errors.New("--max-pdf-bytes must be between 1 and 1073741824")
	}
	if o.browser.DocumentHTTP.MaxAttempts < 1 || o.browser.DocumentHTTP.MaxAttempts > 5 {
		return o, errors.New("--max-attempts must be between 1 and 5")
	}
	if o.command == "preview" {
		if o.all || len(o.ids) != 0 {
			return o, errors.New("--id and --all are download options")
		}
		var outputSet bool
		f.Visit(func(v *flag.Flag) { outputSet = outputSet || v.Name == "output" })
		if outputSet {
			return o, errors.New("--output is a download option; redirect stdout to save a preview")
		}
		return o, nil
	}
	if o.all == (len(o.ids) > 0) {
		return o, errors.New("download requires either one or more --id values or --all, exclusively")
	}
	seen := map[string]bool{}
	var distinct idsFlag
	for _, id := range o.ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if !idPattern.MatchString(id) {
			return o, errors.New("--id must be a full 64-character hexadecimal document_id from preview")
		}
		if !seen[id] {
			distinct = append(distinct, id)
			seen[id] = true
		}
	}
	o.ids = distinct
	if len(o.ids) > o.browser.MaxDocuments {
		return o, errors.New("selection exceeds --max-documents; select fewer IDs or increase the limit")
	}
	if strings.TrimSpace(o.output) == "" {
		return o, errors.New("--output (or ARCOURT_OUTPUT) must name an existing local directory")
	}
	abs, err := filepath.Abs(o.output)
	if err != nil {
		return o, errors.New("cannot resolve --output directory")
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return o, errors.New("--output must be an existing directory; create it before downloading")
	}
	o.output = abs
	return o, nil
}

func validateTemplate(template string) error {
	if !strings.Contains(template, "{case_number}") {
		return errors.New("--case-url-template (or ARCOURT_CASE_URL_TEMPLATE) must contain {case_number}; no default court URL is supplied")
	}
	u, err := url.Parse(template)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || strings.ContainsAny(u.Host, "{}") {
		return errors.New("case URL template must be an absolute HTTP(S) URL without user credentials")
	}
	return nil
}
