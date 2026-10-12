package arcourt

import (
	"strings"
	"testing"
)

// This corpus was authored for the title-excerpt-v3 validator, which adds
// heading word order to the preservation rule. None of its PDFs, titles, or
// qualifier words were used while writing that rule, and none appeared in the
// v3, v4, or v5 reports. Its aliases and scoring rule must be committed before
// running a model against it. Do not adjust production extraction or validation
// in response to its results; that would retire it as held-out evidence and
// require another new corpus.
func namingBenchmarkCorpusV6() []namingBenchCase {
	readable := func(id, label string, aliases, must []string, pages ...[]string) namingBenchCase {
		c := readableCase(id, "held_out", label, must, []string{strings.ToLower(pages[0][0])}, pages...)
		c.Acceptable = append(c.Acceptable, aliases...)
		return c
	}
	return []namingBenchCase{
		readable("v6-motion-strike-defenses", "Motion to Strike Affirmative Defenses", nil, []string{"motion", "strike", "affirmative", "defenses"}, []string{"MOTION TO STRIKE AFFIRMATIVE DEFENSES", "Movant asks that the listed defenses be struck."}),
		readable("v6-order-strike-defenses", "Order Striking Affirmative Defenses", nil, []string{"order", "striking", "affirmative", "defenses"}, []string{"ORDER STRIKING AFFIRMATIVE DEFENSES", "The listed defenses are struck."}),
		readable("v6-objection-motion-strike", "Plaintiff Objection to Motion to Strike", []string{"Plaintiff's Objection to Motion to Strike"}, []string{"plaintiff", "objection", "motion", "strike"}, []string{"PLAINTIFF'S OBJECTION TO MOTION TO STRIKE", "Plaintiff opposes striking the pleaded matter."}),
		readable("v6-motion-protective-order", "Motion for Protective Order", nil, []string{"motion", "protective", "order"}, []string{"MOTION FOR PROTECTIVE ORDER", "Movant asks the court to limit the described disclosure."}),
		readable("v6-order-sealing-exhibits", "Order Sealing Exhibits", nil, []string{"order", "sealing", "exhibits"}, []string{"ORDER SEALING EXHIBITS", "The listed attachments are withheld from public view."}),
		readable("v6-stipulation-extend", "Stipulation to Extend Deadline", nil, []string{"stipulation", "extend", "deadline"}, []string{"STIPULATION TO EXTEND DEADLINE", "The parties agree to move the stated date."}),
		readable("v6-order-approving-stipulation", "Order Approving Stipulation", nil, []string{"order", "approving", "stipulation"}, []string{"ORDER APPROVING STIPULATION", "The agreement of the parties is approved."}),
		readable("v6-seventh-amended-complaint", "Seventh Amended Complaint", nil, []string{"seventh", "amended", "complaint"}, []string{"SEVENTH AMENDED COMPLAINT", "Plaintiff pleads a further revised version."}),
		readable("v6-motion-compel-arbitration", "Motion to Compel Arbitration", nil, []string{"motion", "compel", "arbitration"}, []string{"MOTION TO COMPEL ARBITRATION", "Movant asks that the dispute be sent to arbitration."}),
		readable("v6-order-compel-arbitration", "Order Compelling Arbitration", nil, []string{"order", "compelling", "arbitration"}, []string{"ORDER COMPELLING ARBITRATION", "The dispute is sent to arbitration."}),
		readable("v6-notice-change-address", "Notice of Change of Address", nil, []string{"notice", "change", "address"}, []string{"NOTICE OF CHANGE OF ADDRESS", "The filing party reports a new mailing location."}),
		readable("v6-motion-remand", "Motion to Remand", nil, []string{"motion", "remand"}, []string{"MOTION TO REMAND", "Movant asks that the matter be returned."}),
		readable("v6-order-remanding", "Order Remanding Case", nil, []string{"order", "remanding", "case"}, []string{"ORDER REMANDING CASE", "The matter is returned to the prior court."}),
		readable("v6-affidavit-residency", "Affidavit of Residency", nil, []string{"affidavit", "residency"}, []string{"AFFIDAVIT OF RESIDENCY", "The affiant swears to living at the stated location."}),
		readable("v6-motion-sanctions", "Motion for Sanctions", nil, []string{"motion", "sanctions"}, []string{"MOTION FOR SANCTIONS", "Movant asks the court to penalize the described conduct."}),
		readable("v6-order-awarding-costs", "Order Awarding Costs", nil, []string{"order", "awarding", "costs"}, []string{"ORDER AWARDING COSTS", "The prevailing party recovers the stated expenses."}),
		readable("v6-notice-voluntary-dismissal", "Notice of Voluntary Dismissal", nil, []string{"notice", "voluntary", "dismissal"}, []string{"NOTICE OF VOLUNTARY DISMISSAL", "The filing party ends the claim without a ruling."}),
		readable("v6-motion-consolidate", "Motion to Consolidate Cases", nil, []string{"motion", "consolidate", "cases"}, []string{"MOTION TO CONSOLIDATE CASES", "Movant asks that the related matters be joined."}),
		readable("v6-verified-petition-guardianship", "Verified Petition for Guardianship", nil, []string{"verified", "petition", "guardianship"}, []string{"VERIFIED PETITION FOR GUARDIANSHIP", "The sworn petition asks that a guardian be named."}),
		readable("v6-order-appointing-guardian", "Order Appointing Guardian", nil, []string{"order", "appointing", "guardian"}, []string{"ORDER APPOINTING GUARDIAN", "A guardian is named for the protected person."}),
		readable("v6-motion-withdraw-counsel", "Motion to Withdraw as Counsel", nil, []string{"motion", "withdraw", "counsel"}, []string{"MOTION TO WITHDRAW AS COUNSEL", "Counsel asks to be relieved of the representation."}),
		readable("v6-order-withdrawal-counsel", "Order Granting Withdrawal of Counsel", nil, []string{"order", "granting", "withdrawal", "counsel"}, []string{"ORDER GRANTING WITHDRAWAL OF COUNSEL", "Counsel is relieved of the representation."}),
		readable("v6-exhibit-d", "Exhibit D", nil, []string{"exhibit", "d"}, []string{"EXHIBIT D", "Attached is a synthetic inventory page."}),
		readable("v6-notice-lien", "Notice of Lien", nil, []string{"notice", "lien"}, []string{"NOTICE OF LIEN", "A claim against the described property is recorded."}),
		readable("v6-motion-summary-disposition", "Motion for Summary Disposition", nil, []string{"motion", "summary", "disposition"}, []string{"MOTION FOR SUMMARY DISPOSITION", "Movant asks for decision without a trial."}),
		readable("v6-order-deny-summary-disposition", "Order Denying Summary Disposition", nil, []string{"order", "denying", "summary", "disposition"}, []string{"ORDER DENYING SUMMARY DISPOSITION", "Decision without a trial is refused."}),
		readable("v6-supplemental-brief", "Defendant Supplemental Brief", []string{"Defendant's Supplemental Brief"}, []string{"defendant", "supplemental", "brief"}, []string{"DEFENDANT'S SUPPLEMENTAL BRIEF", "Defendant adds argument on the pending question."}),
		readable("v6-proposed-order-settlement", "Proposed Order Approving Settlement", nil, []string{"proposed", "order", "approving", "settlement"}, []string{"PROPOSED ORDER APPROVING SETTLEMENT", "Submitted for the court's signature."}),
		readable("v6-motion-bifurcate", "Motion to Bifurcate Trial", nil, []string{"motion", "bifurcate", "trial"}, []string{"MOTION TO BIFURCATE TRIAL", "Movant asks that the issues be tried separately."}),
		readable("v6-order-bifurcating", "Order Bifurcating Trial", nil, []string{"order", "bifurcating", "trial"}, []string{"ORDER BIFURCATING TRIAL", "The issues will be tried separately."}),
		readable("v6-notice-entry-judgment", "Notice of Entry of Judgment", nil, []string{"notice", "entry", "judgment"}, []string{"NOTICE OF ENTRY OF JUDGMENT", "The parties are told the decision was recorded."}),
		readable("v6-motion-attorney-fees", "Motion for Attorney Fees", nil, []string{"motion", "attorney", "fees"}, []string{"MOTION FOR ATTORNEY FEES", "Movant asks to recover the cost of representation."}),
		ambiguousCase("v6-table-of-contents", "held_out", []string{"TABLE OF CONTENTS", "Sections are listed without naming this filing."}),
		ambiguousCase("v6-separator-sheet", "held_out", []string{"SEPARATOR SHEET", "This page divides two attachments."}),
		ambiguousCase("v6-envelope-image", "held_out", []string{"RETURN ADDRESS", "An envelope image appears with no filing type."}),
		ambiguousCase("v6-control-number", "held_out", []string{"000123", "Only a control number appears on this page."}),
		ambiguousCase("v6-handwritten-note", "held_out", []string{"HANDWRITTEN NOTE", "The writing does not state a filing type."}),
		ambiguousCase("v6-redacted-page", "held_out", []string{"REDACTED", "The content of this page has been withheld."}),
		ambiguousCase("v6-transmittal-stub", "held_out", []string{"TRANSMITTAL", "Documents are forwarded without a stated title."}),
		ambiguousCase("v6-quoted-continuance", "held_out", []string{"The earlier MOTION FOR CONTINUANCE is quoted below.", "This page itself carries no heading."}),
		{ID: "v6-scan-a", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(25)},
		{ID: "v6-scan-b", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(26)},
		{ID: "v6-scan-c", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(27)},
		{ID: "v6-scan-d", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(28)},
		{ID: "v6-scan-e", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(29)},
		{ID: "v6-scan-f", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(30)},
		{ID: "v6-scan-g", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(31)},
		{ID: "v6-scan-h", Split: "held_out", Cohort: "scanned", PDF: syntheticScan(32)},
	}
}

