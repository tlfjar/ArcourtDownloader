package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
)

// CLI DTOs deliberately omit all source/request URLs and underlying errors.
type response struct {
	Command  string          `json:"command,omitempty"`
	Status   string          `json:"status"`
	ExitCode int             `json:"exit_code"`
	Error    string          `json:"error,omitempty"`
	Warnings []string        `json:"warnings,omitempty"`
	Preview  *previewOutput  `json:"preview,omitempty"`
	Download *downloadOutput `json:"download,omitempty"`
	Help     string          `json:"help,omitempty"`
}

type caseOutput struct {
	Number  string        `json:"case_number"`
	Title   string        `json:"title"`
	County  string        `json:"county"`
	Judge   string        `json:"judge"`
	Parties []partyOutput `json:"parties"`
}

type partyOutput struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type documentOutput struct {
	ID          string `json:"document_id"`
	Date        string `json:"filing_date"`
	Description string `json:"description"`
}

type discoveryOutput struct {
	Discoverable   int      `json:"discoverable_documents"`
	Total          *int     `json:"total_documents"`
	Truncated      bool     `json:"truncated"`
	Complete       bool     `json:"complete"`
	Pagination     bool     `json:"pagination_detected"`
	Virtualization bool     `json:"virtualization_detected"`
	ReportedRows   *int     `json:"reported_docket_rows"`
	Limitations    []string `json:"limitations"`
}

type previewOutput struct {
	Case      caseOutput       `json:"case"`
	Documents []documentOutput `json:"documents"`
	Discovery discoveryOutput  `json:"discovery"`
}

type countsOutput struct {
	Selected    int `json:"selected"`
	Succeeded   int `json:"succeeded"`
	Failed      int `json:"failed"`
	Unavailable int `json:"unavailable"`
	Skipped     int `json:"skipped"`
	Canceled    int `json:"canceled"`
}

type downloadOutput struct {
	CaseNumber   string                        `json:"case_number"`
	Directory    string                        `json:"directory"`
	ManifestPath string                        `json:"manifest_path"`
	Documents    []arcourt.LocalDocumentResult `json:"documents"`
	Counts       countsOutput                  `json:"counts"`
	Partial      bool                          `json:"partial"`
}

func publicPreview(p *arcourt.CasePreview) *previewOutput {
	c, d := p.CaseInfo, p.Discovery
	out := &previewOutput{
		Case:      caseOutput{Number: c.CaseNumber, Title: c.CaseTitle, County: c.County, Judge: c.Judge, Parties: []partyOutput{}},
		Documents: []documentOutput{},
		Discovery: discoveryOutput{d.DiscoverableDocuments, d.TotalDocuments, d.Truncated, d.Complete, d.PaginationDetected, d.VirtualizationDetected, d.ReportedDocketRows, append([]string{}, d.Limitations...)},
	}
	for _, p := range c.Parties {
		out.Case.Parties = append(out.Case.Parties, partyOutput{p.Name, p.Role})
	}
	seen := map[string]bool{}
	for _, e := range p.DocketEntries {
		if e.SourceURL == "" {
			continue
		}
		id := arcourt.DocumentID(e.SourceURL)
		if !seen[id] {
			out.Documents = append(out.Documents, documentOutput{id, e.FilingDate, e.DocketDescription})
			seen[id] = true
		}
	}
	return out
}

func publicDownload(r *arcourt.LocalDownloadResult) *downloadOutput {
	c := r.Counts
	return &downloadOutput{r.CaseNumber, r.Directory, r.ManifestPath, r.Documents,
		countsOutput{c.Selected, c.Succeeded, c.Failed, c.Unavailable, c.Skipped, c.Canceled}, r.Partial}
}

func writeResponse(w io.Writer, jsonMode bool, r response) error {
	if jsonMode {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(r)
	}
	if r.Help != "" {
		_, err := io.WriteString(w, r.Help)
		return err
	}
	// Build once so output failures are reported even for the human-readable form.
	var buf []byte
	if r.Preview != nil {
		p := r.Preview
		buf = fmt.Appendf(buf, "Case %s: %q\nCounty: %q; Judge: %q\nDiscovered documents: %d (observed before limit: %d)\n",
			p.Case.Number, p.Case.Title, p.Case.County, p.Case.Judge, len(p.Documents), p.Discovery.Discoverable)
		for _, d := range p.Documents {
			buf = fmt.Appendf(buf, "%s  %q  %q\n", d.ID, d.Date, d.Description)
		}
	}
	if r.Download != nil {
		d := r.Download
		for _, doc := range d.Documents {
			buf = fmt.Appendf(buf, "%s  %s  %q  %q\n", doc.DocumentID, doc.Status, doc.Filename, doc.Error)
		}
		c := d.Counts
		buf = fmt.Appendf(buf, "Selected %d; succeeded %d; failed %d; unavailable %d; skipped %d; canceled %d\nManifest: %q\n",
			c.Selected, c.Succeeded, c.Failed, c.Unavailable, c.Skipped, c.Canceled, d.ManifestPath)
	}
	if len(buf) == 0 {
		return nil
	}
	_, err := w.Write(buf)
	return err
}
