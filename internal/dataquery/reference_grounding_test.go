package dataquery

import (
	"strings"
	"testing"
)

func groundingCandidate() ReferenceKeyCandidate {
	return ReferenceKeyCandidate{
		Path:             "targets.csv",
		Field:            "canonical_label",
		KeyCount:         3,
		Keys:             []string{"GroupA", "GroupX", "GroupC"},
		NonEmptyRowCount: 3,
	}
}

func groundingContribution(group, value string) ContributionRecord {
	return ContributionRecord{
		ItemID:        LooseText("item-" + group),
		Source:        LooseText("observations.csv"),
		SourceLocator: LooseText("row"),
		GroupKey:      LooseText(group),
		Metric:        LooseText("total_value"),
		Value:         LooseText(value),
		Operation:     LooseText("add"),
		Role:          LooseText("target"),
	}
}

// TestEvaluateReferenceGroundingCardinalityArm is the run-1 witness shape
// (replay 2026-07-19): answer "17,5" drops a reference slot — 2 items for 3
// reference keys must violate arm A.
func TestEvaluateReferenceGroundingCardinalityArm(t *testing.T) {
	contributions := []ContributionRecord{
		groundingContribution("GroupA", "17"),
		groundingContribution("GroupC", "5"),
	}
	report, applicable := EvaluateReferenceGrounding(groundingCandidate(), contributions, "17,5")
	if !applicable {
		t.Fatal("grounding must be applicable to a numeric list answer over a resolved reference set")
	}
	if !report.CardinalityMismatch || !report.Violated() {
		t.Fatalf("report=%+v, want cardinality violation for 2 items over 3 reference keys", report)
	}
	detail := DescribeReferenceGroundingMismatches(report)
	if !strings.Contains(detail, "2 item(s)") || !strings.Contains(detail, "3 key(s)") {
		t.Fatalf("detail=%q, want the 2-vs-3 cardinality mismatch spelled out", detail)
	}
}

// TestEvaluateReferenceGroundingPerSlotArm is the run-2 witness shape: answer
// "17,4,5" lets non-reference GroupB's total occupy slot 2, which belongs to
// GroupX with zero contribution records and must be 0.
func TestEvaluateReferenceGroundingPerSlotArm(t *testing.T) {
	contributions := []ContributionRecord{
		groundingContribution("GroupA", "10"),
		groundingContribution("GroupA", "7"),
		groundingContribution("GroupB", "4"),
		groundingContribution("GroupC", "5"),
	}
	report, applicable := EvaluateReferenceGrounding(groundingCandidate(), contributions, "17,4,5")
	if !applicable {
		t.Fatal("grounding must be applicable")
	}
	if report.CardinalityMismatch {
		t.Fatalf("report=%+v, cardinality matches (3==3); the violation is per-slot", report)
	}
	if len(report.Mismatches) != 1 {
		t.Fatalf("mismatches=%+v, want exactly the usurped slot 2", report.Mismatches)
	}
	mismatch := report.Mismatches[0]
	if mismatch.Slot != 2 || mismatch.Key != "GroupX" || mismatch.Expected != "0" || mismatch.Actual != "4" || mismatch.HasContributions {
		t.Fatalf("mismatch=%+v, want slot 2 key GroupX expected 0 actual 4 with no contributions", mismatch)
	}
	if !strings.Contains(DescribeReferenceGroundingMismatches(report), "no contribution records for this key; it must be 0") {
		t.Fatalf("detail=%q, want the zero-for-missing-key rule disclosed", DescribeReferenceGroundingMismatches(report))
	}
}

// TestEvaluateReferenceGroundingAcceptsGroundedAnswer: the fixture truth
// "17,0,5" satisfies both arms — the check is truth-preserving and the
// correct value ships byte-identical.
func TestEvaluateReferenceGroundingAcceptsGroundedAnswer(t *testing.T) {
	contributions := []ContributionRecord{
		groundingContribution("GroupA", "10"),
		groundingContribution("GroupA", "7"),
		groundingContribution("GroupB", "4"),
		groundingContribution("GroupC", "5"),
	}
	report, applicable := EvaluateReferenceGrounding(groundingCandidate(), contributions, "17,0,5")
	if !applicable || report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want a clean pass for the grounded answer", report, applicable)
	}
}

