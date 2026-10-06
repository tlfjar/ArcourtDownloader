package arcourt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	_ "time/tzdata" // Arkansas dates must also work on Windows without zoneinfo.
)

// Only the verified public OPAD route uses this adapter. Custom templates keep
// their browser behavior; query strings and alternate hosts are not discarded.
func opadCaseAPI(template, caseNumber string) string {
	u, err := url.Parse(strings.ReplaceAll(template, "{case_number}", caseNumber))
	if err != nil || !strings.Contains(template, "{case_number}") ||
		u.Scheme != "https" || u.Host != "caseinfonew.arcourts.gov" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		u.RawPath != "" || u.Path != "/opad/case/"+caseNumber {
		return ""
	}
	u.Path = "/opad/api/cases/" + caseNumber
	return u.String()
}

type opadDocument struct {
	CaseID      string `json:"caseId"`
	FileID      string `json:"documentFileId"`
	Name        string `json:"documentName"`
	Description string `json:"documentDesc"`
	UploadDate  string `json:"documentUploadDate"`
}

type opadCase struct {
	ID        string         `json:"caseId"`
	Title     string         `json:"caseTitle"`
	County    string         `json:"courtDesc"`
	Documents []opadDocument `json:"caseDocuments"`
	Dockets   []struct {
		CaseID      string         `json:"caseId"`
		Description string         `json:"docketDesc"`
		FilingDate  string         `json:"docketFilingDate"`
		Documents   []opadDocument `json:"docketDocuments"`
	} `json:"caseDockets"`
	Participants []struct {
		CaseID string `json:"caseId"`
		Name   string `json:"name"`
		Role   string `json:"partyType"`
	} `json:"caseParticipants"`
}

func (f *BrowserFetcher) readOPADCase(ctx context.Context, endpoint, caseNumber string) (caseDiscovery, error) {
	timeout := f.cfg.PageTimeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := newDocumentClient(f.cfg.DocumentHTTP)
	defer c.http.CloseIdleConnections()
	var err error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		var d caseDiscovery
		d, err = c.readOPADCaseOnce(ctx, endpoint, caseNumber)
		if err == nil {
			return d, nil
		}
		if !IsRetryable(err) || attempt == c.cfg.MaxAttempts {
			break
		}
		timer := time.NewTimer(time.Duration(attempt) * c.cfg.RetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return caseDiscovery{}, ctx.Err()
		case <-timer.C:
		}
	}
	if ctx.Err() != nil {
		return caseDiscovery{}, ctx.Err()
	}
	return caseDiscovery{}, err
}

func (c *documentClient) readOPADCaseOnce(ctx context.Context, endpoint, caseNumber string) (caseDiscovery, error) {
	if _, err := validateDocumentFetchURL(endpoint); err != nil {
		return caseDiscovery{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return caseDiscovery{}, ErrDestinationRejected
	}
	req.Header = documentHeaders()
	res, err := c.http.Do(req)
	if err != nil {
		return caseDiscovery{}, fmt.Errorf("%w: %w", ErrBackendUnavailable, &transportError{cause: err})
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		status := &HTTPStatusError{StatusCode: res.StatusCode}
		switch res.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return caseDiscovery{}, fmt.Errorf("%w: %w", ErrCaseAccessDenied, status)
		case http.StatusNotFound:
			return caseDiscovery{}, fmt.Errorf("%w: case not found", ErrCaseLoadFailed)
		default:
			return caseDiscovery{}, fmt.Errorf("%w: %w", ErrBackendUnavailable, status)
		}
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(res.Header.Get("Content-Type"), ";")[0]))
	if ct != "application/json" && !strings.HasSuffix(ct, "+json") {
		return caseDiscovery{}, fmt.Errorf("%w: expected OPAD case JSON", ErrCaseLoadFailed)
	}
	payload, err := io.ReadAll(io.LimitReader(res.Body, c.cfg.MaxJSONBytes+1))
	if err != nil {
		return caseDiscovery{}, fmt.Errorf("%w: %w", ErrBackendUnavailable, &transportError{cause: err})
	}
	if int64(len(payload)) > c.cfg.MaxJSONBytes {
		return caseDiscovery{}, fmt.Errorf("%w: OPAD case response exceeds JSON size limit", ErrCaseLoadFailed)
	}
	var data opadCase
	if json.Unmarshal(payload, &data) != nil {
		return caseDiscovery{}, fmt.Errorf("%w: invalid OPAD case JSON", ErrCaseLoadFailed)
	}
	if err := validateCaseNumberMatch(data.ID, caseNumber); err != nil {
		return caseDiscovery{}, err
	}
	// Absent arrays may indicate a changed schema or an error envelope. Do not
	// silently present that as a successful empty docket.
	if strings.TrimSpace(data.Title) == "" || data.Dockets == nil || data.Documents == nil || data.Participants == nil {
		return caseDiscovery{}, fmt.Errorf("%w: incomplete OPAD case response", ErrCaseLoadFailed)
	}
	d := caseDiscovery{Info: CaseInfo{CaseNumber: data.ID, CaseTitle: data.Title, County: data.County}}
	belongs := func(id string) bool { return id == "" || validateCaseNumberMatch(id, caseNumber) == nil }
	for _, p := range data.Participants {
		if !belongs(p.CaseID) {
			return caseDiscovery{}, ErrCaseNumberMismatch
		}
		if strings.EqualFold(strings.TrimSpace(p.Role), "JUDGE") {
			if d.Info.Judge != "" {
				d.Info.Judge += "; "
			}
			d.Info.Judge += strings.TrimSpace(p.Name)
		} else {
			d.Info.Parties = append(d.Info.Parties, Party{Name: p.Name, Role: p.Role})
		}
	}
	base, _ := url.Parse(endpoint)
	appendDocument := func(doc opadDocument, description, date string) error {
		if !belongs(doc.CaseID) {
			return ErrCaseNumberMismatch
		}
		if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(doc.Name)), ".pdf") {
			return nil
		}
		if strings.TrimSpace(doc.FileID) == "" || strings.ContainsAny(doc.FileID, "/\\") || doc.FileID == "." || doc.FileID == ".." {
			return fmt.Errorf("%w: invalid OPAD document identity", ErrCaseLoadFailed)
		}
		// The file ID, not the display filename, is the document API key.
		target := base.Scheme + "://" + base.Host + "/opad/api/documents/" + url.PathEscape(doc.FileID)
		d.Rows = append(d.Rows, linkRow{URL: target, SourceURL: target, Description: description, FilingDate: opadDate(date)})
		return nil
	}
	for _, docket := range data.Dockets {
		if !belongs(docket.CaseID) {
			return caseDiscovery{}, ErrCaseNumberMismatch
		}
		for _, doc := range docket.Documents {
			if err := appendDocument(doc, docket.Description, docket.FilingDate); err != nil {
				return caseDiscovery{}, err
			}
		}
	}
	for _, doc := range data.Documents {
		if err := appendDocument(doc, doc.Description, doc.UploadDate); err != nil {
			return caseDiscovery{}, err
		}
	}
	count := len(data.Dockets)
	d.Coverage.ReportedDocketRows = &count
	d.Coverage.Limitations = []string{"Documents were read from the public OPAD case response, including case-level documents and every returned docket attachment. Sealed, withheld, or unreturned records are not discoverable; whole-docket completeness is unknown."}
	return d, nil
}

func opadDate(value string) string {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	zone, err := time.LoadLocation("America/Chicago")
	if err != nil {
		return ""
	} // Embedded tzdata makes this independent of the OS.
	return t.In(zone).Format("01/02/2006")
}
