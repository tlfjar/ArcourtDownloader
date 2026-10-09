package arcourt

import (
	"strings"
	"testing"
)

// This corpus was authored after reviewing the v3 model outputs. None of its
// PDFs appeared in either prior live report. Its aliases and scoring rule must
// be committed before running a model against it.
func namingBenchmarkCorpusV4() []namingBenchCase {
	readable := func(id, label string, aliases, must []string, pages ...[]string) namingBenchCase {
		c := readableCase(id, "held_out", label, must, []string{strings.ToLower(pages[0][0])}, pages...)
		c.Acceptable = append(c.Acceptable, aliases...)
		return c
	}
	return []namingBenchCase{
		readable("v4-motion-strike-answer", "Motion to Strike Answer", nil, []string{"motion", "strike", "answer"}, []string{"MOTION TO STRIKE ANSWER", "Movant asks to strike the answer."}),
		readable("v4-response-strike-answer", "Defendant Response to Motion to Strike Answer", []string{"Defendant's Response to Motion to Strike Answer"}, []string{"defendant", "response", "strike", "answer"}, []string{"DEFENDANT'S RESPONSE TO MOTION TO STRIKE ANSWER", "Defendant opposes the motion."}),
		readable("v4-reply-strike-answer", "Plaintiff Reply to Defendant Response", []string{"Plaintiff's Reply to Defendant's Response"}, []string{"plaintiff", "reply", "defendant", "response"}, []string{"PLAINTIFF'S REPLY TO DEFENDANT'S RESPONSE", "Plaintiff replies to the response on the motion to strike."}),
		readable("v4-order-partial-summary", "Order Granting Partial Summary Judgment", nil, []string{"order", "granting", "partial", "summary", "judgment"}, []string{"ORDER GRANTING PARTIAL SUMMARY JUDGMENT", "The court grants the motion in part."}),
		readable("v4-order-deny-recusal", "Order Denying Recusal", nil, []string{"order", "denying", "recusal"}, []string{"ORDER DENYING RECUSAL", "The request for recusal is denied."}),
		readable("v4-proposed-sealing-order", "Proposed Order Sealing Exhibits", nil, []string{"proposed", "order", "sealing", "exhibits"}, []string{"PROPOSED ORDER SEALING EXHIBITS", "Submitted for the court's consideration; not entered."}),
		readable("v4-entered-vacate-hearing", "Entered Order Vacating Hearing", nil, []string{"entered", "order", "vacating", "hearing"}, []string{"ENTERED ORDER VACATING HEARING", "The hearing is vacated by the court."}),
		readable("v4-fifth-amended-complaint", "Fifth Amended Complaint", nil, []string{"fifth", "amended", "complaint"}, []string{"FIFTH AMENDED COMPLAINT", "Plaintiff files a fifth version of the complaint."}),
		readable("v4-amended-answer-defenses", "Amended Answer and Affirmative Defenses", nil, []string{"amended", "answer", "affirmative", "defenses"}, []string{"AMENDED ANSWER AND AFFIRMATIVE DEFENSES", "Defendant states the amended defenses."}),
		readable("v4-verified-mandamus-petition", "Verified Petition for Writ of Mandamus", nil, []string{"verified", "petition", "writ", "mandamus"}, []string{"VERIFIED PETITION FOR WRIT OF MANDAMUS", "Petitioner verifies the allegations."}),
		readable("v4-counterclaim-fraud", "Answer and Counterclaim for Fraud", nil, []string{"answer", "counterclaim", "fraud"}, []string{"ANSWER AND COUNTERCLAIM FOR FRAUD", "Defendant answers and alleges a fraud counterclaim."}),
		readable("v4-notice-substitution", "Notice of Substitution of Counsel", nil, []string{"notice", "substitution", "counsel"}, []string{"NOTICE OF SUBSTITUTION OF COUNSEL", "New counsel appears for the defendant."}),
		readable("v4-notice-removal", "Notice of Removal", nil, []string{"notice", "removal"}, []string{"NOTICE OF REMOVAL", "Defendant gives notice of removal."}),
		readable("v4-affidavit-diligence", "Affidavit of Due Diligence", nil, []string{"affidavit", "due", "diligence"}, []string{"AFFIDAVIT OF DUE DILIGENCE", "The affiant describes service attempts."}),
		readable("v4-declaration-electronic-service", "Declaration of Electronic Service", nil, []string{"declaration", "electronic", "service"}, []string{"DECLARATION OF ELECTRONIC SERVICE", "Declarant served the filing electronically."}),
		readable("v4-certificate-conference", "Certificate of Conference", nil, []string{"certificate", "conference"}, []string{"CERTIFICATE OF CONFERENCE", "Counsel conferred before filing."}),
		readable("v4-brief-opposition-protective", "Brief in Opposition to Protective Order", nil, []string{"brief", "opposition", "protective", "order"}, []string{"BRIEF IN OPPOSITION TO PROTECTIVE ORDER", "The brief opposes the proposed protective order."}),
		readable("v4-objection-magistrate", "Objection to Magistrate Report", nil, []string{"objection", "magistrate", "report"}, []string{"OBJECTION TO MAGISTRATE REPORT", "The party objects to the report."}),
		readable("v4-motion-continue-trial", "Motion to Continue Trial", nil, []string{"motion", "continue", "trial"}, []string{"MOTION TO CONTINUE TRIAL", "Movant seeks a later trial date."}),
		readable("v4-notice-withdraw-appeal", "Notice of Withdrawal of Appeal", nil, []string{"notice", "withdrawal", "appeal"}, []string{"NOTICE OF WITHDRAWAL OF APPEAL", "Appellant withdraws the appeal."}),
		readable("v4-stipulation-extension", "Stipulation to Extend Deadlines", nil, []string{"stipulation", "extend", "deadlines"}, []string{"STIPULATION TO EXTEND DEADLINES", "The parties stipulate to new deadlines."}),
		readable("v4-subpoena-records", "Subpoena for Business Records", nil, []string{"subpoena", "business", "records"}, []string{"SUBPOENA FOR BUSINESS RECORDS", "The custodian must produce business records."}),
		readable("v4-writ-execution", "Writ of Execution", nil, []string{"writ", "execution"}, []string{"WRIT OF EXECUTION", "The sheriff is directed to execute the judgment."}),
		readable("v4-default-damages", "Default Judgment on Damages", nil, []string{"default", "judgment", "damages"}, []string{"DEFAULT JUDGMENT ON DAMAGES", "The court enters damages following default."}),
		readable("v4-application-costs", "Application for Taxable Costs", nil, []string{"application", "taxable", "costs"}, []string{"APPLICATION FOR TAXABLE COSTS", "Applicant requests taxable costs."}),
		readable("v4-memorandum-compel", "Memorandum in Support of Motion to Compel", nil, []string{"memorandum", "support", "motion", "compel"}, []string{"MEMORANDUM IN SUPPORT OF MOTION TO COMPEL", "This memorandum supports the motion to compel."}),
		readable("v4-order-dismiss-without-prejudice", "Order Dismissing Without Prejudice", nil, []string{"order", "dismissing", "without", "prejudice"}, []string{"ORDER DISMISSING WITHOUT PREJUDICE", "The matter is dismissed without prejudice."}),
		readable("v4-proposed-findings", "Proposed Order and Findings of Fact", nil, []string{"proposed", "order", "findings", "fact"}, []string{"PROPOSED ORDER AND FINDINGS OF FACT", "Counsel submits proposed findings for consideration."}),
		readable("v4-motion-reconsider", "Motion for Reconsideration", nil, []string{"motion", "reconsideration"}, []string{"MOTION FOR RECONSIDERATION", "Movant requests reconsideration of the prior order."}),
		readable("v4-response-contempt", "Respondent Response to Motion for Contempt", []string{"Respondent's Response to Motion for Contempt"}, []string{"respondent", "response", "motion", "contempt"}, []string{"RESPONDENT'S RESPONSE TO MOTION FOR CONTEMPT", "Respondent opposes the motion."}),
		readable("v4-second-amended-protection", "Second Amended Motion for Protective Order", nil, []string{"second", "amended", "motion", "protective", "order"}, []string{"SECOND AMENDED MOTION FOR PROTECTIVE ORDER", "Movant requests a protective order."}),
		readable("v4-order-grant-in-part", "Order Granting in Part Motion to Compel", nil, []string{"order", "granting", "part", "motion", "compel"}, []string{"ORDER GRANTING IN PART MOTION TO COMPEL", "The court grants only part of the motion."}),
		ambiguousCase("v4-untitled-transmittal", "held_out", []string{"To the filing clerk:", "Enclosed are documents for the matter.", "The document type is not stated."}),
		ambiguousCase("v4-quoted-motion", "held_out", []string{"The prior MOTION FOR RECONSIDERATION is quoted below.", "This unsigned page has no filing title."}),
		ambiguousCase("v4-caption-only", "held_out", []string{"IN THE CIRCUIT COURT", "CASE NO. 60CV-2026-88", "PLAINTIFF v. DEFENDANT"}),
		ambiguousCase("v4-unlabeled-attachment", "held_out", []string{"ATTACHMENT B", "The enclosure is not independently titled."}),
		ambiguousCase("v4-redacted-heading", "held_out", []string{"REDACTED", "The filing heading and purpose are not visible."}),
		ambiguousCase("v4-unsigned-memo", "held_out", []string{"MEMORANDUM", "Background facts are summarized without stating the filing purpose."}),
		ambiguousCase("v4-archive-receipt", "held_out", []string{"FILE RECEIPT", "A receipt lists docket entries but does not identify the attached filing."}),
		ambiguousCase("v4-blank-form", "held_out", []string{"COURT FORM", "The selected box and filing type are blank."}),
		{ID: "v4-scan-a", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(9)},
		{ID: "v4-scan-b", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(10)},
		{ID: "v4-scan-c", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(11)},
		{ID: "v4-scan-d", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(12)},
		{ID: "v4-scan-e", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(13)},
		{ID: "v4-scan-f", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(14)},
		{ID: "v4-scan-g", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(15)},
		{ID: "v4-scan-h", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(16)},
	}
}

