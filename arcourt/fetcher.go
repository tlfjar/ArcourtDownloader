package arcourt

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// ErrCaseNumberMismatch indicates that the rendered case differs from the request.
var ErrCaseNumberMismatch = errors.New("case number mismatch")

var ErrInvalidCaseNumber = errors.New("invalid case number")

var caseNumberRegex = regexp.MustCompile(`(?i)^\d{2}[a-z]{0,4}-\d{1,6}-\d{1,6}$`)

const maxCasePageLoadAttempts = 3

type casePageState struct {
	Title          string `json:"title"`
	Loading        bool   `json:"loading"`
	HasCaseContent bool   `json:"hasCaseContent"`
	ErrorHeading   string `json:"errorHeading"`
	ErrorDetail    string `json:"errorDetail"`
	AccessDenied   bool   `json:"accessDenied"`
}

// CaseFetcher fetches docket PDFs for a given Arkansas case number.
type CaseFetcher interface {
	DownloadCaseDocuments(ctx context.Context, caseNumber string, selection DocumentSelection, sink DocumentSink) (*DownloadResult, error)
	PreviewCaseDocuments(ctx context.Context, caseNumber string) (*CasePreview, error)
}

// BrowserFetcherConfig configures browser-backed Arkansas court document fetching.
type BrowserFetcherConfig struct {
	CaseURLTemplate string
	// ExecutablePath is an unquoted file path; empty enables automatic discovery.
	ExecutablePath string
	Headless       bool
	PageTimeout    time.Duration
	// MaxDocuments caps preview entries and attempted documents per download.
	// Missing selections do not consume the limit; failed downloads do.
	MaxDocuments int
	DocumentHTTP DocumentHTTPConfig
}

// BrowserFetcher fetches Arkansas court case pages and docket PDFs through a browser session.
type BrowserFetcher struct {
	cfg            BrowserFetcherConfig
	browser        BrowserInfo
	launch         browserLauncher
	shutdown       context.Context
	cancelShutdown context.CancelFunc
	mu             sync.Mutex
	closed         bool
	active         sync.WaitGroup
	cleanupErr     error
	loadCase       func(context.Context, string) (caseDiscovery, error)
}

// NewBrowserFetcher resolves an installed browser without launching it.
// The caller must Close the fetcher during application shutdown.
func NewBrowserFetcher(cfg BrowserFetcherConfig) (*BrowserFetcher, error) {
	if strings.TrimSpace(cfg.CaseURLTemplate) == "" {
		return nil, errors.New("ARCOURT_CASE_URL_TEMPLATE is required")
	}
	if cfg.PageTimeout <= 0 {
		cfg.PageTimeout = 90 * time.Second
	}
	if cfg.MaxDocuments <= 0 {
		cfg.MaxDocuments = 200
	}
	return newBrowserFetcher(cfg, hostBrowserResolver(), launchChromedp)
}

func (f *BrowserFetcher) openCasePage(ctx context.Context, caseNumber string, timeout time.Duration) (context.Context, func(), error) {
	var lastErr error
	for attempt := 1; attempt <= maxCasePageLoadAttempts; attempt++ {
		pageCtx, cleanup, err := f.openCasePageOnce(ctx, caseNumber, timeout)
		if err == nil {
			return pageCtx, cleanup, nil
		}
		lastErr = err
		if attempt == maxCasePageLoadAttempts || !shouldRetryCasePageLoad(err) {
			return nil, nil, err
		}

		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
		}
	}

	return nil, nil, lastErr
}

func (f *BrowserFetcher) openCasePageOnce(ctx context.Context, caseNumber string, timeout time.Duration) (context.Context, func(), error) {
	if timeout <= 0 {
		timeout = f.cfg.PageTimeout
	}

	caseURL := strings.ReplaceAll(f.cfg.CaseURLTemplate, "{case_number}", url.QueryEscape(caseNumber))
	entryURL := caseAppEntryURL(caseURL)

	pageCtx, cleanup, err := f.openBrowser(ctx, timeout)
	if err != nil {
		return nil, nil, err
	}

	actions := make([]chromedp.Action, 0, 8)
	if entryURL != "" && entryURL != caseURL {
		actions = append(
			actions,
			chromedp.Navigate(entryURL),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Sleep(500*time.Millisecond),
			acceptDisclaimerAction(),
			chromedp.Sleep(300*time.Millisecond),
		)
	}
	actions = append(
		actions,
		chromedp.Navigate(caseURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(1500*time.Millisecond),
		acceptDisclaimerAction(),
		chromedp.Sleep(1200*time.Millisecond),
	)

	if err := chromedp.Run(pageCtx, actions...); err != nil {
		cleanup()
		return nil, nil, classifyOpenCasePageError(caseNumber, caseURL, err)
	}

	if err := waitForCasePageReady(pageCtx, caseNumber); err != nil {
		cleanup()
		return nil, nil, err
	}

	return pageCtx, cleanup, nil
}

func caseAppEntryURL(caseURL string) string {
	parsed, err := url.Parse(caseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	if idx := strings.Index(parsed.Path, "/case/"); idx >= 0 {
		parsed.Path = strings.TrimRight(parsed.Path[:idx], "/")
		if parsed.Path == "" {
			parsed.Path = "/"
		}
	} else {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) > 0 && parts[0] != "" {
			parsed.Path = "/" + parts[0]
		} else {
			parsed.Path = "/"
		}
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func acceptDisclaimerAction() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		var clicked bool
		_ = chromedp.Evaluate(disclaimerClickJS, &clicked).Do(ctx)
		return nil
	})
}

