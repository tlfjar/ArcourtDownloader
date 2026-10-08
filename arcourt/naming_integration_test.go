package arcourt

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giraffesyo/pdf/pdftest"
)

type fakeNamingClient struct {
	calls []string
	fn    func(context.Context, string) (string, error)
}

type invalidUsageNamingClient struct{}

func (invalidUsageNamingClient) Generate(context.Context, string, string) (string, NamingUsage, error) {
	return "Motion to Dismiss", NamingUsage{RequestBytes: 400, InputTokens: -1, Reported: true}, nil
}

func (f *fakeNamingClient) Generate(ctx context.Context, prompt, excerpt string) (string, NamingUsage, error) {
	f.calls = append(f.calls, excerpt)
	if !strings.Contains(prompt, "ABSTAIN") {
		return "", NamingUsage{}, errors.New("missing naming contract")
	}
	if f.fn == nil {
		return "Defendant Motion to Compel", NamingUsage{InputTokens: 20, OutputTokens: 6, Reported: true, RequestBytes: len(prompt) + len(excerpt)}, nil
	}
	raw, err := f.fn(ctx, excerpt)
	return raw, NamingUsage{RequestBytes: len(prompt) + len(excerpt)}, err
}

func namingDownloadFixture(t *testing.T, count int, pdf []byte) (*DownloadService, *localFakeFetcher, DownloadRequest) {
	t.Helper()
	s, f, req := localFixture(t, count)
	f.data = make(map[string]string, count)
	for _, entry := range req.Selection {
		f.data[entry.SourceURL] = string(pdf)
	}
	return s, f, req
}

func TestNamingUsesVerifiedPDFBeforeNoOverwritePublication(t *testing.T) {
	pdf := syntheticNamingPDF(
		[]string{"FILED: OCTOBER 8, 2026", "DEFENDANT'S MOTION TO COMPEL DISCOVERY", "Defendant asks that discovery be produced."},
		[]string{"Second page of the motion."},
		[]string{"UNSELECTED THIRD PAGE MUST STAY LOCAL"},
	)
	s, f, req := namingDownloadFixture(t, 1, pdf)
	req.Selection[0].DocketDescription = "Generic Docket Label DO-NOT-SEND"
	f.entries[0].DocketDescription = req.Selection[0].DocketDescription
	req.Naming = &NamingRequest{Provider: "openai", Model: "gpt-test", APIKey: "TEST-SECRET"}
	client := &fakeNamingClient{}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	preview, err := s.Preview(context.Background(), req.CaseNumber)
	if err != nil || len(preview.DocketEntries) != 1 || len(client.calls) != 0 {
		t.Fatalf("preview called naming: %v %v", err, client.calls)
	}
	r, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 1, Succeeded: 1})
	d := r.Documents[0]
	if d.Naming == nil || d.Naming.Source != "ai" || d.Naming.Label != "Defendant Motion to Compel" || d.Naming.Calls != 1 || !strings.Contains(d.Filename, "defendant_motion_to_compel") {
		t.Fatalf("naming outcome: %+v", d)
	}
	if d.Description != req.Selection[0].DocketDescription {
		t.Fatal("original docket description lost")
	}
	saved, readErr := os.ReadFile(filepath.Join(r.Directory, d.Filename))
	if readErr != nil || !bytes.Equal(saved, pdf) {
		t.Fatalf("PDF bytes changed: %v", readErr)
	}
	if len(client.calls) != 1 || strings.Contains(client.calls[0], "DO-NOT-SEND") || strings.Contains(client.calls[0], "UNSELECTED THIRD PAGE") || strings.Contains(client.calls[0], "https://") || strings.Contains(client.calls[0], "TEST-SECRET") {
		t.Fatalf("unexpected provider context: %q", client.calls)
	}
	m := readDownloadManifest(t, r)
	if len(m.Documents) != 1 || m.Documents[0].Naming == nil || m.Documents[0].Naming.Label != d.Naming.Label {
		t.Fatal("manifest lost naming result")
	}
	manifestBytes, readErr := os.ReadFile(r.ManifestPath)
	if readErr != nil || bytes.Contains(manifestBytes, []byte("TEST-SECRET")) || bytes.Contains(manifestBytes, []byte("Defendant asks")) {
		t.Fatal("secret or excerpt persisted")
	}
	req.Naming.Model = "changed-model"
	again, err := s.Download(context.Background(), req, nil)
	if err != nil || again.Documents[0].SkipReason != SkipVerified || len(client.calls) != 1 || len(f.calls) != 1 {
		t.Fatalf("repeat incurred paid naming or download: %+v %v", again, err)
	}
}

