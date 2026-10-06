package arcourt

import "testing"

func TestNormalizeLinkRows(t *testing.T) {
	rows := []linkRow{
		{URL: "", Description: "missing"},
		{URL: "https://example/doc1.pdf", Description: "First", FilingDate: " 01/01/2026 "},
		{URL: "https://example/doc1.pdf", Description: "dup", FilingDate: "02/02/2026"},
		{URL: " https://example/doc2.pdf ", SourceURL: " https://example/source/2 ", Description: " Second ", FilingDate: " 03/03/2026 "},
	}
	got := normalizeLinkRows(rows)
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}
	if got[0].URL != "https://example/doc1.pdf" || got[0].Description != "First" {
		t.Fatalf("unexpected first row: %+v", got[0])
	}
	if got[0].FilingDate != "01/01/2026" {
		t.Fatalf("unexpected first filing_date: %q", got[0].FilingDate)
	}
	if got[1].URL != "https://example/doc2.pdf" || got[1].Description != "Second" {
		t.Fatalf("unexpected second row: %+v", got[1])
	}
	if got[1].SourceURL != "https://example/source/2" {
		t.Fatalf("unexpected second source_url: %q", got[1].SourceURL)
	}
	if got[1].FilingDate != "03/03/2026" {
		t.Fatalf("unexpected second filing_date: %q", got[1].FilingDate)
	}
}
