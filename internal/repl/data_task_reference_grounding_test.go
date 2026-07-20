package repl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// Witness-isomorphic pins for the reference-set output grounding gate
// (DATAGATE-1 follow-up; witness eval/results/
// data_multifile_reference_projection-20260719-074923): after the G10
// deadlock fix, both replay runs completed but published internally
// consistent WRONG answers — run-1 "17,5" (2 items for 3 targets), run-2
// "17,4,5" (non-target GroupB's total in the slot of GroupX, which has zero
// mapped records and must output 0). Truth is "17,0,5". Root cause: no gate
// ever grounded answer↔targets; reconcile only proves contributions↔answer.
// These tests reproduce the fixture materials byte-faithfully and pin both
// disease shapes red, the truth green, and the fail-open lanes silent.

func referenceGroundingFixtureRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"targets.csv": "target_id,canonical_label\nT1,GroupA\nT2,GroupX\nT3,GroupC\n",
		"labels.csv":  "raw_label,canonical_label\nA-one,GroupA\nA-two,GroupA\nBeta,GroupB\nGamma alt,GroupC\n",
		"observations.csv": "record_id,raw_label,value,active\nr1,A-one,10,true\nr2,A-two,7,true\nr3,A-one,3,false\n" +
			"r4,Beta,4,true\nr5,Gamma alt,5,true\nr6,unmapped,11,true\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func referenceGroundingContract() dataquery.CoverageContract {
	return dataquery.CoverageContract{
		RequiredMaterials: []dataquery.CoverageMaterial{
			{Path: "observations.csv", Purpose: "source rows", Required: true},
			{Path: "labels.csv", Purpose: "label mapping", Required: true},
			{Path: "targets.csv", Purpose: "output reference set", Required: true},
		},
	}
}

func referenceGroundingRecords() []dataTaskWorkflowRecord {
	return []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: referenceGroundingContract()},
	}}
}

func referenceGroundingResult(answer string, groups map[string]string) dataquery.Result {
	res := dataquery.Result{
		Answer:         answer,
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		ConsumedPaths:  []string{"observations.csv", "labels.csv", "targets.csv"},
	}
	for _, group := range []string{"GroupA", "GroupB", "GroupC"} {
		value, ok := groups[group]
		if !ok {
			continue
		}
		res.Contributions = append(res.Contributions, dataquery.ContributionRecord{
			ItemID:        dataquery.LooseText("item-" + group),
			Source:        dataquery.LooseText("observations.csv"),
			SourceLocator: dataquery.LooseText("row"),
			GroupKey:      dataquery.LooseText(group),
			Metric:        dataquery.LooseText("total_value"),
			Value:         dataquery.LooseText(value),
			Operation:     dataquery.LooseText("add"),
			Role:          dataquery.LooseText("target"),
		})
	}
	return res
}

// TestReferenceGroundingGuardRedOnCardinalityShape is replay run-1: "17,5"
// with contributions {GroupA:17, GroupC:5} — two items can never project a
// three-key reference set.
func TestReferenceGroundingGuardRedOnCardinalityShape(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}
	result := referenceGroundingResult("17,5", map[string]string{"GroupA": "17", "GroupC": "5"})

	guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, result)
	if guard.Empty() {
		t.Fatal("grounding guard must fire on the run-1 cardinality shape (2 items, 3 reference keys)")
	}
	text := guard.ErrorText()
	if !strings.Contains(text, "output_reference_grounding_mismatch") && !strings.Contains(text, "targets.csv") {
		t.Fatalf("guard=%q, want the reference set named", text)
	}
	if !strings.Contains(text, "2 item(s)") || !strings.Contains(text, "3 key(s)") {
		t.Fatalf("guard=%q, want the cardinality mismatch detail", text)
	}
	if gate := dataTaskWorkflowCompletionGateGuardResultWithRepo(root, records, current, result); gate.Empty() || !strings.Contains(gate.ErrorText(), "grounding failed") {
		t.Fatalf("completion gate=%q, want the grounding violation surfaced", gate.ErrorText())
	}
	if dataTaskResultStructurallyCompleteWithRepo(root, records, current, result) {
		t.Fatal("an ungrounded answer must not be structurally complete")
	}
}