// TestEvaluateReferenceGroundingMixedKeyEchoArm pins the merged-review F1
// escape (adversarial A1, 2026-07-19): "17,GroupX,5" — numeric slots plus one
// key echo standing exactly where the 0 must be — is neither a totals list
// nor a key projection. The echo exemption is uniform: only an answer that
// echoes EVERY slot's key is a pure key projection; a mixed shape violates.
func TestEvaluateReferenceGroundingMixedKeyEchoArm(t *testing.T) {
	contributions := []ContributionRecord{
		groundingContribution("GroupA", "17"),
		groundingContribution("GroupB", "4"),
		groundingContribution("GroupC", "5"),
	}
	report, applicable := EvaluateReferenceGrounding(groundingCandidate(), contributions, "17,GroupX,5")
	if !applicable {
		t.Fatal("mixed key-echo answer must stay applicable")
	}
	if !report.Violated() || len(report.Mismatches) != 1 {
		t.Fatalf("report=%+v, want exactly the echoed slot 2 red (truth is 17,0,5)", report)
	}
	mismatch := report.Mismatches[0]
	if mismatch.Slot != 2 || mismatch.Key != "GroupX" || mismatch.Expected != "0" || mismatch.Actual != "GroupX" {
		t.Fatalf("mismatch=%+v, want slot 2 key GroupX expected 0 actual GroupX", mismatch)
	}
	// The uniform echo lane itself stays open: a full slot-faithful key
	// projection remains clean (the legitimate carve-out motive).
	report, applicable = EvaluateReferenceGrounding(groundingCandidate(), contributions, "GroupA,GroupX,GroupC")
	if !applicable || report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want the pure key projection to stay clean", report, applicable)
	}
}

// TestEvaluateReferenceGroundingPoisonPerKey pins the merged-review F2
// escapes (adversarial A2a/A2b/A3, 2026-07-19): a single noisy ledger row —
// a second metric on one group, or a text aggregate — must disarm only that
// key's slot, never the whole check. The previous global fail-open let one
// poison row resurrect both replay disease shapes ("17,4,5" and "17,5").
func TestEvaluateReferenceGroundingPoisonPerKey(t *testing.T) {
	base := func() []ContributionRecord {
		return []ContributionRecord{
			groundingContribution("GroupA", "10"),
			groundingContribution("GroupA", "7"),
			groundingContribution("GroupB", "4"),
			groundingContribution("GroupC", "5"),
		}
	}
	secondMetric := func() ContributionRecord {
		rec := groundingContribution("GroupA", "2")
		rec.ItemID = LooseText("item-poison")
		rec.Metric = LooseText("row_count")
		return rec
	}
	// A2a: usurped-slot shape stays red — GroupA goes ambiguous but slot 2
	// (GroupX, no records, must be 0) keeps its hard verdict.
	report, applicable := EvaluateReferenceGrounding(groundingCandidate(), append(base(), secondMetric()), "17,4,5")
	if !applicable || !report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want the usurped slot red despite the second-metric poison", report, applicable)
	}
	for _, mismatch := range report.Mismatches {
		if mismatch.Key == "GroupA" {
			t.Fatalf("mismatch=%+v, the ambiguous GroupA slot itself must stay unjudged (per-key fail-open)", mismatch)
		}
	}
	// A2b: the cardinality arm needs only keys+items and must survive any
	// ledger poison.
	report, applicable = EvaluateReferenceGrounding(groundingCandidate(), append(base(), secondMetric()), "17,5")
	if !applicable || !report.CardinalityMismatch {
		t.Fatalf("report=%+v applicable=%v, want the cardinality arm alive under the poison", report, applicable)
	}
	// A3: a text-operation row (include with prose value) on a junk group
	// must not silence the check either.
	textPoison := ContributionRecord{
		ItemID:        LooseText("item-note"),
		Source:        LooseText("observations.csv"),
		SourceLocator: LooseText("row"),
		GroupKey:      LooseText("methodology_note"),
		Metric:        LooseText("note"),
		Value:         LooseText("checked all rows"),
		Operation:     LooseText("include"),
		Role:          LooseText("target"),
	}
	report, applicable = EvaluateReferenceGrounding(groundingCandidate(), append(base(), textPoison), "17,4,5")
	if !applicable || !report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want the usurped slot red despite the text-include poison", report, applicable)
	}
	// Truth preservation: the same poison must not damn the grounded truth —
	// the ambiguous slot is exempt, every other slot passes on its merits.
	report, applicable = EvaluateReferenceGrounding(groundingCandidate(), append(base(), secondMetric()), "17,0,5")
	if !applicable || report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want the truth to survive the poison", report, applicable)
	}
}

