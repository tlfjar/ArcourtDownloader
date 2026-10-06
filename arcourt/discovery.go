package arcourt

import (
	"context"
	"errors"
	"fmt"

	"github.com/chromedp/chromedp"
)

var (
	ErrDocumentsIncomplete = errors.New("one or more selected documents did not succeed")
	ErrIncompleteDiscovery = errors.New("discovery does not establish a complete docket")
	ErrDocumentUnavailable = errors.New("selected document was not discovered")
	ErrDocumentLimit       = errors.New("document skipped by MaxDocuments")
)

type caseDiscovery struct {
	Info     CaseInfo
	Rows     []linkRow
	Coverage DiscoveryCoverage
}

func (f *BrowserFetcher) discover(ctx context.Context, caseNumber string) (caseDiscovery, error) {
	if !looksLikeCaseNumber(caseNumber) {
		return caseDiscovery{}, ErrInvalidCaseNumber
	}
	if err := ctx.Err(); err != nil {
		return caseDiscovery{}, err
	}
	load := f.loadCase
	if load == nil {
		load = f.readCaseDOM
		if endpoint := opadCaseAPI(f.cfg.CaseURLTemplate, normalizeCaseNumber(caseNumber)); endpoint != "" {
			load = func(ctx context.Context, number string) (caseDiscovery, error) {
				return f.readOPADCase(ctx, endpoint, number)
			}
		}
	}
	d, err := load(ctx, normalizeCaseNumber(caseNumber))
	if err != nil {
		return caseDiscovery{}, err
	}
	d.Info = normalizeCaseInfo(d.Info, "")
	if err := validateCaseNumberMatch(d.Info.CaseNumber, caseNumber); err != nil {
		return caseDiscovery{}, err
	}
	d.Info.CaseNumber = normalizeCaseNumber(d.Info.CaseNumber)
	d.Rows = normalizeLinkRows(d.Rows)
	if !hasMeaningfulCaseContent(d.Info, d.Rows) {
		return caseDiscovery{}, buildCaseLoadError(caseNumber, casePageState{})
	}
	d.Coverage.DiscoverableDocuments = len(d.Rows)
	return d, nil
}

func (f *BrowserFetcher) readCaseDOM(ctx context.Context, caseNumber string) (caseDiscovery, error) {
	pageCtx, cleanup, err := f.openCasePage(ctx, caseNumber, f.cfg.PageTimeout)
	if err != nil {
		return caseDiscovery{}, err
	}
	defer cleanup()
	var d caseDiscovery
	err = chromedp.Run(pageCtx,
		chromedp.Evaluate(extractCaseInfoJS, &d.Info),
		chromedp.Evaluate(extractDocketRowsJS, &d.Rows),
		chromedp.Evaluate(inspectDocketCoverageJS, &d.Coverage),
	)
	if err != nil {
		return caseDiscovery{}, fmt.Errorf("extract case docket: %w", err)
	}
	d.Coverage.Limitations = []string{"Only currently rendered document links are discovered; pagination, lazy loading, and virtualized rows are not traversed. Whole-docket completeness and document total are unknown."}
	return d, nil
}

func entryFromRow(row linkRow) DocketEntry {
	return DocketEntry{SourceURL: normalizeSourceURL(row.SourceURL), RequestURL: row.URL,
		DocketDescription: row.Description, FilingDate: row.FilingDate}
}

func (f *BrowserFetcher) documentLimit() int {
	if f.cfg.MaxDocuments > 0 {
		return f.cfg.MaxDocuments
	}
	return 200
}

// PreviewCaseDocuments validates case identity and reports coverage before any
// documents are fetched. MaxDocuments caps the returned entries, not the count.
func (f *BrowserFetcher) PreviewCaseDocuments(ctx context.Context, caseNumber string) (*CasePreview, error) {
	ctx, done, err := f.beginOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	d, err := f.discover(ctx, caseNumber)
	if err != nil {
		return nil, err
	}
	preview := &CasePreview{CaseInfo: d.Info, Discovery: d.Coverage}
	rows := d.Rows
	if len(rows) > f.documentLimit() {
		rows = rows[:f.documentLimit()]
		preview.Discovery.Truncated = true
	}
	for _, row := range rows {
		preview.DocketEntries = append(preview.DocketEntries, entryFromRow(row))
	}
	return preview, nil
}