// TestReferenceGroundingGuardRedOnUsurpedSlotShape is replay run-2: "17,4,5"
// — cardinality matches but slot 2 (GroupX, zero mapped records, must be 0)
// carries non-reference GroupB's total.
func TestReferenceGroundingGuardRedOnUsurpedSlotShape(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}
	result := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})

	guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, result)
	if guard.Empty() {
		t.Fatal("grounding guard must fire on the run-2 usurped-slot shape")
	}
	text := guard.ErrorText()
	for _, want := range []string{"GroupX", "expected 0", "4", "no contribution records"} {
		if !strings.Contains(text, want) {
			t.Fatalf("guard=%q, want per-slot mismatch detail containing %q", text, want)
		}
	}
	if !strings.Contains(text, "assemble_answer") || !strings.Contains(text, "complete_reference=true") {
		t.Fatalf("guard=%q, want an emit-stage-admissible repair path (assemble_answer with complete_reference)", text)
	}
	if dataTaskResultStructurallyCompleteWithRepo(root, records, current, result) {
		t.Fatal("an ungrounded answer must not be structurally complete")
	}
}

// TestReferenceGroundingGuardAcceptsGroundedTruth: the correct "17,0,5"
// passes both arms and the guard stays quiet — the gate releases the truth,
// it never edits values.
func TestReferenceGroundingGuardAcceptsGroundedTruth(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}
	result := referenceGroundingResult("17,0,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})

	if guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, result); !guard.Empty() {
		t.Fatalf("grounded truth must pass: %s", guard.ErrorText())
	}
}

// TestReferenceGroundingGuardFailOpenLanes pins zero behavior change for
// workflows genuinely without a resolvable reference set: no key-table
// material anywhere, ambiguous competing key tables, and non-numeric answers
// all keep the guard silent (precise signals only — no hard gate on a
// guessed reference universe).
func TestReferenceGroundingGuardFailOpenLanes(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}

	// True negative: a repo with NO key-table material at all (data-basic
	// shape). Activation is census-driven, so the absence must come from
	// the bytes, not from the model's contract filing.
	basicRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(basicRoot, "orders.csv"), []byte("id,amount\na,10\nb,7\nc,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	basicContract := dataquery.CoverageContract{RequiredMaterials: []dataquery.CoverageMaterial{
		{Path: "orders.csv", Purpose: "source rows", Required: true},
	}}
	basicRecords := []dataTaskWorkflowRecord{{Plan: dataquery.TaskPlan{CoverageContract: basicContract}}}
	basicResult := referenceGroundingResult("17", map[string]string{"GroupA": "17"})
	basicResult.ConsumedPaths = []string{"orders.csv"}
	if guard := dataTaskOutputReferenceGroundingGuardResult(basicRoot, basicRecords, dataquery.TaskPlan{CoverageContract: basicContract}, basicResult); !guard.Empty() {
		t.Fatalf("no key-table material anywhere must mean zero behavior change: %s", guard.ErrorText())
	}

	// Ambiguity: a second, disagreeing key table must fail open.
	if err := os.WriteFile(filepath.Join(root, "targets_alt.csv"), []byte("alt_id,canonical_label\nZ1,GroupC\nZ2,GroupA\n"), 0600); err != nil {
		t.Fatal(err)
	}
	wrong := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	ambiguous := referenceGroundingContract()
	ambiguous.RequiredMaterials = append(ambiguous.RequiredMaterials, dataquery.CoverageMaterial{Path: "targets_alt.csv", Purpose: "second reference", Required: true})
	ambiguousRecords := []dataTaskWorkflowRecord{{Plan: dataquery.TaskPlan{CoverageContract: ambiguous}}}
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, ambiguousRecords, dataquery.TaskPlan{CoverageContract: ambiguous}, wrong); !guard.Empty() {
		t.Fatalf("competing key tables are ambiguous and must fail open: %s", guard.ErrorText())
	}

	// A slot-faithful textual key projection stays clean (grounding judges
	// values and slots, not the choice to print keys).
	textual := referenceGroundingResult("GroupA,GroupX,GroupC", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, referenceGroundingRecords(), current, textual); !guard.Empty() {
		t.Fatalf("a slot-faithful key projection must stay clean: %s", guard.ErrorText())
	}
}

func referenceGroundingSecondMetricPoison() dataquery.ContributionRecord {
	return dataquery.ContributionRecord{
		ItemID:        dataquery.LooseText("item-poison"),
		Source:        dataquery.LooseText("observations.csv"),
		SourceLocator: dataquery.LooseText("row"),
		GroupKey:      dataquery.LooseText("GroupA"),
		Metric:        dataquery.LooseText("row_count"),
		Value:         dataquery.LooseText("2"),
		Operation:     dataquery.LooseText("add"),
		Role:          dataquery.LooseText("target"),
	}
}

