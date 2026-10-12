package arcourt

import (
	"strings"
	"testing"
)

// This corpus was authored for the title-excerpt-v2 validator. None of its
// PDFs, titles, or qualifier words were used while tuning that validator, and
// none appeared in the v3 or v4 reports. Its aliases and scoring rule must be
// committed before running a model against it. Do not adjust production
// extraction or validation in response to its results; that would retire it as
// held-out evidence and require another new corpus.
func namingBenchmarkCorpusV5() []namingBenchCase {
	readable := func(id, label string, aliases, must []string, pages ...[]string) namingBenchCase {
		c := readableCase(id, "held_out", label, must, []string{strings.ToLower(pages[0][0])}, pages...)
		c.Acceptable = append(c.Acceptable, aliases...)
		return c
	}
	return []namingBenchCase{
		readable("v5-motion-limine", "Motion in Limine", nil, []string{"motion", "limine"}, []string{"MOTION IN LIMINE", "Movant asks to exclude the described proof."}),
		readable("v5-order-grant-limine", "Order Granting Motion in Limine", nil, []string{"order", "granting", "motion", "limine"}, []string{"ORDER GRANTING MOTION IN LIMINE", "The court excludes the described proof."}),
		readable("v5-response-limine", "Plaintiff Response to Motion in Limine", []string{"Plaintiff's Response to Motion in Limine"}, []string{"plaintiff", "response", "motion", "limine"}, []string{"PLAINTIFF'S RESPONSE TO MOTION IN LIMINE", "Plaintiff opposes exclusion of the proof."}),
		readable("v5-motion-new-trial", "Motion for New Trial", nil, []string{"motion", "new", "trial"}, []string{"MOTION FOR NEW TRIAL", "Movant asks the court to retry the matter."}),
		readable("v5-order-deny-new-trial", "Order Denying Motion for New Trial", nil, []string{"order", "denying", "motion", "new", "trial"}, []string{"ORDER DENYING MOTION FOR NEW TRIAL", "The request to retry the matter is denied."}),
		readable("v5-reply-new-trial", "Reply in Support of Motion for New Trial", nil, []string{"reply", "support", "motion", "new", "trial"}, []string{"REPLY IN SUPPORT OF MOTION FOR NEW TRIAL", "Movant answers the opposition and renews the request."}),
		readable("v5-sixth-amended-complaint", "Sixth Amended Complaint", nil, []string{"sixth", "amended", "complaint"}, []string{"SIXTH AMENDED COMPLAINT", "Plaintiff files a sixth version of the pleading."}),
		readable("v5-third-amended-answer", "Third Amended Answer", nil, []string{"third", "amended", "answer"}, []string{"THIRD AMENDED ANSWER", "Defendant states a third version of the responsive pleading."}),
		readable("v5-proposed-receiver", "Proposed Order Appointing Receiver", nil, []string{"proposed", "order", "appointing", "receiver"}, []string{"PROPOSED ORDER APPOINTING RECEIVER", "Submitted for the court's consideration; not entered."}),
		readable("v5-entered-confirming-sale", "Entered Order Confirming Sale", nil, []string{"entered", "order", "confirming", "sale"}, []string{"ENTERED ORDER CONFIRMING SALE", "The court confirms the reported sale."}),
		readable("v5-verified-complaint-replevin", "Verified Complaint for Replevin", nil, []string{"verified", "complaint", "replevin"}, []string{"VERIFIED COMPLAINT FOR REPLEVIN", "The plaintiff verifies the claim to the property."}),
		readable("v5-petition-habeas", "Petition for Writ of Habeas Corpus", nil, []string{"petition", "writ", "habeas", "corpus"}, []string{"PETITION FOR WRIT OF HABEAS CORPUS", "Petitioner challenges the detention."}),
		readable("v5-notice-lis-pendens", "Notice of Lis Pendens", nil, []string{"notice", "lis", "pendens"}, []string{"NOTICE OF LIS PENDENS", "Notice is given that the described land is in litigation."}),
		readable("v5-notice-filing-transcript", "Notice of Filing Transcript", nil, []string{"notice", "filing", "transcript"}, []string{"NOTICE OF FILING TRANSCRIPT", "The reporter's transcript is filed with the clerk."}),
		readable("v5-affidavit-indigency", "Affidavit of Indigency", nil, []string{"affidavit", "indigency"}, []string{"AFFIDAVIT OF INDIGENCY", "The affiant states an inability to pay costs."}),
		readable("v5-declaration-lost-instrument", "Declaration of Lost Instrument", nil, []string{"declaration", "lost", "instrument"}, []string{"DECLARATION OF LOST INSTRUMENT", "Declarant states the original cannot be produced."}),
		readable("v5-certificate-compliance", "Certificate of Compliance", nil, []string{"certificate", "compliance"}, []string{"CERTIFICATE OF COMPLIANCE", "Counsel certifies the filing meets the length limit."}),
		readable("v5-objection-exemption", "Respondent Objection to Claim of Exemption", []string{"Respondent's Objection to Claim of Exemption"}, []string{"respondent", "objection", "claim", "exemption"}, []string{"RESPONDENT'S OBJECTION TO CLAIM OF EXEMPTION", "Respondent disputes the claimed exemption."}),
		readable("v5-motion-strike-jury", "Motion to Strike Jury Demand", nil, []string{"motion", "strike", "jury", "demand"}, []string{"MOTION TO STRIKE JURY DEMAND", "Movant asks that the jury request be struck."}),
		readable("v5-memorandum-opinion", "Memorandum Opinion and Order", nil, []string{"memorandum", "opinion", "order"}, []string{"MEMORANDUM OPINION AND ORDER", "The court explains its reasoning and directs relief."}),
		readable("v5-stipulation-protective", "Stipulation for Protective Order", nil, []string{"stipulation", "protective", "order"}, []string{"STIPULATION FOR PROTECTIVE ORDER", "The parties agree to limits on disclosure."}),
		readable("v5-subpoena-deposition", "Subpoena for Deposition Testimony", nil, []string{"subpoena", "deposition", "testimony"}, []string{"SUBPOENA FOR DEPOSITION TESTIMONY", "The witness must appear and testify."}),
		readable("v5-writ-possession", "Writ of Possession", nil, []string{"writ", "possession"}, []string{"WRIT OF POSSESSION", "The sheriff shall restore possession of the premises."}),
		readable("v5-consent-judgment", "Consent Judgment", nil, []string{"consent", "judgment"}, []string{"CONSENT JUDGMENT", "The parties agree to the terms recited here."}),
		readable("v5-application-default", "Application for Default", nil, []string{"application", "default"}, []string{"APPLICATION FOR DEFAULT", "Applicant reports no responsive pleading was served."}),
		readable("v5-summons-amended-complaint", "Summons on Amended Complaint", nil, []string{"summons", "amended", "complaint"}, []string{"SUMMONS ON AMENDED COMPLAINT", "You are summoned to answer the amended pleading."}),
		readable("v5-exhibit-c", "Exhibit C", nil, []string{"exhibit", "c"}, []string{"EXHIBIT C", "Attached is a synthetic ledger page."}),
		readable("v5-answer-counterclaim", "Defendant Answer to Counterclaim", []string{"Defendant's Answer to Counterclaim"}, []string{"defendant", "answer", "counterclaim"}, []string{"DEFENDANT'S ANSWER TO COUNTERCLAIM", "Defendant responds to the asserted counterclaim."}),
		readable("v5-order-deny-quash", "Order Denying Motion to Quash", nil, []string{"order", "denying", "motion", "quash"}, []string{"ORDER DENYING MOTION TO QUASH", "The request to quash is denied."}),
		readable("v5-response-show-cause", "Response to Order to Show Cause", nil, []string{"response", "order", "show", "cause"}, []string{"RESPONSE TO ORDER TO SHOW CAUSE", "The responding party explains the asserted failure."}),
		readable("v5-motion-set-aside-default", "Motion to Set Aside Default Judgment", nil, []string{"motion", "set", "aside", "default", "judgment"}, []string{"MOTION TO SET ASIDE DEFAULT JUDGMENT", "Movant asks that the prior judgment be set aside."}),
		readable("v5-brief-reply-opposition", "Brief in Reply to Opposition", nil, []string{"brief", "reply", "opposition"}, []string{"BRIEF IN REPLY TO OPPOSITION", "The brief answers the arguments raised in opposition."}),
		ambiguousCase("v5-counsel-cover", "held_out", []string{"Counsel for the parties:", "The enclosed pages accompany today's filing.", "No filing type is identified."}),
		ambiguousCase("v5-exhibit-index", "held_out", []string{"INDEX", "Items are listed by number without a filing title."}),
		ambiguousCase("v5-quoted-order", "held_out", []string{"The prior ORDER GRANTING CONTINUANCE is reproduced below.", "This page itself carries no heading."}),
		ambiguousCase("v5-signature-page", "held_out", []string{"SIGNATURE PAGE", "Only signature blocks appear on this sheet."}),
		ambiguousCase("v5-fax-cover", "held_out", []string{"FAX COVER SHEET", "Pages are transmitted without naming the filing."}),
		ambiguousCase("v5-illegible-heading", "held_out", []string{"ILLEGIBLE", "The heading cannot be determined from this page."}),
		ambiguousCase("v5-continued-page", "held_out", []string{"CONTINUED FROM PREVIOUS PAGE", "The document heading appeared on an earlier sheet."}),
		ambiguousCase("v5-mailing-label", "held_out", []string{"MAILING LABEL", "An address block appears with no filing type."}),
		{ID: "v5-scan-a", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(17)},
		{ID: "v5-scan-b", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(18)},
		{ID: "v5-scan-c", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(19)},
		{ID: "v5-scan-d", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(20)},
		{ID: "v5-scan-e", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(21)},
		{ID: "v5-scan-f", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(22)},
		{ID: "v5-scan-g", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(23)},
		{ID: "v5-scan-h", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(24)},
	}
}