// TestEvaluateReferenceGroundingLedgerDomainArm is the replay#3 run-1 shape
// at the engine level: a ledger whose numeric totals share no key with the
// reference set (degenerate literal group key) cannot discharge the per-key
// obligation — the domain arm must fire even before per-slot comparison.
func TestEvaluateReferenceGroundingLedgerDomainArm(t *testing.T) {
	degenerate := []ContributionRecord{
		groundingContribution("canonical_label", "10"),
		groundingContribution("canonical_label", "7"),
		groundingContribution("canonical_label", "4"),
		groundingContribution("canonical_label", "5"),
		groundingContribution("canonical_label", "11"),
	}
	report, applicable := EvaluateReferenceGrounding(groundingCandidate(), degenerate, "37")
	if !applicable {
		t.Fatal("degenerate-ledger grounding must be applicable (numeric answer, resolved reference)")
	}
	if !report.LedgerDomainMismatch || !report.CardinalityMismatch || !report.Violated() {
		t.Fatalf("report=%+v, want ledger-domain + cardinality violations", report)
	}
	if !strings.Contains(DescribeReferenceGroundingMismatches(report), "recomputed grouped by the reference key domain") {
		t.Fatalf("detail=%q, want the recompute repair direction", DescribeReferenceGroundingMismatches(report))
	}
	// The all-zero laundering counter-arm: with a domain-mismatched ledger,
	// a zero-filled "0,0,0" must NOT ground clean either.
	report, applicable = EvaluateReferenceGrounding(groundingCandidate(), degenerate, "0,0,0")
	if !applicable || !report.LedgerDomainMismatch || !report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want the domain arm to reject a zero-filled projection over a ledger that speaks no reference key", report, applicable)
	}
}

// TestValidateContributionDecisionConsistency pins the E-3 cross-ledger arm:
// a contribution summing a row the decision ledger excludes must fail loud
// with per-row detail; consistent ledgers and unjoinable rows stay silent.
func TestValidateContributionDecisionConsistency(t *testing.T) {
	rows := []RowDecision{
		{RowID: "obs#1", Decision: "include", SourceLocator: "line:1"},
		{RowID: "obs#3", Decision: "exclude", SourceLocator: "line:3"},
	}
	bad := []ContributionRecord{
		{ItemID: LooseText("obs#1"), GroupKey: LooseText("GroupA"), Metric: LooseText("total"), Value: LooseText("10"), Operation: LooseText("add"), Role: LooseText("target")},
		{ItemID: LooseText("obs#3"), GroupKey: LooseText("GroupA"), Metric: LooseText("total"), Value: LooseText("3"), Operation: LooseText("add"), Role: LooseText("target")},
	}
	err := ValidateContributionDecisionConsistency(rows, bad)
	if err == nil {
		t.Fatal("a contribution summing an excluded row must fail loud")
	}
	for _, want := range []string{"obs#3", "GroupA", "excludes", "line:3", "recompute contributions over included rows only"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err=%v, want per-row repair detail containing %q", err, want)
		}
	}
	good := bad[:1]
	if err := ValidateContributionDecisionConsistency(rows, good); err != nil {
		t.Fatalf("consistent ledgers must pass: %v", err)
	}
	// Audit-role contributions and unjoinable ids stay silent (fail-open).
	auditOnly := []ContributionRecord{{ItemID: LooseText("obs#3"), GroupKey: LooseText("GroupA"), Value: LooseText("3"), Operation: LooseText("add"), Role: LooseText("audit")}}
	if err := ValidateContributionDecisionConsistency(rows, auditOnly); err != nil {
		t.Fatalf("audit-role contributions do not participate in reconcile and must stay silent: %v", err)
	}
	unjoinable := []ContributionRecord{{ItemID: LooseText("other#9"), GroupKey: LooseText("GroupA"), Value: LooseText("3"), Operation: LooseText("add"), Role: LooseText("target")}}
	if err := ValidateContributionDecisionConsistency(rows, unjoinable); err != nil {
		t.Fatalf("unjoinable ids must stay silent (no fuzzy joins): %v", err)
	}

	// Wire-level arm: the workflow validator throat must carry the check —
	// a direct-call-only pin would let the wiring be silently deleted.
	wireErr := ValidateResultAgainstContract(CoverageContract{}, Result{
		Answer:        "13",
		Rows:          rows,
		Contributions: bad,
	}, LedgerSatisfactionFacts{})
	if wireErr == nil || !strings.Contains(wireErr.Error(), "decision ledger excludes") {
		t.Fatalf("ValidateResultAgainstContract must route the cross-ledger arm: %v", wireErr)
	}
}