// TestReferenceGroundingRedOnMixedKeyEcho is the merged-review F1 adversarial
// shape (A1, 2026-07-19): "17,GroupX,5" — the slot that must be 0 echoes its
// own key while the other slots carry totals. Under the per-slot echo
// carve-out this passed the guard AND the full completion gate and would have
// published complete. The echo exemption must be uniform.
func TestReferenceGroundingRedOnMixedKeyEcho(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}
	result := referenceGroundingResult("17,GroupX,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})

	guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, result)
	if guard.Empty() {
		t.Fatal("mixed key-echo answer 17,GroupX,5 must not pass grounding (truth is 17,0,5)")
	}
	if !strings.Contains(guard.ErrorText(), "GroupX") {
		t.Fatalf("guard=%q, want the echoed GroupX slot named", guard.ErrorText())
	}
	if gate := dataTaskWorkflowCompletionGateGuardResultWithRepo(root, records, current, result); gate.Empty() {
		t.Fatal("mixed key-echo answer must not pass the full completion gate")
	}
	if dataTaskResultStructurallyCompleteWithRepo(root, records, current, result) {
		t.Fatal("mixed key-echo answer must not be structurally complete")
	}
}

// TestReferenceGroundingPoisonedLedgerKeepsDiseaseShapesRed is the
// merged-review F2 adversarial family (A2a/A2b/A3, 2026-07-19): one noisy
// target-role row — a second metric on an existing group, or a text include
// note — used to nil the totals map and silence the whole grounding check,
// resurrecting both replay disease shapes through guard AND full gate.
func TestReferenceGroundingPoisonedLedgerKeepsDiseaseShapesRed(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}

	// A2a: usurped-slot "17,4,5" + second-metric poison.
	usurped := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	usurped.Contributions = append(usurped.Contributions, referenceGroundingSecondMetricPoison())
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, usurped); guard.Empty() {
		t.Fatal("one same-group second-metric row must not silence grounding (17,4,5 sailed)")
	}
	if gate := dataTaskWorkflowCompletionGateGuardResultWithRepo(root, records, current, usurped); gate.Empty() {
		t.Fatal("poisoned-ledger 17,4,5 must not pass the full completion gate")
	}

	// A2b: cardinality "17,5" + the same poison — arm A needs only
	// keys+items and must survive.
	cardinality := referenceGroundingResult("17,5", map[string]string{"GroupA": "17", "GroupC": "5"})
	cardinality.Contributions = append(cardinality.Contributions, referenceGroundingSecondMetricPoison())
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, cardinality); guard.Empty() {
		t.Fatal("the poison must not kill the cardinality arm (17,5 sailed)")
	}

	// A3: text include note on a junk group.
	textPoisoned := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	textPoisoned.Contributions = append(textPoisoned.Contributions, dataquery.ContributionRecord{
		ItemID:        dataquery.LooseText("item-note"),
		Source:        dataquery.LooseText("observations.csv"),
		SourceLocator: dataquery.LooseText("row"),
		GroupKey:      dataquery.LooseText("methodology_note"),
		Metric:        dataquery.LooseText("note"),
		Value:         dataquery.LooseText("checked all rows"),
		Operation:     dataquery.LooseText("include"),
		Role:          dataquery.LooseText("target"),
	})
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, textPoisoned); guard.Empty() {
		t.Fatal("one text include row must not silence grounding (17,4,5 sailed)")
	}

	// Truth preservation under the same poison: the grounded truth still
	// passes the guard (per-key exemption only, never a new hard reject).
	truth := referenceGroundingResult("17,0,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	truth.Contributions = append(truth.Contributions, referenceGroundingSecondMetricPoison())
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, truth); !guard.Empty() {
		t.Fatalf("the poison must not damn the grounded truth: %s", guard.ErrorText())
	}
}

// TestReferenceGroundingRedOnLedgerProseMush is the replay#5 run-1 witness
// shape: the published answer was a semicolon prose dump of the reconcile
// groups ("value=11; GroupA/value=17; …") that even carried the unmapped
// row's total — a single-line string, so the plain_single_line contract
// passed it, and the old blanket non-numeric fail-open exempted it from
// grounding. On a reference-bound case an unparseable answer is a violation.
func TestReferenceGroundingRedOnLedgerProseMush(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}
	mush := referenceGroundingResult("value=11; GroupA/value=17; GroupB/value=4; GroupC/value=5",
		map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, mush)
	if guard.Empty() {
		t.Fatal("ledger-prose mush must not pass reference grounding (replay#5 run-1 shipped it)")
	}
	if !strings.Contains(guard.ErrorText(), "1 item(s)") || !strings.Contains(guard.ErrorText(), "3 key(s)") {
		t.Fatalf("guard=%q, want the cardinality arm named", guard.ErrorText())
	}
	if dataTaskResultStructurallyCompleteWithRepo(root, records, current, mush) {
		t.Fatal("mush must not be structurally complete")
	}
}

