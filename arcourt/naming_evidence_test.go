package arcourt

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/giraffesyo/pdf/pdftest"
)

func syntheticNamingPDF(pages ...[]string) []byte {
	objects := []string{pdftest.Catalog(2), ""}
	ids := make([]int, 0, len(pages))
	fontID := 3 + len(pages)*2
	for _, lines := range pages {
		pageID := len(objects) + 1
		contentID := pageID + 1
		ids = append(ids, pageID)
		objects = append(objects, pdftest.Page(2, contentID, fmt.Sprintf("<< /Font << /F1 %d 0 R >> >>", fontID)), pdftest.Stream("", namingPageContent(lines)))
	}
	objects[1] = pdftest.Pages(ids...)
	objects = append(objects, pdftest.Helvetica())
	return pdftest.Build(1, objects...)
}

func namingPageContent(lines []string) string {
	var b strings.Builder
	b.WriteString("BT /F1 12 Tf 72 700 Td ")
	for i, line := range lines {
		if i > 0 {
			b.WriteString("0 -18 Td ")
		}
		line = strings.ReplaceAll(line, `\`, `\\`)
		line = strings.ReplaceAll(line, `(`, `\(`)
		line = strings.ReplaceAll(line, `)`, `\)`)
		fmt.Fprintf(&b, "(%s) Tj ", line)
	}
	b.WriteString("ET")
	return b.String()
}

func evidenceFromBytes(data []byte) NamingEvidence {
	return extractNamingEvidence(context.Background(), bytes.NewReader(data), int64(len(data)))
}

func TestNamingEvidenceTargetsHeadingAndOnlyOpeningPages(t *testing.T) {
	data := syntheticNamingPDF(
		[]string{
			"FILED: OCTOBER 8, 2026", "IN THE CIRCUIT COURT OF PULASKI COUNTY", "CASE NO. 60CV-2026-1",
			"PLAINTIFF", "v.", "DEFENDANT", "DEFENDANT'S RESPONSE TO", "PLAINTIFF'S MOTION TO DISMISS",
			"Defendant opposes dismissal because the complaint states a claim.",
			"The motion to dismiss quoted above does not dispose of the petition.",
		},
		[]string{"Further argument on the response follows here."},
		[]string{"TOP SECRET THIRD PAGE SHOULD NEVER BE EXTRACTED"},
	)
	e := evidenceFromBytes(data)
	if e.Reason != "" || e.Pages != 2 {
		t.Fatalf("unexpected evidence status: %+v", e)
	}
	for _, s := range []string{e.Small, e.Medium} {
		if !strings.HasPrefix(s, "DEFENDANT'S RESPONSE TO\nPLAINTIFF'S MOTION TO DISMISS") {
			t.Fatalf("heading was not prioritized: %q", s)
		}
		if strings.Contains(s, "IN THE CIRCUIT COURT") || strings.Contains(s, "CASE NO.") || strings.Contains(s, "THIRD PAGE") {
			t.Fatalf("irrelevant or nonopening text transmitted: %q", s)
		}
	}
	if strings.Contains(e.Baseline, "THIRD PAGE") {
		t.Fatal("baseline included third page")
	}
	if utf8.RuneCountInString(e.Small) > 512 || utf8.RuneCountInString(e.Medium) > 1024 || utf8.RuneCountInString(e.Baseline) > 4096 {
		t.Fatal("evidence exceeds character budgets")
	}
}

func TestNamingEvidenceFindsSecondPageHeading(t *testing.T) {
	data := syntheticNamingPDF(
		[]string{"IN THE CIRCUIT COURT", "CASE NO. 60CV-2026-2", "PLAINTIFF", "DEFENDANT"},
		[]string{"SECOND AMENDED COMPLAINT", "Plaintiff alleges the following facts."},
	)
	e := evidenceFromBytes(data)
	if e.Reason != "" || !strings.HasPrefix(e.Small, "SECOND AMENDED COMPLAINT") || e.Pages != 2 {
		t.Fatalf("second-page title not selected: %+v", e)
	}
}

func TestNamingEvidencePrefersResponseOverEarlierQuotedMotion(t *testing.T) {
	data := syntheticNamingPDF([]string{
		"IN THE CIRCUIT COURT", "CASE NO. 60CV-2026-3",
		"MOTION TO DISMISS", "Previously filed by the plaintiff.",
		"DEFENDANT'S RESPONSE TO MOTION TO DISMISS",
		"Defendant opposes the motion because the complaint states a claim.",
	})
	e := evidenceFromBytes(data)
	if e.Reason != "" || !strings.HasPrefix(e.Small, "DEFENDANT'S RESPONSE TO MOTION TO DISMISS") {
		t.Fatalf("response lost to quoted motion: %+v", e)
	}
}

func TestNamingEvidenceKeepsOpeningOrderAheadOfQuotedResponse(t *testing.T) {
	for _, title := range []string{"Order Granting Motion to Dismiss", "ORDER GRANTING MOTION TO DISMISS"} {
		data := syntheticNamingPDF([]string{
			title,
			"PLAINTIFF'S RESPONSE TO MOTION TO DISMISS",
			"The response above is quoted from the docket for context.",
			"The court grants the motion to dismiss.",
		})
		e := evidenceFromBytes(data)
		if e.Reason != "" || !strings.HasPrefix(e.Small, title) {
			t.Fatalf("quoted response displaced opening order %q: %+v", title, e)
		}
	}
}

func TestNamingEvidenceRetainsSeparateMaterialQualifiers(t *testing.T) {
	for _, tc := range []struct {
		lines []string
		want  string
	}{
		{[]string{"PROPOSED", "ORDER GRANTING MOTION", "Submitted for the court's consideration."}, "PROPOSED\nORDER GRANTING MOTION"},
		{[]string{"SECOND AMENDED", "COMPLAINT", "Plaintiff alleges additional facts."}, "SECOND AMENDED\nCOMPLAINT"},
	} {
		e := evidenceFromBytes(syntheticNamingPDF(tc.lines))
		if e.Reason != "" || !strings.HasPrefix(e.Small, tc.want) {
			t.Fatalf("material qualifier omitted from %q: %+v", tc.want, e)
		}
	}
}

func TestNamingEvidenceDoesNotPreferBodySentenceToTitle(t *testing.T) {
	e := evidenceFromBytes(syntheticNamingPDF([]string{
		"DEFAULT JUDGMENT", "Judgment is entered after default.",
	}))
	if e.Reason != "" || !strings.HasPrefix(e.Small, "DEFAULT JUDGMENT") {
		t.Fatalf("body sentence displaced title: %+v", e)
	}
}

func TestNamingEvidenceClassifiesImageOnlyAndUnreadable(t *testing.T) {
	image := pdftest.Build(1,
		pdftest.Catalog(2),
		pdftest.Pages(3),
		pdftest.Page(2, 4, "<< /XObject << /Im1 5 0 R >> >>"),
		pdftest.Stream("", "/Im1 Do"),
		pdftest.Stream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8", "x"),
	)
	if e := evidenceFromBytes(image); e.Reason != "image_only" || e.Small != "" || e.Pages != 1 {
		t.Fatalf("image-only fixture: %+v", e)
	}
	stampedImage := pdftest.Build(1,
		pdftest.Catalog(2),
		pdftest.Pages(3),
		pdftest.Page(2, 4, "<< /XObject << /Im1 6 0 R >> /Font << /F1 5 0 R >> >>"),
		pdftest.Stream("", namingPageContent([]string{"FILED: OCTOBER 8, 2026", "ORDER"})+" /Im1 Do"),
		pdftest.Helvetica(),
		pdftest.Stream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8", "x"),
	)
	if e := evidenceFromBytes(stampedImage); e.Reason != "image_only" || e.Small != "" {
		t.Fatalf("stamped image-only fixture: %+v", e)
	}
	blank := syntheticNamingPDF([]string{})
	if e := evidenceFromBytes(blank); e.Reason != "unreadable" {
		t.Fatalf("blank fixture: %+v", e)
	}
	if e := evidenceFromBytes([]byte("%PDF-1.4\ninvalid\n%%EOF")); e.Reason != "unreadable" {
		t.Fatalf("malformed fixture: %+v", e)
	}
}

func TestNamingEvidenceClassifiesEncryptedOversizedAndCanceled(t *testing.T) {
	objects := []string{
		pdftest.Catalog(2), pdftest.Pages(3), pdftest.Page(2, 4, "<< /Font << /F1 5 0 R >> >>"),
		pdftest.Stream("", namingPageContent([]string{"MOTION TO COMPEL"})), pdftest.Helvetica(),
	}
	encrypted := pdftest.BuildEncrypted(1, pdftest.EncryptSpec{R: 3, UserPassword: "secret", OwnerPassword: "owner"}, objects...)
	if e := evidenceFromBytes(encrypted); e.Reason != "encrypted" {
		t.Fatalf("encrypted fixture: %+v", e)
	}
	if e := extractNamingEvidence(context.Background(), bytes.NewReader(nil), maxNamingPDFBytes+1); e.Reason != "too_large" {
		t.Fatalf("oversized fixture: %+v", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := syntheticNamingPDF([]string{"ORDER GRANTING MOTION"})
	if e := extractNamingEvidence(ctx, bytes.NewReader(data), int64(len(data))); e.Reason != "canceled" {
		t.Fatalf("canceled fixture: %+v", e)
	}
}

func TestNamingEvidenceRejectsParserWorkLimits(t *testing.T) {
	// A valid PDF stream that inflates past the configured one-megabyte
	// parser limit must not leak a partial title to the provider.
	content := "BT /F1 12 Tf 72 700 Td (ORDER DENYING RELIEF) Tj ET\n" + strings.Repeat(" ", maxNamingStreamBytes+1)
	data := pdftest.Build(1,
		pdftest.Catalog(2), pdftest.Pages(3),
		pdftest.Page(2, 4, "<< /Font << /F1 5 0 R >> >>"),
		pdftest.Flate("", content), pdftest.Helvetica(),
	)
	if e := evidenceFromBytes(data); e.Reason != "unreadable" || e.Small != "" {
		t.Fatalf("stream-limit fixture: %+v", e)
	}
}

func TestNamingEvidenceUnicodeOutputCeiling(t *testing.T) {
	text := strings.Repeat("表", 5000)
	for _, tc := range []struct{ chars, bytes int }{{512, 2048}, {1024, 4096}, {4096, 8192}} {
		bounded := limitNamingText(text, tc.chars, tc.bytes)
		if utf8.RuneCountInString(bounded) > tc.chars || len(bounded) > tc.bytes || !utf8.ValidString(bounded) {
			t.Fatalf("invalid ceiling for %d chars / %d bytes", tc.chars, tc.bytes)
		}
	}
}

func TestNamingEvidenceCumulativeReadBudget(t *testing.T) {
	used := new(atomic.Int64)
	used.Store(maxNamingReadBytes)
	reader := namingReaderAt{ctx: context.Background(), source: bytes.NewReader([]byte("x")), size: 1, used: used}
	if _, err := reader.ReadAt(make([]byte, 1), 0); err == nil {
		t.Fatal("repeated structural reads exceeded cumulative budget")
	}
}
