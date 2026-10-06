package arcourt

import (
	"context"
	"io"
)

// CaseInfo contains header metadata parsed from an Arkansas court case page.
type CaseInfo struct {
	CaseNumber string
	CaseTitle  string
	County     string
	Judge      string
	Parties    []Party
}

// Party describes a named case participant from the court page.
type Party struct {
	Name string
	Role string
}

// DocketEntry describes a visible docket row from the court page.
type DocketEntry struct {
	// SourceURL is the canonical selection identity, never a download target.
	SourceURL string
	// RequestURL preserves the extracted URL, including signed query order.
	RequestURL        string `json:"-"`
	DocketDescription string
	FilingDate        string
}

// CasePreview contains case metadata and docket rows before download.
type CasePreview struct {
	CaseInfo      CaseInfo
	DocketEntries []DocketEntry
	Discovery     DiscoveryCoverage
}

// DiscoveryCoverage describes what was observed, not an inferred complete docket.
type DiscoveryCoverage struct {
	DiscoverableDocuments  int  // distinct documents observed before MaxDocuments
	TotalDocuments         *int // whole-docket document count, nil when unknown
	Truncated              bool // MaxDocuments omitted observed documents from the preview
	Complete               bool // false: neither public API nor DOM proves whole-docket completeness
	PaginationDetected     bool
	VirtualizationDetected bool
	ReportedDocketRows     *int // UI or API docket row count, not a document count
	Limitations            []string
}

// DocumentSelection selects canonical SourceURLs. Its zero value selects nothing.
// All explicitly selects every document discovered on a fresh case load; it must
// not be combined with SourceURLs. Unknown docket completeness remains an error.
type DocumentSelection struct {
	SourceURLs []string
	All        bool
}

// DocumentWriter stages one attempt. Commit publishes validated bytes; Abort
// discards that attempt, including after a failed Commit. Both release resources.
// Implementations must honor the context passed to OpenDocument and bound writes.
type DocumentWriter interface {
	io.Writer
	Commit() error
	Abort() error
}

// DocumentSink opens a fresh, empty writer for each attempt (including retries).
// On an OpenDocument error the sink must release any resources it acquired.
type DocumentSink interface {
	OpenDocument(context.Context, DocketEntry) (DocumentWriter, error)
}

type DocumentStatus string

const (
	DocumentSucceeded    DocumentStatus = "succeeded"
	DocumentFailed       DocumentStatus = "failed"
	DocumentUnavailable  DocumentStatus = "unavailable"
	DocumentSkippedLimit DocumentStatus = "skipped_limit"
	DocumentCanceled     DocumentStatus = "canceled"
)

// DocumentOutcome is terminal for one distinct selected identity. Bytes and
// SHA256 describe committed content only. Transfer error messages omit URLs;
// callers must not log RequestURL or unwrapped transport errors.
type DocumentOutcome struct {
	Document DocketEntry
	Status   DocumentStatus
	Bytes    int64
	SHA256   string
	Attempts int
	Err      error
}

// DownloadResult retains successes even when the accompanying error is non-nil.
type DownloadResult struct {
	CaseInfo  CaseInfo
	Discovery DiscoveryCoverage
	Outcomes  []DocumentOutcome
}