// TestAuditOnlyContributionLedgerFailsLoud is the replay#5 run-2 witness
// shape: every contribution is an audit-role count, the target ledger is
// empty, and the zero-expectation inversion blessed "0,0,0" while rejecting
// the correct answer. The required-ledger validator must red the audit-only
// ledger itself, and grounding must stay inapplicable rather than inverted.
func TestAuditOnlyContributionLedgerFailsLoud(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: func() dataquery.CoverageContract {
		c := referenceGroundingContract()
		c.ContributionLedgerRequired = true
		return c
	}()}
	res := referenceGroundingResult("0,0,0", nil)
	for i := 1; i <= 5; i++ {
		res.Contributions = append(res.Contributions, dataquery.ContributionRecord{
			ItemID:        dataquery.LooseText(fmt.Sprintf("active_observations#%d", i)),
			Source:        dataquery.LooseText("observations.csv"),
			SourceLocator: dataquery.LooseText("row"),
			GroupKey:      dataquery.LooseText("workflow_audit"),
			Metric:        dataquery.LooseText("workflow_audit"),
			Value:         dataquery.LooseText("1"),
			Operation:     dataquery.LooseText("count"),
			Role:          dataquery.LooseText("audit"),
		})
	}
	err := validateDataTaskWorkflowResult("", records, current, res)
	if err == nil {
		t.Fatal("an audit-only contribution ledger must fail the required-ledger validator")
	}
	if !strings.Contains(err.Error(), "only audit/diagnostic-role records") {
		t.Fatalf("err=%v, want the role-starved ledger named", err)
	}
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, res); !guard.Empty() {
		t.Fatalf("grounding must stay inapplicable over an empty target ledger (the validator owns this violation): %s", guard.ErrorText())
	}
	if dataTaskResultStructurallyCompleteWithRepo(root, records, current, res) {
		t.Fatal("the audit-only ledger shape must not be structurally complete")
	}
	// F5 (merged review): the inversion lock must hold independently on THIS
	// surface — over the same audit-only ledger the CORRECT answer must stay
	// unjudged (guard empty), never damned by zero expectations. Under the
	// zero-expectation inversion this guard fires on the truth, so this arm
	// kills that mutation without leaning on the dataquery unit pin.
	correct := referenceGroundingResult("17,0,5", nil)
	correct.Contributions = append([]dataquery.ContributionRecord{}, res.Contributions...)
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, correct); !guard.Empty() {
		t.Fatalf("zero-expectation inversion: the correct answer must not be damned over an audit-only ledger: %s", guard.ErrorText())
	}
}

// TestReferenceGroundingActivationIsCensusDriven pins 件D (replay#3
// 2026-07-19, both runs bypassed the previous activation): the reference
// obligation activates from the typed material census and no model-shaped
// state can exempt it.
func TestReferenceGroundingActivationIsCensusDriven(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)

	// Run-2 shape: targets.csv filed only under optional_materials (the
	// required-only scan missed it and "17,4,5" shipped).
	optionalOnly := dataquery.CoverageContract{
		RequiredMaterials: []dataquery.CoverageMaterial{
			{Path: "observations.csv", Purpose: "source rows", Required: true},
			{Path: "labels.csv", Purpose: "label mapping", Required: true},
		},
		OptionalMaterials: []dataquery.CoverageMaterial{
			{Path: "targets.csv", Purpose: "output reference set"},
		},
	}
	optRecords := []dataTaskWorkflowRecord{{Plan: dataquery.TaskPlan{CoverageContract: optionalOnly}}}
	wrong := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	guard := dataTaskOutputReferenceGroundingGuardResult(root, optRecords, dataquery.TaskPlan{CoverageContract: optionalOnly}, wrong)
	if guard.Empty() {
		t.Fatal("filing targets.csv under optional_materials must not exempt the reference obligation (件D)")
	}
	if !strings.Contains(guard.ErrorText(), "GroupX") {
		t.Fatalf("guard=%q, want the GroupX slot named", guard.ErrorText())
	}

	// Census reach via consumed paths only: not in the contract at all.
	// The record additionally registers artifacts whose IDs literally equal
	// the source file names — the replay#4 run-2 poisoning shape, where the
	// generated-artifact alias set marked targets.csv itself as generated
	// and blinded the census. The disk-first credential must survive it.
	poisoned := dataquery.Result{
		Artifacts: []dataquery.DataArtifact{
			{ID: "targets.csv", Kind: "extract_records/reference", SourcePaths: []string{"targets.csv"}},
			{ID: "labels.csv", Kind: "extract_records/csv", SourcePaths: []string{"labels.csv"}},
		},
	}
	consumedOnly := dataquery.CoverageContract{RequiredMaterials: []dataquery.CoverageMaterial{
		{Path: "observations.csv", Purpose: "source rows", Required: true},
	}}
	consumedRecords := []dataTaskWorkflowRecord{{Plan: dataquery.TaskPlan{CoverageContract: consumedOnly}, Result: &poisoned}}
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, consumedRecords, dataquery.TaskPlan{CoverageContract: consumedOnly}, wrong); guard.Empty() {
		t.Fatal("a consumed reference material outside the contract census must still arm the obligation, even when source-named artifacts poison the generated-alias set (replay#4 run-2)")
	}

	// Format declaration cannot exempt (件D): freeform + explanation-allowed
	// declared contract, same wrong numeric list answer.
	relaxed := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	relaxed.OutputContract = dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true}
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, referenceGroundingRecords(), dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}, relaxed); guard.Empty() {
		t.Fatal("a model-declared relaxed output contract must not exempt the reference obligation (件D)")
	}
}