// DownloadCaseDocuments reloads and validates the case before downloading. Empty
// selections do no browser/network work. Errors never erase earlier outcomes.
// All includes every observed identity, even those subsequently skipped by limit.
func (f *BrowserFetcher) DownloadCaseDocuments(ctx context.Context, caseNumber string, selection DocumentSelection, sink DocumentSink) (*DownloadResult, error) {
	result := &DownloadResult{}
	selected := normalizedStringSlice(selection.SourceURLs)
	if selection.All && len(selection.SourceURLs) != 0 {
		return result, errors.New("All and SourceURLs cannot be combined")
	}
	for _, id := range selected {
		result.Outcomes = append(result.Outcomes, DocumentOutcome{Document: DocketEntry{SourceURL: id}})
	}
	failCase := func(err error) (*DownloadResult, error) {
		for i := range result.Outcomes {
			result.Outcomes[i].Status = DocumentFailed
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				result.Outcomes[i].Status = DocumentCanceled
			}
			result.Outcomes[i].Err = err
		}
		return result, err
	}
	if !looksLikeCaseNumber(caseNumber) {
		return failCase(ErrInvalidCaseNumber)
	}
	ctx, done, err := f.beginOperation(ctx)
	if err != nil {
		return failCase(err)
	}
	defer done()
	if err := ctx.Err(); err != nil {
		return failCase(err)
	}
	if !selection.All && len(selected) == 0 {
		return result, nil
	}
	if sink == nil {
		return failCase(errors.New("document sink is required"))
	}
	d, err := f.discover(ctx, caseNumber)
	if err != nil {
		return failCase(err)
	}
	result.CaseInfo, result.Discovery = d.Info, d.Coverage
	entries := make(map[string]DocketEntry, len(d.Rows))
	for _, row := range d.Rows {
		entry := entryFromRow(row)
		entries[entry.SourceURL] = entry
		if selection.All {
			result.Outcomes = append(result.Outcomes, DocumentOutcome{Document: entry})
		}
	}
	client := newDocumentClient(f.cfg.DocumentHTTP)
	defer client.http.CloseIdleConnections()
	var batchErr error
	if selection.All && !d.Coverage.Complete {
		batchErr = ErrIncompleteDiscovery
	}
	attempted := 0
	failed := false
	for i := range result.Outcomes {
		out := &result.Outcomes[i]
		entry, found := entries[out.Document.SourceURL]
		if found {
			out.Document = entry
		}
		switch {
		case ctx.Err() != nil:
			out.Status, out.Err = DocumentCanceled, ctx.Err()
		case !found:
			out.Status, out.Err = DocumentUnavailable, ErrDocumentUnavailable
		case attempted >= f.documentLimit():
			out.Status, out.Err = DocumentSkippedLimit, ErrDocumentLimit
		default:
			attempted++
			*out = client.download(ctx, entry, sink)
		}
		if out.Status != DocumentSucceeded {
			failed = true
		}
	}
	if failed {
		batchErr = errors.Join(batchErr, ErrDocumentsIncomplete)
	}
	if ctx.Err() != nil {
		batchErr = errors.Join(batchErr, ctx.Err())
	}
	return result, batchErr
}

// These are diagnostic signals only. Their absence does not establish that all
// rows have been rendered. ARIA row totals must not be counted as PDF totals.
const inspectDocketCoverageJS = `(() => {
	const next = Array.from(document.querySelectorAll('button, a')).some(el => {
		const label = (el.getAttribute('aria-label') || el.textContent || '').trim();
		return /next(?:\s+page)?/i.test(label) && !el.disabled && el.getAttribute('aria-disabled') !== 'true';
	});
	const grids = Array.from(document.querySelectorAll('[aria-rowcount]'));
	const counts = grids.map(el => Number(el.getAttribute('aria-rowcount'))).filter(n => Number.isInteger(n) && n >= 0);
	return {
		paginationDetected: next || Boolean(document.querySelector('.MuiTablePagination-root, [aria-label*="pagination" i]')),
		virtualizationDetected: Boolean(document.querySelector('.MuiDataGrid-virtualScroller, [data-virtualized]')) || grids.some(el => Number(el.getAttribute('aria-rowcount')) > el.querySelectorAll('[role="row"]').length),
		reportedDocketRows: counts.length ? Math.max(...counts) : null
	};
})()`
