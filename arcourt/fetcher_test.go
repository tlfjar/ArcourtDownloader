package arcourt

// Case identities in these tests are fabricated; no live case data is used.

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeCaseInfo(t *testing.T) {
	got := normalizeCaseInfo(CaseInfo{
		CaseNumber: "   ",
		CaseTitle:  "  State v. Doe  ",
		County:     "  Pulaski  ",
		Judge:      "  Hon. Smith  ",
		Parties: []Party{
			{Name: "  John Doe  ", Role: "  Defendant  "},
			{Name: "", Role: ""},
		},
	}, "60CV-2026-1")

	if got.CaseNumber != "60CV-2026-1" {
		t.Fatalf("case_number=%q, want %q", got.CaseNumber, "60CV-2026-1")
	}
	if got.CaseTitle != "State v. Doe" {
		t.Fatalf("case_title=%q, want %q", got.CaseTitle, "State v. Doe")
	}
	if got.County != "Pulaski" {
		t.Fatalf("county=%q, want %q", got.County, "Pulaski")
	}
	if got.Judge != "Hon. Smith" {
		t.Fatalf("judge=%q, want %q", got.Judge, "Hon. Smith")
	}
	if len(got.Parties) != 1 {
		t.Fatalf("parties=%d, want 1", len(got.Parties))
	}
	if got.Parties[0].Name != "John Doe" || got.Parties[0].Role != "Defendant" {
		t.Fatalf("unexpected party: %+v", got.Parties[0])
	}
}

func TestNormalizeCaseInfoMismatchedExtractedCaseNumberIsPreserved(t *testing.T) {
	got := normalizeCaseInfo(CaseInfo{CaseNumber: "60CV-2020-999"}, "60CV-2026-1")
	if got.CaseNumber != "60CV-2020-999" {
		t.Fatalf("case_number=%q, want %q", got.CaseNumber, "60CV-2020-999")
	}
}

func TestValidateCaseNumberMatch(t *testing.T) {
	tests := []struct {
		name      string
		extracted string
		requested string
	}{
		{name: "exact", extracted: "60CV-2026-1", requested: "60CV-2026-1"},
		{name: "case insensitive", extracted: "60cv-2026-1", requested: "60CV-2026-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateCaseNumberMatch(tt.extracted, tt.requested); err != nil {
				t.Fatalf("validateCaseNumberMatch(%q, %q) returned error: %v", tt.extracted, tt.requested, err)
			}
		})
	}
}

func TestValidateCaseNumberMatchMissingExtractedFails(t *testing.T) {
	err := validateCaseNumberMatch("", "60CV-2026-1")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !errors.Is(err, ErrCaseNumberMismatch) {
		t.Fatalf("errors.Is(err, ErrCaseNumberMismatch)=false, err=%v", err)
	}
}

func TestValidateCaseNumberMatchNonCaseExtracted(t *testing.T) {
	err := validateCaseNumberMatch("case header unavailable", "60CV-2026-1")
	if err == nil {
		t.Fatal("expected mismatch error when extracted case number is not structured")
	}
	if !errors.Is(err, ErrCaseNumberMismatch) {
		t.Fatalf("errors.Is(err, ErrCaseNumberMismatch)=false, err=%v", err)
	}
}

func TestValidateCaseNumberMatchMismatch(t *testing.T) {
	err := validateCaseNumberMatch("60CV-2020-999", "60CV-2026-1")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !errors.Is(err, ErrCaseNumberMismatch) {
		t.Fatalf("errors.Is(err, ErrCaseNumberMismatch)=false, err=%v", err)
	}
}

func TestHasMeaningfulCaseContent(t *testing.T) {
	if hasMeaningfulCaseContent(CaseInfo{CaseNumber: "99ZZZ-99-1"}, nil) {
		t.Fatal("expected case number alone to be treated as empty content")
	}

	if !hasMeaningfulCaseContent(CaseInfo{CaseTitle: "JESSE WILSON V TRISTAN WILSON"}, nil) {
		t.Fatal("expected case title to count as meaningful content")
	}

	if !hasMeaningfulCaseContent(CaseInfo{}, []linkRow{{URL: "https://example/doc.pdf"}}) {
		t.Fatal("expected docket rows to count as meaningful content")
	}
}