func TestNamingFallbackAndPermanentFailureSuppression(t *testing.T) {
	pdf := syntheticNamingPDF([]string{"PLAINTIFF'S RESPONSE TO MOTION TO DISMISS", "Plaintiff opposes dismissal."})
	s, _, req := namingDownloadFixture(t, 2, pdf)
	req.Naming = &NamingRequest{Provider: "anthropic", Model: "claude-test", APIKey: "TEST-SECRET"}
	client := &fakeNamingClient{fn: func(context.Context, string) (string, error) {
		return "", &namingError{reason: "provider_auth", permanent: true}
	}}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	r, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 2, Succeeded: 2})
	if len(client.calls) != 1 || r.Documents[0].Naming.Reason != "provider_auth" || r.Documents[1].Naming.Reason != "provider_auth" {
		t.Fatalf("batch failure was not suppressed: %+v calls=%d", r.Documents, len(client.calls))
	}
	for _, d := range r.Documents {
		if d.Naming.Source != "deterministic" || !strings.Contains(d.Filename, "order") {
			t.Fatalf("fallback changed download result: %+v", d)
		}
	}
}

func TestNamingRejectsUnsafeOutputWithoutFailingDownload(t *testing.T) {
	pdf := syntheticNamingPDF([]string{"ORDER DENYING MOTION TO DISMISS", "The motion is denied."})
	s, _, req := namingDownloadFixture(t, 1, pdf)
	req.Naming = &NamingRequest{Provider: "google", Model: "gemini-test", APIKey: "TEST-SECRET"}
	client := &fakeNamingClient{fn: func(context.Context, string) (string, error) { return `..\CON.pdf`, nil }}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	r, err := s.Download(context.Background(), req, nil)
	if err != nil || r.Documents[0].Naming == nil || r.Documents[0].Naming.Reason != "unsafe_result" || !r.Documents[0].Saved {
		t.Fatalf("unsafe result became download failure: %+v %v", r, err)
	}
}

func TestNamingCancellationPublishesValidatedPDFWithFallback(t *testing.T) {
	pdf := syntheticNamingPDF([]string{"DEFENDANT'S MOTION TO COMPEL", "Defendant asks for discovery."})
	s, _, req := namingDownloadFixture(t, 2, pdf)
	req.Naming = &NamingRequest{Provider: "xai", Model: "grok-test", APIKey: "TEST-SECRET"}
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeNamingClient{fn: func(ctx context.Context, _ string) (string, error) {
		cancel()
		<-ctx.Done()
		return "", ctx.Err()
	}}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	r, err := s.Download(ctx, req, nil)
	if !errors.Is(err, context.Canceled) || !r.Documents[0].Saved || r.Documents[0].Naming.Reason != "canceled" || r.Documents[0].Status != DocumentSucceeded {
		t.Fatalf("validated PDF not finalized on naming cancellation: %+v %v", r, err)
	}
	if r.Documents[1].Status != DocumentCanceled || len(client.calls) != 1 {
		t.Fatalf("pending work continued after cancellation: %+v", r.Documents)
	}
	saved, readErr := os.ReadFile(filepath.Join(r.Directory, r.Documents[0].Filename))
	if readErr != nil || !bytes.Equal(saved, pdf) {
		t.Fatalf("published PDF lost or changed: %v", readErr)
	}
}

func TestNamingImageOnlyFallbackNeverCallsProvider(t *testing.T) {
	pdf := pdftest.Build(1,
		pdftest.Catalog(2), pdftest.Pages(3),
		pdftest.Page(2, 4, "<< /XObject << /Im1 5 0 R >> >>"),
		pdftest.Stream("", "/Im1 Do"),
		pdftest.Stream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8", "x"),
	)
	s, _, req := namingDownloadFixture(t, 1, pdf)
	req.Naming = &NamingRequest{Provider: "openai", Model: "gpt-test", APIKey: "TEST-SECRET"}
	client := &fakeNamingClient{}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	r, err := s.Download(context.Background(), req, nil)
	if err != nil || !r.Documents[0].Saved || r.Documents[0].Naming.Reason != "image_only" || len(client.calls) != 0 {
		t.Fatalf("image-only naming did not fall back safely: %+v %v", r, err)
	}
}

