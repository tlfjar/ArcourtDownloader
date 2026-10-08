package arcourt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

var (
	ErrDownloadBusy      = errors.New("a case operation is already active")
	ErrOutputBusy        = errors.New("another download owns this case directory")
	ErrUnsafeDestination = errors.New("unsafe output destination")
	ErrManifest          = errors.New("local manifest could not be read or saved")
)

const (
	DocumentSkipped DocumentStatus = "skipped"
	SkipVerified                   = "verified_existing"
	SkipExplicit                   = "explicit"
	SkipLimit                      = "limit"
)

// DownloadRequest freezes the documents selected from a preview. Select All is
// expressed by copying preview.DocketEntries. Nothing outside Selection is fetched.
// SkipSourceURLs marks selected identities explicitly skipped by the caller.
// OutputDirectory must be an existing absolute local directory.
type DownloadRequest struct {
	CaseNumber      string
	Selection       []DocketEntry
	SkipSourceURLs  []string
	OutputDirectory string
	Naming          *NamingRequest
}

// DocumentID hashes the canonical source identity. URLs, including unknown query
// credentials, are never persisted. This is an identifier, not anonymization.
func DocumentID(sourceURL string) string {
	h := sha256.Sum256([]byte(normalizeSourceURL(sourceURL)))
	return hex.EncodeToString(h[:])
}

// LocalDocumentResult contains no request/source URL. Saved remains true when a
// PDF was published but its manifest update failed. Inspect Err as well as Status.
type LocalDocumentResult struct {
	DocumentID  string         `json:"document_id"`
	Description string         `json:"description"`
	FilingDate  string         `json:"filing_date"`
	Filename    string         `json:"filename,omitempty"`
	Size        int64          `json:"size"`
	SHA256      string         `json:"sha256,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
	Status      DocumentStatus `json:"outcome"`
	SkipReason  string         `json:"skip_reason,omitempty"`
	Saved       bool           `json:"saved"`
	Naming      *NamingOutcome `json:"naming,omitempty"`
	Error       string         `json:"error,omitempty"`
	Err         error          `json:"-"`
}

// DownloadCounts partitions Selected; Unavailable is separate from Failed.
type DownloadCounts struct {
	Selected, Succeeded, Failed, Unavailable, Skipped, Canceled int
}

type LocalDownloadResult struct {
	CaseNumber   string
	Directory    string
	ManifestPath string
	Documents    []LocalDocumentResult
	Counts       DownloadCounts
	Partial      bool
}

type DownloadEventKind string

const (
	DownloadStarted      DownloadEventKind = "started"
	DownloadTransferring DownloadEventKind = "transferring"
	DownloadDocumentDone DownloadEventKind = "document_done"
	DownloadFinished     DownloadEventKind = "finished"
)

// DownloadEvent is best-effort progress. Sends never block; slow subscribers may
// miss any event, including Finished. The returned result is authoritative.
// The caller owns the channel and must not close it until Download returns.
type DownloadEvent struct {
	Kind       DownloadEventKind
	DocumentID string
	Bytes      int64
	Document   LocalDocumentResult
	Counts     DownloadCounts
}

const ManifestVersion = 1
const ManifestFilename = "arcourt-manifest.json"

// DownloadManifest is a versioned local snapshot. Documents retain prior saved
// versions as well as the latest failed/skipped attempt; filenames are basenames.
type DownloadManifest struct {
	Version    int                   `json:"version"`
	CaseNumber string                `json:"case_number"`
	UpdatedAt  time.Time             `json:"updated_at"`
	Documents  []LocalDocumentResult `json:"documents"`
}