// TestEvaluateReferenceGroundingFailOpenArms pins the inapplicability lanes:
// each ambiguous input must make the check inapplicable, never a guessed
// verdict in either direction.
func TestEvaluateReferenceGroundingFailOpenArms(t *testing.T) {
	contributions := []ContributionRecord{groundingContribution("GroupA", "17")}
	// Non-numeric non-key items on a resolved reference are a VIOLATION,
	// not an exemption (replay#5 run-1: "value=11; GroupA/value=17; …"
	// shipped through the old blanket non-numeric fail-open).
	report, applicable := EvaluateReferenceGrounding(groundingCandidate(), contributions, "alpha,beta,gamma")
	if !applicable || !report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want mush items to violate grounding", report, applicable)
	}
	// A slot-faithful textual key projection stays clean.
	report, applicable = EvaluateReferenceGrounding(groundingCandidate(), contributions, "GroupA,GroupX,GroupC")
	if !applicable || report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want the key projection to pass", report, applicable)
	}
	// An empty target ledger cannot ground any slot: absence-only totals
	// must not bless zero-filled answers (replay#5 run-2: audit-role-only
	// ledger blessed "0,0,0" and damned the correct answer).
	auditOnly := []ContributionRecord{func() ContributionRecord {
		rec := groundingContribution("GroupA", "1")
		rec.Role = LooseText("audit")
		rec.Operation = LooseText("count")
		return rec
	}()}
	if _, applicable := EvaluateReferenceGrounding(groundingCandidate(), auditOnly, "0,0,0"); applicable {
		t.Fatal("an audit-only ledger has no target totals and must be inapplicable")
	}
	if _, applicable := EvaluateReferenceGrounding(ReferenceKeyCandidate{}, contributions, "17,0,5"); applicable {
		t.Fatal("empty reference candidate must be inapplicable")
	}
	if _, applicable := EvaluateReferenceGrounding(groundingCandidate(), contributions, ""); applicable {
		t.Fatal("empty answer must be inapplicable")
	}
	// A group key split across metrics is ambiguous PER-KEY (merged review
	// F2): only that key's slot goes unjudged; every other slot keeps its
	// verdict. Full-check inapplicability for ambiguity was the escape that
	// let one poisoned row silence the whole gate.
	multiMetric := []ContributionRecord{
		groundingContribution("GroupA", "10"),
		func() ContributionRecord {
			rec := groundingContribution("GroupA", "7")
			rec.Metric = LooseText("other_metric")
			return rec
		}(),
	}
	// With NO unambiguous numeric total left anywhere, the case is not a
	// totals projection at all (selection-shaped and fully-ambiguous
	// ledgers): inapplicable, matching the audit-only boundary.
	if _, applicable := EvaluateReferenceGrounding(groundingCandidate(), multiMetric, "17,0,5"); applicable {
		t.Fatal("a ledger with zero unambiguous numeric totals must be inapplicable")
	}
	// With at least one unambiguous numeric total, ambiguity stays per-key:
	// GroupA's slot is unjudged, every other slot keeps its hard verdict.
	withAnchor := append([]ContributionRecord{groundingContribution("GroupC", "5")}, multiMetric...)
	report, applicable = EvaluateReferenceGrounding(groundingCandidate(), withAnchor, "999,0,5")
	if !applicable || report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want the ambiguous GroupA slot exempt and the clean remaining slots to pass", report, applicable)
	}
	report, applicable = EvaluateReferenceGrounding(groundingCandidate(), withAnchor, "17,0,4")
	if !applicable || !report.Violated() {
		t.Fatalf("report=%+v applicable=%v, want the unambiguous GroupC slot still judged", report, applicable)
	}
	for _, mismatch := range report.Mismatches {
		if mismatch.Key == "GroupA" {
			t.Fatalf("mismatch=%+v, the ambiguous key's own slot must stay unjudged", mismatch)
		}
	}
}