func shouldRetryCasePageLoad(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, ErrCaseNumberMismatch) {
		return false
	}
	if errors.Is(err, ErrBackendUnavailable) {
		return true
	}
	if !errors.Is(err, ErrCaseLoadFailed) {
		return false
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "case not found") {
		return false
	}

	return strings.Contains(msg, "unexpected token '<'") ||
		strings.Contains(msg, "response returned an error code") ||
		strings.Contains(msg, "unexpected problem occurred") ||
		strings.Contains(msg, "timed out waiting") ||
		strings.Contains(msg, "without case details or docket entries")
}

func hasMeaningfulCaseContent(info CaseInfo, rows []linkRow) bool {
	return info.CaseTitle != "" || info.County != "" || info.Judge != "" || len(info.Parties) > 0 || len(rows) > 0
}

func classifyOpenCasePageError(caseNumber, caseURL string, err error) error {
	if err == nil {
		return nil
	}

	host := caseURL
	if parsed, parseErr := url.Parse(caseURL); parseErr == nil && parsed.Host != "" {
		host = parsed.Host
	}

	msg := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: timed out while loading case %q from %s", ErrBackendUnavailable, caseNumber, host)
	case errors.Is(err, context.Canceled):
		return err
	case strings.Contains(msg, "net::err_") || strings.Contains(msg, "dial tcp") || strings.Contains(msg, "lookup ") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "tls"):
		return fmt.Errorf("%w: cannot reach %s while loading case %q: %w", ErrBackendUnavailable, host, caseNumber, err)
	default:
		return fmt.Errorf("open case page: %w", err)
	}
}

func waitForCasePageReady(ctx context.Context, caseNumber string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var last casePageState
	for {
		if err := acceptDisclaimerAction().Do(ctx); err != nil {
			return err
		}

		state, err := readCasePageState(ctx)
		if err != nil {
			return fmt.Errorf("inspect case page: %w", err)
		}
		last = state

		if state.AccessDenied {
			return ErrCaseAccessDenied
		}
		if state.HasCaseContent {
			return nil
		}
		if summary := state.errorSummary(); summary != "" {
			return buildCaseLoadError(caseNumber, state)
		}
		if !state.Loading {
			return buildCaseLoadError(caseNumber, state)
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return buildCaseLoadTimeoutError(caseNumber, last)
		case <-ticker.C:
		}
	}
}

func readCasePageState(ctx context.Context) (casePageState, error) {
	var state casePageState
	if err := chromedp.Run(ctx, chromedp.Evaluate(inspectCasePageStateJS, &state)); err != nil {
		return casePageState{}, err
	}
	return state, nil
}

func buildCaseLoadError(caseNumber string, state casePageState) error {
	if summary := state.errorSummary(); summary != "" {
		return fmt.Errorf("%w: Arkansas Judiciary returned an error while loading case %q: %s", ErrCaseLoadFailed, caseNumber, summary)
	}
	return fmt.Errorf("%w: Arkansas Judiciary loaded case %q without case details or docket entries", ErrCaseLoadFailed, caseNumber)
}

func buildCaseLoadTimeoutError(caseNumber string, state casePageState) error {
	if summary := state.errorSummary(); summary != "" {
		return buildCaseLoadError(caseNumber, state)
	}
	return fmt.Errorf("%w: timed out waiting for Arkansas Judiciary to load case %q", ErrCaseLoadFailed, caseNumber)
}

func (s casePageState) errorSummary() string {
	parts := make([]string, 0, 2)
	heading := strings.TrimSpace(s.ErrorHeading)
	detail := strings.TrimSpace(s.ErrorDetail)
	title := strings.TrimSpace(s.Title)

	if heading != "" {
		parts = append(parts, heading)
	} else if strings.Contains(strings.ToLower(title), "error") {
		parts = append(parts, title)
	}
	if detail != "" && !strings.EqualFold(detail, heading) {
		parts = append(parts, detail)
	}

	return strings.Join(parts, ": ")
}