// TestReferenceGroundingRedOnDegenerateLedgerGrandTotal is the replay#3
// run-1 witness shape: every contribution carries the literal field name
// "canonical_label" as its group key, the ledger sums everything (including
// unmapped and non-target rows) to 37, reconcile self-passes, and "37"
// shipped. The census anchor (targets.csv#canonical_label cross-linked into
// labels.csv) must arm the obligation and red both the cardinality and the
// ledger-domain arms.
func TestReferenceGroundingRedOnDegenerateLedgerGrandTotal(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	res := dataquery.Result{
		Answer:         "37",
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		ConsumedPaths:  []string{"observations.csv", "labels.csv", "targets.csv"},
	}
	for _, value := range []string{"10", "7", "4", "5", "11"} {
		res.Contributions = append(res.Contributions, dataquery.ContributionRecord{
			ItemID:        dataquery.LooseText("row-" + value),
			Source:        dataquery.LooseText("observations.csv"),
			SourceLocator: dataquery.LooseText("row"),
			GroupKey:      dataquery.LooseText("canonical_label"),
			Metric:        dataquery.LooseText("total_value"),
			Value:         dataquery.LooseText(value),
			Operation:     dataquery.LooseText("add"),
			Role:          dataquery.LooseText("target"),
		})
	}
	res.Reconcile = &dataquery.ReconcileReport{
		Status:         dataquery.LooseText("pass"),
		ExpectedAnswer: dataquery.LooseText("37"),
		ActualAnswer:   dataquery.LooseText("37"),
		Groups: []dataquery.ReconcileGroup{{
			GroupKey: dataquery.LooseText("canonical_label"),
			Metric:   dataquery.LooseText("total_value"),
			Expected: dataquery.LooseText("37"),
			Actual:   dataquery.LooseText("37"),
		}},
	}
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}
	guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, res)
	if guard.Empty() {
		t.Fatal("a degenerate grand-total ledger must not exempt the reference obligation (replay#3 run-1: 37 shipped)")
	}
	text := guard.ErrorText()
	if !strings.Contains(text, "1 item(s)") || !strings.Contains(text, "3 key(s)") {
		t.Fatalf("guard=%q, want the 1-vs-3 cardinality mismatch", text)
	}
	if !strings.Contains(text, "recomputed grouped by the reference key domain") {
		t.Fatalf("guard=%q, want the ledger-domain repair direction disclosed", text)
	}
	if dataTaskResultStructurallyCompleteWithRepo(root, records, current, res) {
		t.Fatal("the degenerate grand total must not be structurally complete")
	}
}

// --- Answer-authority stickiness pins (件C; witness replay#2 run-2
// 2026-07-19: system-synthesized re-projections under a laundered
// reference_path=contributions.json declaration overwrote the validated
// grounded "17,0,5" with "17,5" then "0", and "0" shipped). The ledger in
// these fixtures is T-keyed exactly like the witness (model grouped by
// target_id): targets.csv#target_id is the unique structural reference
// (canonical_label has zero overlap with T-keys).