func TestNamingBenchmarkV6CorpusSize(t *testing.T) {
	counts := map[string]int{}
	for _, c := range namingBenchmarkCorpusV6() {
		counts[c.Cohort]++
		if c.Split != "held_out" {
			t.Fatalf("v6 case %s is not held out", c.ID)
		}
	}
	if counts["readable"] != 32 || counts["ambiguous"] != 8 || counts["scanned"] != 8 {
		t.Fatalf("unexpected v6 cohort counts: %v", counts)
	}
}

// The v6 titles must be new work, not a restatement of an earlier corpus.
func TestNamingBenchmarkV6TitlesAreUnseen(t *testing.T) {
	seen := map[string]string{}
	prior := append(namingBenchmarkCorpus(), namingBenchmarkCorpusV4()...)
	prior = append(prior, namingBenchmarkCorpusV5()...)
	for _, c := range prior {
		for _, label := range c.Acceptable {
			seen[namingBenchmarkCanonicalV4(label)] = c.ID
		}
	}
	for _, c := range namingBenchmarkCorpusV6() {
		for _, label := range c.Acceptable {
			if from, ok := seen[namingBenchmarkCanonicalV4(label)]; ok {
				t.Errorf("v6 case %s reuses label %q from %s", c.ID, label, from)
			}
		}
	}
}

// Rearranging the title words names a different filing, so the scoring rule
// must reject a reordering even when every material word survives.
func TestNamingBenchmarkV6Scoring(t *testing.T) {
	var sample namingBenchCase
	for _, c := range namingBenchmarkCorpusV6() {
		if c.ID == "v6-objection-motion-strike" {
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
		{"Plaintiff Objection to Motion to Strike", true},
		{"PLAINTIFF'S OBJECTION TO MOTION TO STRIKE", true},
		{"Plaintiff   Objection to Motion to Strike", true},
		{"Defendant Objection to Motion to Strike", false},
		{"Plaintiff Response to Motion to Strike", false},
		{"Plaintiff Objection to Motion to Dismiss", false},
		{"Motion to Strike Objection of Plaintiff", false},
	} {
		if got := namingBenchmarkCorrectLabel("synthetic-court-v6", sample, tc.label); got != tc.want {
			t.Errorf("v6 score %q = %v, want %v", tc.label, got, tc.want)
		}
	}
}