func TestNamingBenchmarkV4CorpusSize(t *testing.T) {
	counts := map[string]int{}
	for _, c := range namingBenchmarkCorpusV4() {
		counts[c.Cohort]++
		if c.Split != "held_out" {
			t.Fatalf("v4 case %s is not held out", c.ID)
		}
	}
	if counts["readable"] != 32 || counts["ambiguous"] != 8 || counts["scanned"] != 8 {
		t.Fatalf("unexpected v4 cohort counts: %v", counts)
	}
}

func TestNamingBenchmarkV4Scoring(t *testing.T) {
	var sample namingBenchCase
	for _, c := range namingBenchmarkCorpusV4() {
		if c.ID == "v4-response-strike-answer" {
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
		{"Defendant Response to Motion to Strike Answer", true},
		{"DEFENDANT'S RESPONSE TO MOTION TO STRIKE ANSWER", true},
		{"Defendant   Response to Motion to Strike Answer", true},
		{"Plaintiff Response to Motion to Strike Answer", false},
		{"Defendant Reply to Motion to Strike Answer", false},
		{"Defendant Response to Motion to Strike Complaint", false},
		{"Defendant Urgent Response to Motion to Strike Answer", false},
	} {
		if got := namingBenchmarkCorrectLabel("synthetic-court-v4", sample, tc.label); got != tc.want {
			t.Errorf("v4 score %q = %v, want %v", tc.label, got, tc.want)
		}
	}
	for _, old := range namingBenchmarkCorpus() {
		if old.ID == "response-dismiss" && namingBenchmarkCorrectLabel("synthetic-court-v3", old, "Defendant's Response to Motion to Dismiss") {
			t.Fatal("v3 exact-match scoring changed retroactively")
		}
	}
}