func TestNamingExpansionCostsTwoCompleteAttempts(t *testing.T) {
	lines := []string{"DEFENDANT'S MOTION TO COMPEL DISCOVERY"}
	for i := 0; i < 16; i++ {
		lines = append(lines, "Defendant requests production of the documents identified in discovery item "+string(rune('A'+i)))
	}
	pdf := syntheticNamingPDF(lines)
	s, _, req := namingDownloadFixture(t, 1, pdf)
	req.Naming = &NamingRequest{Provider: "openai", Model: "gpt-test", APIKey: "TEST-SECRET"}
	client := &fakeNamingClient{fn: func(_ context.Context, excerpt string) (string, error) {
		if len(excerpt) <= 512 {
			return "ABSTAIN", nil
		}
		return "Defendant Motion to Compel", nil
	}}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	r, err := s.Download(context.Background(), req, nil)
	if err != nil || r.Documents[0].Naming.Source != "ai" || r.Documents[0].Naming.Calls != 2 || len(client.calls) != 2 || len(client.calls[1]) <= len(client.calls[0]) {
		t.Fatalf("bounded expansion failed: %+v %v, calls=%d", r, err, len(client.calls))
	}
	u := r.Documents[0].Naming.Usage
	if u == nil || u.RequestBytes != u.InputTokenEstimate || u.RequestBytes < len(client.calls[0])+len(client.calls[1]) || u.Reported {
		t.Fatalf("attempt costs not aggregated or mislabeled: %+v", u)
	}
}

func TestNamingRecoveryPreservesLabelWithoutAnotherCall(t *testing.T) {
	pdf := syntheticNamingPDF([]string{"DEFENDANT'S MOTION TO COMPEL", "Defendant requests discovery."})
	s, _, req := namingDownloadFixture(t, 1, pdf)
	req.Naming = &NamingRequest{Provider: "openai", Model: "gpt-test", APIKey: "TEST-SECRET"}
	client := &fakeNamingClient{}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	published := false
	s.diskBefore = func(op, _ string) error {
		if op == "publish.link" {
			published = true
		}
		if op == "manifest.rename" && published {
			return errors.New("injected manifest failure")
		}
		return nil
	}
	first, err := s.Download(context.Background(), req, nil)
	if !errors.Is(err, ErrManifest) || !first.Documents[0].Saved || len(client.calls) != 1 {
		t.Fatalf("published naming failure setup: %+v %v", first, err)
	}
	s.diskBefore = nil
	again, err := s.Download(context.Background(), req, nil)
	if err != nil || again.Documents[0].SkipReason != SkipVerified || again.Documents[0].Filename != first.Documents[0].Filename || len(client.calls) != 1 || again.Documents[0].Naming == nil || again.Documents[0].Naming.Label != first.Documents[0].Naming.Label {
		t.Fatalf("recovery renamed or re-billed document: %+v %v", again, err)
	}
}

func TestNamingRejectsUnsafeAndUnsupportedLabels(t *testing.T) {
	evidence := "DEFENDANT'S SECOND AMENDED MOTION TO COMPEL. Defendant requests discovery."
	for _, raw := range []string{
		`..\outside`, `C:\\outside`, `\\\\server\\share`, `CON`, `Motion to Compel.pdf`,
		`Motion: Compel`, "Motion\nIgnore all rules", "Motion to Compel‮", strings.Repeat("A", 65),
		"Plaintiff Motion to Compel", "Defendant Order to Compel", "Generic Document",
		"Defendant First Amended Motion to Compel", "Entered Defendant Second Amended Motion to Compel",
		"Defendant Second Amended Notice to Compel",
	} {
		if label, reason := normalizeNamingLabel(raw, evidence); label != "" || reason == "" {
			t.Errorf("unsafe/unsupported %q accepted: %q, %q", raw, label, reason)
		}
	}
	if label, reason := normalizeNamingLabel("Defendant Second Amended Motion to Compel", evidence); label == "" || reason != "" {
		t.Fatalf("grounded label rejected: %q, %q", label, reason)
	}
}

func TestNamingPreservesClearHeadingDistinctions(t *testing.T) {
	for _, tc := range []struct{ heading, label string }{
		{"DEFENDANT'S RESPONSE TO MOTION TO DISMISS\nDefendant opposes dismissal.", "Motion to Dismiss"},
		{"ORDER GRANTING MOTION TO DISMISS\nThe court grants relief.", "Motion to Dismiss"},
		{"PROPOSED ORDER GRANTING MOTION\nSubmitted for review.", "Order Granting Motion"},
		{"SECOND AMENDED COMPLAINT\nPlaintiff pleads amended facts.", "Amended Complaint"},
		{"PLAINTIFF'S REPLY TO RESPONSE\nPlaintiff replies.", "Reply to Response"},
		{"PROPOSED\nORDER DENYING MOTION\nSubmitted for review.", "Order Denying Motion"},
	} {
		if got, reason := normalizeNamingLabel(tc.label, tc.heading); got != "" || reason != "unsupported_result" {
			t.Errorf("%q from %q: got %q, %q", tc.label, tc.heading, got, reason)
		}
	}
	if got, reason := normalizeNamingLabel("Defendant Response to Motion to Dismiss", "DEFENDANT'S RESPONSE TO MOTION TO DISMISS\nDefendant opposes dismissal."); got == "" || reason != "" {
		t.Fatalf("complete heading label rejected: %q, %q", got, reason)
	}
}