func normalizeCaseInfo(info CaseInfo, fallbackCaseNumber string) CaseInfo {
	fallbackCaseNumber = strings.TrimSpace(fallbackCaseNumber)
	info.CaseNumber = strings.TrimSpace(info.CaseNumber)
	if info.CaseNumber == "" {
		info.CaseNumber = fallbackCaseNumber
	}
	info.CaseTitle = strings.TrimSpace(info.CaseTitle)
	info.County = strings.TrimSpace(info.County)
	info.Judge = strings.TrimSpace(info.Judge)

	parties := make([]Party, 0, len(info.Parties))
	for _, party := range info.Parties {
		name := strings.TrimSpace(party.Name)
		role := strings.TrimSpace(party.Role)
		if name == "" && role == "" {
			continue
		}
		parties = append(parties, Party{Name: name, Role: role})
	}
	info.Parties = parties

	return info
}

func validateCaseNumberMatch(extractedCaseNumber, requestedCaseNumber string) error {
	if !looksLikeCaseNumber(requestedCaseNumber) {
		return ErrInvalidCaseNumber
	}
	if !looksLikeCaseNumber(extractedCaseNumber) || normalizeCaseNumber(extractedCaseNumber) != normalizeCaseNumber(requestedCaseNumber) {
		return ErrCaseNumberMismatch
	}
	return nil
}

func looksLikeCaseNumber(value string) bool {
	return caseNumberRegex.MatchString(strings.TrimSpace(value))
}

const disclaimerClickJS = `(function () {
	const lower = (value) => (value || '').toLowerCase().replace(/\s+/g, ' ').trim();
	const roots = Array.from(document.querySelectorAll('[role="dialog"], [aria-modal="true"], .MuiDialog-root'));
	const scopes = roots.length > 0 ? roots : [document];
	const wants = ['accept', 'agree', 'continue'];
	const matches = (text) => wants.some((token) => text === token || text === 'i ' + token || text.startsWith(token + ' '));
	const isVisible = (element) => Boolean(element.offsetWidth || element.offsetHeight || element.getClientRects().length);

	for (const scope of scopes) {
		const buttons = Array.from(scope.querySelectorAll('button, input[type="button"], input[type="submit"], a'));
		for (const element of buttons) {
			if (!isVisible(element)) continue;
			const text = lower((element.innerText || element.value || element.getAttribute('aria-label') || '').trim());
			if (matches(text)) {
				element.click();
				return true;
			}
		}
	}
	return false;
})();`

const inspectCasePageStateJS = `(() => {
	const clean = (value) => (value || '').replace(/\s+/g, ' ').trim();
	const bodyText = document.body?.innerText || '';
	const lines = bodyText
		.split(/\n+/)
		.map(clean)
		.filter(Boolean);
	const title = clean(document.title || '');
	const hasProgress = Boolean(document.querySelector('[role="progressbar"]'));
	const accessDenied = /\b(?:403\s+forbidden|401\s+unauthorized|access denied)\b/i.test(title + '\n' + lines.slice(0, 5).join('\n'));
	const hasCaseContent = lines.includes('Case Summary') && lines.includes('Docket Entries');
	const errorHeading = lines.find((line) => /something went wrong|case not found|not found/i.test(line)) || '';
	const errorDetail = lines.find((line) => /unexpected problem occurred|response returned an error code|unable to|failed to load/i.test(line)) || '';
	const loading = !hasCaseContent && !errorHeading && ((/case loading/i.test(title)) || hasProgress);

	return { title, loading, hasCaseContent, errorHeading, errorDetail, accessDenied };
})();`