func stickinessContract() dataquery.CoverageContract {
	return dataquery.CoverageContract{
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
		RequiredMaterials: []dataquery.CoverageMaterial{
			{Path: "observations.csv", Purpose: "source rows", Required: true},
			{Path: "labels.csv", Purpose: "label mapping", Required: true},
			{Path: "targets.csv", Purpose: "output reference set", Required: true},
		},
	}
}

func stickinessResult(answer string) dataquery.Result {
	res := dataquery.Result{
		Answer:         answer,
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		ConsumedPaths:  []string{"observations.csv", "labels.csv", "targets.csv"},
	}
	for _, row := range [][2]string{{"T1", "10"}, {"T1", "7"}, {"T3", "5"}} {
		res.Contributions = append(res.Contributions, dataquery.ContributionRecord{
			ItemID:        dataquery.LooseText("item-" + row[0]),
			Source:        dataquery.LooseText("observations.csv"),
			SourceLocator: dataquery.LooseText("row"),
			GroupKey:      dataquery.LooseText(row[0]),
			Metric:        dataquery.LooseText("sum"),
			Value:         dataquery.LooseText(row[1]),
			Operation:     dataquery.LooseText("add"),
			Role:          dataquery.LooseText("target"),
		})
	}
	res.Reconcile = &dataquery.ReconcileReport{
		Status:         dataquery.LooseText("pass"),
		ExpectedAnswer: dataquery.LooseText(answer),
		ActualAnswer:   dataquery.LooseText(answer),
		Groups: []dataquery.ReconcileGroup{
			{GroupKey: dataquery.LooseText("T1"), Metric: dataquery.LooseText("sum"), Expected: dataquery.LooseText("17"), Actual: dataquery.LooseText("17"), Difference: dataquery.LooseText("0")},
			{GroupKey: dataquery.LooseText("T3"), Metric: dataquery.LooseText("sum"), Expected: dataquery.LooseText("5"), Actual: dataquery.LooseText("5"), Difference: dataquery.LooseText("0")},
		},
	}
	// Engine-real shape: the contribution ledger materializes as a generated
	// artifact whose alias is contributions.json — exactly the alias the
	// witness laundering declared as reference_path.
	res.Artifacts = []dataquery.DataArtifact{{
		ID:          "contributions",
		Kind:        "compute_contributions",
		SourcePaths: []string{"observations.csv"},
	}}
	return res
}

func launderedResult(answer string, keyField string) dataquery.Result {
	res := stickinessResult(answer)
	// The witness overwrite declared the workflow's own generated artifact
	// as the reference universe (r12: group_key → "17,5"; r13: metric → "0").
	res.OutputContract = dataquery.OutputContract{
		Format:             dataquery.OutputPlainSingleLine,
		ExplanationAllowed: false,
		CompleteReference:  true,
		ReferencePath:      "contributions.json",
		ReferenceKeyField:  keyField,
	}
	return res
}

// TestAnswerStickinessRetainsValidatedAnswerAgainstDegradedCandidate is pin
// ① of 件C: a validated grounded answer is in place; a later laundered
// candidate ("0") fails grounding and must NOT take the slot — the original
// answer is retained byte-identical and the retention is disclosed.
func TestAnswerStickinessRetainsValidatedAnswerAgainstDegradedCandidate(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	validated := stickinessResult("17,0,5")
	records := []dataTaskWorkflowRecord{
		{Plan: dataquery.TaskPlan{CoverageContract: stickinessContract()}},
		{Plan: dataquery.TaskPlan{CoverageContract: stickinessContract()}, Result: &validated},
	}
	degraded := launderedResult("0", "metric")
	current := dataquery.TaskPlan{CoverageContract: stickinessContract()}

	// The laundered declaration must not ground the degraded candidate:
	// lane 1 refuses the generated-artifact path and the structural lane
	// grounds it against targets.csv#target_id instead (C1).
	if guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, degraded); guard.Empty() {
		t.Fatal("laundered contributions.json reference declaration must not ground the degraded candidate")
	}
	sel := selectDataTaskTerminalAnswerWithRepo(root, records, current, degraded, stickinessContract(), validated.OutputContract)
	if !sel.FromFallback || !sel.ValidatedRetained {
		t.Fatalf("selection=%+v, want the validated answer retained via the stickiness gate", sel)
	}
	if sel.Result.Answer != "17,0,5" {
		t.Fatalf("retained answer=%q, want the validated 17,0,5 byte-identical", sel.Result.Answer)
	}
	if strings.TrimSpace(sel.RetainReason) == "" {
		t.Fatal("retention must be disclosed with a typed reason")
	}
}