func TestBuildCaseLoadErrorUsesRenderedPageError(t *testing.T) {
	err := buildCaseLoadError("99ZZZ-99-1", casePageState{
		Title:        "Error • Arkansas Judiciary",
		ErrorHeading: "Something went wrong",
		ErrorDetail:  "Response returned an error code",
	})
	if !errors.Is(err, ErrCaseLoadFailed) {
		t.Fatalf("errors.Is(err, ErrCaseLoadFailed)=false, err=%v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "Something went wrong") || !strings.Contains(msg, "Response returned an error code") {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestBuildCaseLoadTimeoutError(t *testing.T) {
	err := buildCaseLoadTimeoutError("99ZZZ-99-1", casePageState{Title: "Case Loading • Arkansas Judiciary", Loading: true})
	if !errors.Is(err, ErrCaseLoadFailed) {
		t.Fatalf("errors.Is(err, ErrCaseLoadFailed)=false, err=%v", err)
	}
	if !strings.Contains(err.Error(), "timed out waiting for Arkansas Judiciary to load case") {
		t.Fatalf("unexpected message: %q", err.Error())
	}
}

func TestValidateDocumentFetchURLRejectsLocalTargets(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/document.pdf",
		"http://localhost/document.pdf",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.5/document.pdf",
	}
	for _, target := range blocked {
		t.Run(target, func(t *testing.T) {
			if _, err := validateDocumentFetchURL(target); err == nil {
				t.Fatalf("expected %q to be blocked", target)
			}
		})
	}
}

func TestValidateResolvedDocumentURLAllowsRelativeAndArcourtHosts(t *testing.T) {
	got, err := validateResolvedDocumentURL("https://caseinfonew.arcourts.gov/opad/api/documents/ABC", "../files/order.pdf")
	if err != nil {
		t.Fatalf("expected relative same-host URL to pass: %v", err)
	}
	if got != "https://caseinfonew.arcourts.gov/opad/api/files/order.pdf" {
		t.Fatalf("resolved URL=%q", got)
	}

	if _, err := validateResolvedDocumentURL("https://caseinfonew.arcourts.gov/opad/api/documents/ABC", "https://static.arcourts.gov/doc.pdf"); err != nil {
		t.Fatalf("expected arcourts host to pass: %v", err)
	}

	if _, err := validateResolvedDocumentURL("https://caseinfonew.arcourts.gov/opad/api/documents/ABC", "https://cdr-prod-cmslegacy-images-bucket.s3.us-gov-west-1.amazonaws.com/doc.pdf"); err != nil {
		t.Fatalf("expected Arcourt legacy document bucket to pass: %v", err)
	}
}

func TestValidateResolvedDocumentURLRejectsPrivateOrUnexpectedHosts(t *testing.T) {
	tests := []string{
		"http://127.0.0.1/document.pdf",
		"https://attacker.example/document.pdf",
		"https://attacker-bucket.s3.us-gov-west-1.amazonaws.com/document.pdf",
		"https://s3.us-gov-west-1.amazonaws.com/cdr-prod-cmslegacy-images-bucket/document.pdf",
	}
	for _, next := range tests {
		t.Run(next, func(t *testing.T) {
			if _, err := validateResolvedDocumentURL("https://caseinfonew.arcourts.gov/opad/api/documents/ABC", next); err == nil {
				t.Fatalf("expected %q to be rejected", next)
			}
		})
	}
}

func TestShouldRetryCasePageLoad(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "backend unavailable",
			err:  ErrBackendUnavailable,
			want: true,
		},
		{
			name: "transient json parse error",
			err:  buildCaseLoadError("99ZZZ-99-1", casePageState{ErrorHeading: "Something went wrong", ErrorDetail: "Unexpected token '<', \"<html> <h\"... is not valid JSON"}),
			want: true,
		},
		{
			name: "transient generic response error",
			err:  buildCaseLoadError("99ZZZ-99-1", casePageState{ErrorHeading: "Something went wrong", ErrorDetail: "Response returned an error code"}),
			want: true,
		},
		{
			name: "timeout waiting for page content",
			err:  buildCaseLoadTimeoutError("99ZZZ-99-1", casePageState{Title: "Case Loading • Arkansas Judiciary", Loading: true}),
			want: true,
		},
		{
			name: "case not found",
			err:  buildCaseLoadError("99ZZZ-99-1", casePageState{ErrorHeading: "Case not found"}),
			want: false,
		},
		{
			name: "preview mismatch",
			err:  ErrCaseNumberMismatch,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryCasePageLoad(tt.err); got != tt.want {
				t.Fatalf("shouldRetryCasePageLoad()=%v, want %v, err=%v", got, tt.want, tt.err)
			}
		})
	}
}

func TestCaseAppEntryURL(t *testing.T) {
	tests := []struct {
		name    string
		caseURL string
		want    string
	}{
		{
			name:    "opad case route",
			caseURL: "https://caseinfonew.arcourts.gov/opad/case/99ZZZ-99-1",
			want:    "https://caseinfonew.arcourts.gov/opad",
		},
		{
			name:    "opad case route with query",
			caseURL: "https://caseinfonew.arcourts.gov/opad/case/99ZZZ-99-1?caseId=99ZZZ-99-1#summary",
			want:    "https://caseinfonew.arcourts.gov/opad",
		},
		{
			name:    "root case route",
			caseURL: "https://example.test/case/99ZZZ-99-1",
			want:    "https://example.test/",
		},
		{
			name:    "invalid",
			caseURL: "://bad",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caseAppEntryURL(tt.caseURL); got != tt.want {
				t.Fatalf("caseAppEntryURL()=%q, want %q", got, tt.want)
			}
		})
	}
}