func TestNamingBenchmarkV5CorpusSize(t *testing.T) {
	counts := map[string]int{}
	for _, c := range namingBenchmarkCorpusV5() {
		counts[c.Cohort]++
		if c.Split != "held_out" {
			t.Fatalf("v5 case %s is not held out", c.ID)
		}
	}
	if counts["readable"] != 32 || counts["ambiguous"] != 8 || counts["scanned"] != 8 {
		t.Fatalf("unexpected v5 cohort counts: %v", counts)
	}
}

// The v5 titles must be new work, not a restatement of an earlier corpus.
func TestNamingBenchmarkV5TitlesAreUnseen(t *testing.T) {
	seen := map[string]string{}
	for _, c := range append(namingBenchmarkCorpus(), namingBenchmarkCorpusV4()...) {
		for _, label := range c.Acceptable {
			seen[namingBenchmarkCanonicalV4(label)] = c.ID
		}
	}
	for _, c := range namingBenchmarkCorpusV5() {
		for _, label := range c.Acceptable {
			if prior, ok := seen[namingBenchmarkCanonicalV4(label)]; ok {
				t.Errorf("v5 case %s reuses label %q from %s", c.ID, label, prior)
			}
		}
	}
}

func TestNamingBenchmarkV5Scoring(t *testing.T) {
	var sample namingBenchCase
	for _, c := range namingBenchmarkCorpusV5() {
		if c.ID == "v5-response-limine" {
			sample = c
			break
		}
	}
	if sample.ID == "" {
		t.Fatal("scoring fixture missing")
	}
	for _, tc := range []struct {
		label string
		want  bool
	}{
		{"Plaintiff Response to Motion in Limine", true},
		{"PLAINTIFF'S RESPONSE TO MOTION IN LIMINE", true},
		{"Plaintiff   Response to Motion in Limine", true},
		{"Defendant Response to Motion in Limine", false},
		{"Plaintiff Reply to Motion in Limine", false},
		{"Plaintiff Response to Motion to Dismiss", false},
		{"Plaintiff Emergency Response to Motion in Limine", false},
	} {
		if got := namingBenchmarkCorrectLabel("synthetic-court-v5", sample, tc.label); got != tc.want {
			t.Errorf("v5 score %q = %v, want %v", tc.label, got, tc.want)
		}
	}
}
