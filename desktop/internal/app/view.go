package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
)

// Only these view types cross the binding. Source/request URLs and raw errors
// stay in Go. All accounting and document identity come from the shared service.
type Document struct {
	ID          string `json:"id"`
	Date        string `json:"date"`
	Description string `json:"description"`
	Selected    bool   `json:"selected"`
}

type Preview struct {
	Number    string     `json:"number"`
	Title     string     `json:"title"`
	County    string     `json:"county"`
	Judge     string     `json:"judge"`
	Parties   []string   `json:"parties"`
	Documents []Document `json:"documents"`
	Warnings  []string   `json:"warnings"`
}

type Result struct {
	Directory string                 `json:"directory"`
	Documents []ResultDocument       `json:"documents"`
	Counts    arcourt.DownloadCounts `json:"counts"`
	Partial   bool                   `json:"partial"`
}

type ResultDocument struct {
	DocumentID  string `json:"document_id"`
	Description string `json:"description"`
	FilingDate  string `json:"filing_date"`
	Filename    string `json:"filename"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Status      string `json:"outcome"`
	SkipReason  string `json:"skip_reason"`
	Saved       bool   `json:"saved"`
	Error       string `json:"error"`
}

type State struct {
	Revision    uint64                 `json:"revision"`
	Generation  uint64                 `json:"generation"`
	CaseNumber  string                 `json:"caseNumber"`
	Busy        bool                   `json:"busy"`
	Canceling   bool                   `json:"canceling"`
	Closing     bool                   `json:"closing"`
	Phase       string                 `json:"phase"`
	Message     string                 `json:"message"`
	Diagnostic  string                 `json:"diagnostic"`
	Browser     string                 `json:"browser"`
	Preview     *Preview               `json:"preview"`
	Verified    bool                   `json:"verified"`
	Selected    int                    `json:"selected"`
	All         bool                   `json:"all"`
	Progress    arcourt.DownloadCounts `json:"progress"`
	Bytes       int64                  `json:"bytes"`
	Result      *Result                `json:"result"`
	Preferences Preferences            `json:"preferences"`
}

var linkPattern = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<>"']+`)

func displayText(s string) string { return linkPattern.ReplaceAllString(s, "[link omitted]") }

func previewView(p *arcourt.CasePreview) *Preview {
	c := p.CaseInfo
	v := &Preview{Number: c.CaseNumber, Title: displayText(c.CaseTitle), County: displayText(c.County), Judge: displayText(c.Judge), Documents: []Document{}, Parties: []string{}, Warnings: []string{}}
	for _, party := range c.Parties {
		v.Parties = append(v.Parties, displayText(party.Name+" — "+party.Role))
	}
	seen := map[string]bool{}
	for _, d := range p.DocketEntries {
		id := arcourt.DocumentID(d.SourceURL)
		if d.SourceURL != "" && !seen[id] {
			seen[id] = true
			v.Documents = append(v.Documents, Document{ID: id, Date: displayText(d.FilingDate), Description: displayText(d.DocketDescription)})
		}
	}
	if !p.Discovery.Complete {
		v.Warnings = append(v.Warnings, "Coverage note: This preview lists documents available from the court site. The app cannot check for sealed, withheld, or other records the site does not provide. This does not indicate a download failure.")
	}
	if p.Discovery.Truncated {
		v.Warnings = append(v.Warnings, "Preview truncated at the document limit. Some observed documents are omitted.")
	}
	if p.Discovery.PaginationDetected || p.Discovery.VirtualizationDetected {
		v.Warnings = append(v.Warnings, "The page uses pagination or virtualized rows; additional documents may exist.")
	}
	return v
}

func resultView(r *arcourt.LocalDownloadResult) *Result {
	if r == nil {
		return nil
	}
	v := &Result{Directory: r.Directory, Counts: r.Counts, Partial: r.Partial, Documents: []ResultDocument{}}
	for _, d := range r.Documents {
		message := operationError(d.Err)
		if message == "" && d.Error != "" {
			message = "Document failed. Inspect output permissions and retry."
		}
		v.Documents = append(v.Documents, ResultDocument{
			DocumentID: d.DocumentID, Description: displayText(d.Description), FilingDate: displayText(d.FilingDate),
			Filename: d.Filename, SHA256: d.SHA256, Size: d.Size, Status: string(d.Status), SkipReason: d.SkipReason, Saved: d.Saved, Error: message,
		})
	}
	return v
}