// TestAnswerStickinessAllowsEquallyValidatedReplacement is pin ②: a later
// candidate that passes the full validation itself takes the slot normally —
// stickiness never freezes progress for validated candidates.
func TestAnswerStickinessAllowsEquallyValidatedReplacement(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	validated := stickinessResult("17,0,5")
	records := []dataTaskWorkflowRecord{
		{Plan: dataquery.TaskPlan{CoverageContract: stickinessContract()}, Result: &validated},
	}
	replacement := stickinessResult("17,0,5")
	current := dataquery.TaskPlan{CoverageContract: stickinessContract()}
	sel := selectDataTaskTerminalAnswerWithRepo(root, records, current, replacement, stickinessContract(), replacement.OutputContract)
	if sel.FromFallback || sel.ValidatedRetained {
		t.Fatalf("selection=%+v, want the later equally-validated candidate to take the slot directly", sel)
	}
	if sel.Result.Answer != "17,0,5" {
		t.Fatalf("answer=%q, want 17,0,5", sel.Result.Answer)
	}
}

// TestAnswerStickinessInertWithoutValidatedAnswer is pin ③: when no
// validated answer is in place, selection keeps its pre-existing semantics —
// the direct candidate is selected even though it fails grounding (the
// completion gate downstream stays the honest blocker).
func TestAnswerStickinessInertWithoutValidatedAnswer(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	ungrounded := stickinessResult("17,5")
	records := []dataTaskWorkflowRecord{
		{Plan: dataquery.TaskPlan{CoverageContract: stickinessContract()}, Result: &ungrounded},
	}
	direct := launderedResult("0", "metric")
	current := dataquery.TaskPlan{CoverageContract: stickinessContract()}
	sel := selectDataTaskTerminalAnswerWithRepo(root, records, current, direct, stickinessContract(), direct.OutputContract)
	if sel.FromFallback || sel.ValidatedRetained {
		t.Fatalf("selection=%+v, want pre-existing direct-candidate semantics when nothing validated is in place", sel)
	}
	if sel.Result.Answer != "0" {
		t.Fatalf("answer=%q, want the direct candidate (honest gate blocks it downstream)", sel.Result.Answer)
	}
}

// TestAnswerStickinessPoisonedCandidateCannotUsurpValidated is the
// merged-review F3 adversarial shape (A4, 2026-07-19): a validated grounded
// "17,0,5" is in place; the current candidate is a poisoned-ledger "17,4,5".
// Under the old guard-empty direct short-circuit plus the global totals
// fail-open, the poisoned candidate's grounding went silent and it took the
// slot. The direct lane holds the same bar as the incumbent scan.
func TestAnswerStickinessPoisonedCandidateCannotUsurpValidated(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	validated := referenceGroundingResult("17,0,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	records := []dataTaskWorkflowRecord{
		{Plan: dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}},
		{Plan: dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}, Result: &validated},
	}
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}
	poisoned := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	poisoned.Contributions = append(poisoned.Contributions, referenceGroundingSecondMetricPoison())

	sel := selectDataTaskTerminalAnswerWithRepo(root, records, current, poisoned, referenceGroundingContract(), poisoned.OutputContract)
	if sel.Result.Answer != "17,0,5" {
		t.Fatalf("ESCAPE: poisoned candidate %q usurps the validated 17,0,5", sel.Result.Answer)
	}
	if !sel.FromFallback || !sel.ValidatedRetained {
		t.Fatalf("selection=%+v, want the retention disclosed via the stickiness gate", sel)
	}
}

// TestAnswerStickinessInapplicableCandidateCannotUsurpValidated pins the F3
// direct-lane contract itself: the documented bar is "a later candidate may
// take the slot only by passing the same validation", and fail-open is not
// validation (E-1 addendum). A candidate whose grounding is genuinely
// INAPPLICABLE (here: no contributions at all, so the reference resolver
// stays blind) must not displace a positively grounded incumbent — under the
// old guard-empty short-circuit it did, byte-for-byte.
func TestAnswerStickinessInapplicableCandidateCannotUsurpValidated(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	validated := referenceGroundingResult("17,0,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	records := []dataTaskWorkflowRecord{
		{Plan: dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}},
		{Plan: dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}, Result: &validated},
	}
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}
	unjudgeable := referenceGroundingResult("9,9,9", nil)

	if guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, unjudgeable); !guard.Empty() {
		t.Fatalf("fixture invariant: the no-contribution candidate must be guard-silent (inapplicable), got %s", guard.ErrorText())
	}
	sel := selectDataTaskTerminalAnswerWithRepo(root, records, current, unjudgeable, referenceGroundingContract(), unjudgeable.OutputContract)
	if sel.Result.Answer != "17,0,5" {
		t.Fatalf("ESCAPE: inapplicable-grounding candidate %q usurps the validated 17,0,5 (fail-open treated as validation on the direct lane)", sel.Result.Answer)
	}
	if !sel.FromFallback || !sel.ValidatedRetained {
		t.Fatalf("selection=%+v, want the retention disclosed", sel)
	}
	if !strings.Contains(sel.RetainReason, "could not be positively validated") {
		t.Fatalf("reason=%q, want the fail-open-is-not-validation disclosure", sel.RetainReason)
	}
}