const extractCaseInfoJS = `(() => {
	const clean = (value) => (value || '').replace(/\s+/g, ' ').trim();
	const toKey = (value) => clean(value).toLowerCase().replace(/\s*[:：]\s*$/, '');

	const findBySelectors = (selectors) => {
		for (const selector of selectors) {
			const el = document.querySelector(selector);
			const value = clean(el?.textContent || '');
			if (value) return value;
		}
		return '';
	};

	const pairs = [];
	document.querySelectorAll('dt').forEach((dt) => {
		pairs.push([toKey(dt.textContent), clean(dt.nextElementSibling?.textContent || '')]);
	});
	document.querySelectorAll('tr').forEach((tr) => {
		const cells = Array.from(tr.querySelectorAll('th,td'));
		if (cells.length >= 2) {
			pairs.push([toKey(cells[0].innerText), clean(cells[1].innerText)]);
		}
	});
	document.querySelectorAll('div,span,p,li').forEach((node) => {
		const text = clean(node.innerText || '');
		if (!text || text.length > 200) return;
		const idx = text.indexOf(':');
		if (idx <= 0 || idx > 40) return;
		pairs.push([toKey(text.slice(0, idx)), clean(text.slice(idx + 1))]);
	});

	const findLabelValue = (labels) => {
		const wanted = labels.map((label) => label.toLowerCase());
		for (const [label, value] of pairs) {
			if (!value) continue;
			if (wanted.some((w) => label === w || label.startsWith(w))) {
				return value;
			}
		}
		return '';
	};

	const caseTitle = findBySelectors([
		'[data-testid="case-title"]',
		'.case-title',
		'.case-header .title',
		'h1',
		'h2'
	]);
	const caseNumberRe = /\b\d{2}[A-Za-z]{0,4}-\d{1,6}-\d{1,6}\b/;
	const explicitCaseNumberRaw = findLabelValue(['case number', 'case no', 'case no.', 'case #']);
	const explicitCaseNumber = clean(explicitCaseNumberRaw);
	const bodyCaseNumberMatch = clean(document.body?.innerText || '').match(caseNumberRe);
	const bodyCaseNumber = bodyCaseNumberMatch ? bodyCaseNumberMatch[0] : '';
	const county = findLabelValue(['county']);
	const judge = findLabelValue(['judge', 'assigned judge']);

	const roleTokens = ['plaintiff', 'defendant', 'petitioner', 'respondent', 'appellant', 'appellee', 'movant'];
	const parties = [];
	const seenParties = new Set();

	const appendParty = (name, role) => {
		const n = clean(name);
		const r = clean(role);
		if (!n && !r) return;
		const key = (n + '|' + r).toLowerCase();
		if (seenParties.has(key)) return;
		seenParties.add(key);
		parties.push({ name: n, role: r });
	};

	document.querySelectorAll('tr').forEach((row) => {
		const cells = Array.from(row.querySelectorAll('td'));
		if (cells.length < 2) return;
		const first = clean(cells[0].innerText);
		const second = clean(cells[1].innerText);
		if (!first || !second) return;
		if (roleTokens.some((token) => second.toLowerCase().includes(token))) {
			appendParty(first, second);
		}
	});

	if (parties.length === 0) {
		document.querySelectorAll('li,div,p').forEach((node) => {
			const text = clean(node.innerText || '');
			if (!text || text.length > 240) return;
			const idx = text.indexOf(':');
			if (idx <= 0 || idx > 60) return;
			const role = clean(text.slice(0, idx));
			const name = clean(text.slice(idx + 1));
			if (!role || !name) return;
			if (roleTokens.some((token) => role.toLowerCase().includes(token))) {
				appendParty(name, role);
			}
		});
	}

	const caseNumber = clean(explicitCaseNumber || bodyCaseNumber);

	return { caseNumber, caseTitle, county, judge, parties };
})()`

const extractDocketRowsJS = `(() => {
	const clean = (value) => (value || '').replace(/\s+/g, ' ').trim();
	const dateRe = /\b\d{1,2}\/\d{1,2}\/\d{2,4}\b/;

	const rows = [];
	const tableRows = Array.from(document.querySelectorAll('tr'));
	for (const rowEl of tableRows) {
		const cells = Array.from(rowEl.querySelectorAll('td'));
		if (cells.length === 0) continue;

		const description = clean(cells[1]?.innerText || cells[0]?.innerText || '');
		let filingDate = '';
		for (const cell of cells) {
			const text = clean(cell.innerText || '');
			if (!text) continue;
			const match = text.match(dateRe);
			if (match && match[0]) {
				filingDate = match[0];
				break;
			}
		}
		if (!filingDate && cells[0]) {
			filingDate = clean(cells[0].innerText || '');
		}

		const anchors = Array.from(rowEl.querySelectorAll('a[href]'));
		for (const anchor of anchors) {
			const href = (anchor.getAttribute('href') || '').trim();
			if (!href) continue;

			const text = clean(anchor.innerText || '').toLowerCase();
			const lowerHref = href.toLowerCase();
			const isDocumentLink = lowerHref.includes('.pdf') || lowerHref.includes('/documents/') || text.includes('pdf') || text.includes('document');
			if (!isDocumentLink) continue;

			const absolute = new URL(href, window.location.href).toString();
			rows.push({ url: absolute, sourceUrl: absolute, description, filingDate });
		}

		const buttons = Array.from(rowEl.querySelectorAll('button[title]'));
		for (const button of buttons) {
			const title = clean(button.getAttribute('title') || '');
			if (!title) continue;

			if (!title.toLowerCase().endsWith('.pdf')) continue;
			const token = title.replace(/\.pdf$/i, '').trim();
			if (!token) continue;

			const apiURL = new URL('/opad/api/documents/' + encodeURIComponent(token), window.location.origin).toString();
			rows.push({ url: apiURL, sourceUrl: apiURL, description, filingDate });
		}
	}
	return rows;
})()`