func outcome(r *arcourt.LocalDownloadResult, err error, allCoverage *arcourt.DiscoveryCoverage) (string, string) {
	if errors.Is(err, context.Canceled) {
		return "canceled", operationError(err)
	}
	if r == nil {
		if err == nil {
			return "error", "No result was returned. Preview the case again before retrying."
		}
		return "error", operationError(err)
	}
	c := r.Counts
	if err != nil || r.Partial || c.Failed+c.Unavailable+c.Canceled > 0 || c.Selected == 0 || c.Succeeded+c.Skipped != c.Selected {
		message := operationError(err)
		if message == "" {
			message = "Some selected documents did not complete. Inspect the document results and retry as needed."
		}
		return "partial", message
	}
	for _, d := range r.Documents {
		if d.Err != nil || d.SkipReason == arcourt.SkipLimit {
			return "partial", "Some documents were not saved. Inspect results before retrying."
		}
	}
	message := fmt.Sprintf("Download complete: %d saved; %d already downloaded and verified.", c.Succeeded, c.Skipped)
	// A successful selection does not prove whole-docket coverage. Keep that
	// scope note separate from evidence of omitted documents or transfer errors.
	if allCoverage != nil {
		if allCoverage.Truncated {
			return "partial", message + " Preview limit reached: additional documents were omitted. Select All included only the documents shown."
		}
		if allCoverage.PaginationDetected || allCoverage.VirtualizationDetected {
			return "partial", message + " The court page may contain additional documents on other pages or in unloaded rows. Select All included only the documents shown."
		}
		if !allCoverage.Complete {
			message += " Select All included every document in this preview. Records the court does not make available online cannot be checked."
		}
	}
	return "success", message
}

func operationError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "Canceled. Completed PDFs remain saved; you can retry the selection."
	case errors.Is(err, context.DeadlineExceeded):
		return "Operation timed out. Check the connection and retry; saved PDFs remain available."
	case errors.Is(err, arcourt.ErrInvalidCaseNumber):
		return "Enter a valid case number, for example 60CV-2026-1."
	case errors.Is(err, arcourt.ErrCaseNumberMismatch):
		return "Case header does not match. Check the number and configured case page template."
	case errors.Is(err, arcourt.ErrUnsafeDestination):
		return "Choose an existing local output folder without links or reparse points."
	case errors.Is(err, os.ErrPermission):
		return "Output is not writable. Choose a folder you own and check file permissions."
	case errors.Is(err, arcourt.ErrOutputBusy), errors.Is(err, arcourt.ErrDownloadBusy):
		return "Another operation owns this case or output folder. Wait for it to finish."
	case errors.Is(err, arcourt.ErrManifest):
		return "Manifest could not be read or saved. Inspect existing PDFs and folder permissions before retrying."
	case errors.Is(err, arcourt.ErrDocumentUnavailable):
		return "Document is no longer available. Refresh the preview and check the court page."
	case errors.Is(err, arcourt.ErrDocumentTooLarge):
		return "Document exceeds the download size limit. Use the court page to inspect it."
	case errors.Is(err, arcourt.ErrInvalidPDF):
		return "Response was not a complete PDF. Check document availability and retry."
	case errors.Is(err, arcourt.ErrDocumentsIncomplete):
		return "Some selected documents did not complete. Inspect the results and retry as needed."
	case errors.Is(err, arcourt.ErrBackendUnavailable):
		return "Court service is unavailable. Try again later."
	case errors.Is(err, arcourt.ErrCaseAccessDenied):
		return "Court denied case access (401/403). Open the court page to check access or try again later."
	case errors.Is(err, arcourt.ErrCaseLoadFailed):
		return "Case page did not load. Check the case number, page template, connection, and browser."
	default:
		return "Operation failed. Check browser installation, case page template, connection, and output permissions."
	}
}