// TestFinalDataTaskAnswerForCLIThroatRunsCompletionGate is the merged-review
// F4 pin: every CLI answer exit drains through finalDataTaskAnswerForCLI, and
// the completion-gate call inside it is the publication throat — deleting
// that call must turn this pin red (the review's V1 mutation survived the
// whole repl package because every grounding pin drove the gate functions
// directly). A gate-red result must come back as an error, never markdown.
func TestFinalDataTaskAnswerForCLIThroatRunsCompletionGate(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}

	bad := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	answer, err := finalDataTaskAnswerForCLI(root, records, current, bad, "en")
	if err == nil {
		t.Fatalf("gate-red result must not publish through the CLI throat; got %q", answer)
	}
	if !strings.Contains(err.Error(), "grounding failed") {
		t.Fatalf("err=%v, want the grounding violation carried out of the throat", err)
	}

	good := referenceGroundingResult("17,0,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	markdown, err := finalDataTaskAnswerForCLI(root, records, current, good, "en")
	if err != nil {
		t.Fatalf("the grounded truth must publish: %v", err)
	}
	if !strings.Contains(markdown, "17,0,5") {
		t.Fatalf("markdown=%q, want the verbatim 17,0,5 answer", markdown)
	}
}

// TestReferencePathMaterialCredential pins the C1/C3 hygiene helper: only
// task-declared materials (directly or via #records alias) qualify as a
// reference declaration target; generated artifacts never do.
func TestReferencePathMaterialCredential(t *testing.T) {
	generated := stickinessResult("17,0,5")
	generated.Artifacts = append(generated.Artifacts, dataquery.DataArtifact{
		ID:   "final_answer.json",
		Kind: "assemble_answer",
	})
	records := []dataTaskWorkflowRecord{{
		Plan:   dataquery.TaskPlan{CoverageContract: stickinessContract()},
		Result: &generated,
	}}
	current := dataquery.TaskPlan{CoverageContract: stickinessContract()}
	for path, want := range map[string]bool{
		"targets.csv":                true,
		"targets.csv#records":        true,
		"labels.csv":                 true,
		"contributions.json":         false,
		"contributions.json#records": false,
		"final_answer.json":          false,
		"":                           false,
	} {
		if got := dataTaskReferencePathIsWorkflowMaterial(t.TempDir(), records, current, dataquery.Result{}, path); got != want {
			t.Fatalf("dataTaskReferencePathIsWorkflowMaterial(%q)=%v, want %v", path, got, want)
		}
	}
	// The gate resolver must resolve the laundered declaration to the
	// structural material reference, not the declared artifact.
	root := referenceGroundingFixtureRepo(t)
	laundered := launderedResult("0", "metric")
	candidate, ok := dataTaskResolveOutputReferenceSet(root, records, current, laundered, laundered.OutputContract.Normalize())
	if !ok || candidate.Path != "targets.csv" || candidate.Field != "target_id" {
		t.Fatalf("candidate=%+v ok=%v, want the structural targets.csv#target_id reference despite the laundered declaration", candidate, ok)
	}
}

// TestReferenceGroundingDeclaredLane: lane 1 — an output contract that
// declares complete_reference + reference_path resolves the reference set by
// declaration, without needing the structural credential.
func TestReferenceGroundingDeclaredLane(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records := referenceGroundingRecords()
	current := dataquery.TaskPlan{CoverageContract: referenceGroundingContract()}
	result := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	result.OutputContract = dataquery.OutputContract{
		Format:             dataquery.OutputPlainSingleLine,
		ExplanationAllowed: false,
		CompleteReference:  true,
		ReferencePath:      "targets.csv",
		ReferenceKeyField:  "canonical_label",
	}
	guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, result)
	if guard.Empty() {
		t.Fatal("declared reference lane must ground the answer and fire on the usurped slot")
	}
	if !strings.Contains(guard.ErrorText(), "GroupX") {
		t.Fatalf("guard=%q, want the usurped GroupX slot named", guard.ErrorText())
	}
}