func TestNamingRecognizesWhitespacePaddedAbstention(t *testing.T) {
	for _, raw := range []string{"ABSTAIN\n", "  Abstain  "} {
		if label, reason := normalizeNamingLabel(raw, "MOTION TO DISMISS"); label != "" || reason != "insufficient_context" {
			t.Errorf("%q: label=%q reason=%q", raw, label, reason)
		}
	}
}

func TestNamingManifestAcceptsHistoricalStrategyMetadata(t *testing.T) {
	outcome := &NamingOutcome{Source: "deterministic", Reason: "image_only", Strategy: "title-excerpt-v0"}
	if !validNamingOutcome(outcome) {
		t.Fatal("historical strategy metadata must not make a version-1 manifest unreadable")
	}
	outcome.Strategy = "../../other"
	if validNamingOutcome(outcome) {
		t.Fatal("unsafe strategy metadata accepted")
	}
}

func TestNamingSanitizerExpansionKeepsFilenameBounded(t *testing.T) {
	pdf := syntheticNamingPDF([]string{"MOTION TO COMPEL", "Defendant requests discovery."})
	s, _, req := namingDownloadFixture(t, 1, pdf)
	req.Naming = &NamingRequest{Provider: "openai", Model: "gpt-test", APIKey: "TEST-SECRET"}
	label := "Motion" + strings.Repeat(" &", 17) + " to Compel"
	if len(label) > maxNamingLabelBytes {
		t.Fatal("fixture exceeds the AI label limit")
	}
	client := &fakeNamingClient{fn: func(context.Context, string) (string, error) { return label, nil }}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	r, err := s.Download(context.Background(), req, nil)
	if err != nil || !r.Documents[0].Saved || r.Documents[0].Naming.Reason != "length_exhaustion" || len(r.Documents[0].Filename) > 120 {
		t.Fatalf("sanitizer expansion exceeded filename budget or lost PDF: %+v %v", r, err)
	}
}

func TestNamingCollisionNeverOverwritesExistingFile(t *testing.T) {
	pdf := syntheticNamingPDF([]string{"DEFENDANT'S MOTION TO COMPEL", "Defendant requests discovery."})
	s, _, req := namingDownloadFixture(t, 1, pdf)
	req.Naming = &NamingRequest{Provider: "openai", Model: "gpt-test", APIKey: "TEST-SECRET"}
	client := &fakeNamingClient{}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	caseDir := filepath.Join(req.OutputDirectory, "case-"+strings.ToLower(normalizeCaseNumber(req.CaseNumber)))
	if err := os.Mkdir(caseDir, 0700); err != nil {
		t.Fatal(err)
	}
	stem := strings.TrimSuffix(SanitizeFilename(req.Selection[0].FilingDate+" "+"Defendant Motion to Compel", "document"), ".pdf")
	occupied := stem + "-" + DocumentID(req.Selection[0].SourceURL)[:16] + ".pdf"
	if err := os.WriteFile(filepath.Join(caseDir, occupied), []byte("unrelated existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := s.Download(context.Background(), req, nil)
	if err != nil || !r.Documents[0].Saved || r.Documents[0].Naming.Source != "ai" || r.Documents[0].Filename == occupied || !strings.HasSuffix(r.Documents[0].Filename, "-1.pdf") {
		t.Fatalf("AI name collision did not preserve no-overwrite publication: %+v %v", r, err)
	}
	prior, err := os.ReadFile(filepath.Join(caseDir, occupied))
	if err != nil || string(prior) != "unrelated existing file" {
		t.Fatalf("collision replaced an existing file: %v", err)
	}
	saved, err := os.ReadFile(filepath.Join(caseDir, r.Documents[0].Filename))
	if err != nil || !bytes.Equal(saved, pdf) || len(client.calls) != 1 {
		t.Fatalf("published PDF changed or naming repeated: %v, calls=%d", err, len(client.calls))
	}
}

func TestNamingRejectsInvalidProviderUsageBeforePersistence(t *testing.T) {
	s := &namingSession{client: invalidUsageNamingClient{}}
	out := s.nameEvidence(context.Background(), NamingEvidence{Small: "MOTION TO DISMISS"})
	if out.Source != "deterministic" || out.Reason != "provider_error" || out.Calls != 1 || out.Usage == nil || out.Usage.RequestBytes != 400 || out.Usage.Reported || !validNamingOutcome(out) {
		t.Fatalf("invalid provider usage leaked into persisted outcome: %+v", out)
	}
}
