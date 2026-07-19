package dataworkflow

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// This file pins the single-authority contract of the entity-resolution
// obligation (DATAGATE-1, audit GAP-3/G10, campaign ledger §29.140/§29.142):
//
//  1. Two-authority same judgment: the dataquery result validator and the
//     dataworkflow routing surfaces (NextStage / MissingValidationStages /
//     BuildLedgerGraph Present / ResultIsFinalAnswerCandidate) must agree on
//     "entity obligation discharged" for every (records, materialized,
//     required) combination. The 2026-07-19 deadlock was exactly a divergence:
//     routing said Present (join_records materialized the stage) while the
//     validator demanded records>0 the emit-stage action set could not
//     produce; six repair rounds were all hard-rejected and a correct answer
//     was withheld.
//  2. Single gate predicate (source pin): every satisfaction read point calls
//     the shared predicate instead of re-deriving the boolean expression.
//  3. Hint/gate consistency: guards that demand ledger work must only hint
//     actions the admission gate can accept at the routed stage.

func pinMeaningfulEntityResolutions(n int) []dataquery.EntityResolutionRecord {
	out := make([]dataquery.EntityResolutionRecord, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, dataquery.EntityResolutionRecord{
			SourceValue: dataquery.LooseText("src"),
			CanonicalID: dataquery.LooseText("canon"),
			Status:      dataquery.LooseText("resolved"),
		})
	}
	return out
}

func pinValidatorEntityMissing(t *testing.T, required bool, records int, materialized bool) bool {
	t.Helper()
	res := dataquery.Result{
		Answer:            "ok",
		EntityResolutions: pinMeaningfulEntityResolutions(records),
	}
	err := dataquery.ValidateResultAgainstContract(
		dataquery.CoverageContract{EntityResolutionRequired: required},
		res,
		dataquery.LedgerSatisfactionFacts{EntityStageMaterialized: materialized},
	)
	if err == nil {
		return false
	}
	if !strings.Contains(err.Error(), "result.entity_resolutions") {
		t.Fatalf("validator raised a non-entity error for required=%v records=%d materialized=%v: %v", required, records, materialized, err)
	}
	return true
}

// TestEntityObligationTwoAuthoritiesSameJudgment is the no-deadlock pin arm
// the 2026-07-19 audit found missing: the routing authority
// ("Present = records>0 || materialized") and the validator authority
// ("satisfied") must be the same judgment, in both directions, and both must
// equal the shared predicate.
func TestEntityObligationTwoAuthoritiesSameJudgment(t *testing.T) {
	bools := []bool{false, true}
	for _, required := range bools {
		for _, records := range []int{0, 3} {
			for _, materialized := range bools {
				satisfied := dataquery.EntityResolutionObligationSatisfied(records, materialized)
				facts := StageFacts{
					MaterialCoverageSufficient: true,
					EntityResolutionRequired:   required,
					EntityResolutionRecords:    records,
					EntityStageMaterialized:    materialized,
				}

				// Routing face: published graph Present.
				dep, ok := pinFindDependency(BuildLedgerGraph(facts), LedgerEntityResolutions)
				if !ok {
					t.Fatalf("entity dependency missing from ledger graph for facts=%+v", facts)
				}
				if dep.Present != satisfied {
					t.Fatalf("graph Present=%v diverges from shared predicate=%v for records=%d materialized=%v",
						dep.Present, satisfied, records, materialized)
				}

				// Routing face: stage machine.
				wantsEntityStage := required && !satisfied
				if got := NextStage(facts) == StageNormalizeOrEnrichEntities; got != wantsEntityStage {
					t.Fatalf("NextStage entity routing=%v diverges from predicate (want %v) for facts=%+v", got, wantsEntityStage, facts)
				}
				missing := false
				for _, stage := range MissingValidationStages(facts) {
					if stage == "entity_resolution" {
						missing = true
					}
				}
				if missing != wantsEntityStage {
					t.Fatalf("MissingValidationStages entity=%v diverges from predicate (want %v) for facts=%+v", missing, wantsEntityStage, facts)
				}

				// Validator face: the workflow-contract validator must issue
				// exactly the same judgment.
				if got := pinValidatorEntityMissing(t, required, records, materialized); got != wantsEntityStage {
					t.Fatalf("validator entity-missing=%v diverges from routing judgment=%v for required=%v records=%d materialized=%v — this is the GAP-3/G10 split-brain",
						got, wantsEntityStage, required, records, materialized)
				}

				// Candidacy face: a result satisfying everything but the
				// entity axis is a final-answer candidate iff the shared
				// predicate holds (or the ledger is not required).
				res := dataquery.Result{
					Answer:            "17,0,5",
					OutputContract:    dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine},
					EntityResolutions: pinMeaningfulEntityResolutions(records),
				}
				candidate := ResultIsFinalAnswerCandidate(
					dataquery.TaskPlan{},
					res,
					dataquery.CoverageContract{EntityResolutionRequired: required},
					res.OutputContract,
					dataquery.LedgerSatisfactionFacts{EntityStageMaterialized: materialized},
				)
				if candidate != !wantsEntityStage {
					t.Fatalf("ResultIsFinalAnswerCandidate=%v diverges from routing judgment (want %v) for required=%v records=%d materialized=%v",
						candidate, !wantsEntityStage, required, records, materialized)
				}
			}
		}
	}
}

// TestEntityObligationHonestFailureIsPreserved pins the fail-loud negative
// arm: when the obligation is genuinely unsatisfiable (required, zero records,
// no materializing artifact anywhere), the validator still refuses, the graph
// still reports missing, the budget decision still fails the workflow — the
// GAP-3 fix removes only the false deadlock, not the honest one.
func TestEntityObligationHonestFailureIsPreserved(t *testing.T) {
	if !pinValidatorEntityMissing(t, true, 0, false) {
		t.Fatal("validator must keep rejecting a genuinely missing entity ledger (no records, no materialization)")
	}
	facts := StageFacts{
		MaterialCoverageSufficient: true,
		EntityResolutionRequired:   true,
	}
	guard := LedgerGraphCompletionGuardResult(BuildLedgerGraph(facts))
	if guard.Empty() {
		t.Fatal("completion guard must fire for a genuinely missing entity ledger")
	}
	decision := DecideDataRoundBudget(DataRoundBudgetDecisionInput{
		DataRounds:      13,
		MaxDataRounds:   13,
		HasResult:       true,
		CompletionGuard: guard,
	})
	if decision.Action != DataRoundBudgetFail {
		t.Fatalf("budget decision=%+v, want honest DataRoundBudgetFail on an unsatisfied obligation", decision)
	}
}

// TestLedgerCompletionGuardHintsStayInsideAllowedSet pins hint/gate
// consistency over the full enumerated universe: whenever the ledger
// completion guard demands work for a required ledger, at least one hinted
// producer action must be admissible at the routed stage. A guard that
// demands work while the admission gate rejects every hinted way to do it is
// the self-contradictory projection that burned six repair rounds in the
// GAP-3/G10 witness.
func TestLedgerCompletionGuardHintsStayInsideAllowedSet(t *testing.T) {
	fired := 0
	for _, facts := range pinEnumerateStageFacts() {
		graph := BuildLedgerGraph(facts)
		guard := LedgerGraphCompletionGuardResult(graph)
		if guard.Empty() || len(guard.Violations) == 0 {
			continue
		}
		// final_projection is an assembly-layer concern, not a validation
		// ledger: any batch result carrying an answer envelope satisfies it
		// (see pinLedgerUniverse), so its assemble_answer hint being absent
		// from a mid-workflow allowed set is not a demand/gate contradiction.
		dep, ok := FirstIncompleteRequiredLedger(graph)
		if !ok || dep.Ledger == string(LedgerFinalProjection) {
			continue
		}
		allowed := ActionKindsFromContracts(AllowedNextActionContractsForFacts(facts))
		if dep.Status == LedgerStatusBlockedByPrerequisite {
			// A blocked ledger's hint names its own producer while the
			// message discloses missing_prerequisites; the actionable demand
			// is the production frontier, and at least one frontier producer
			// must be admissible or the disclosure is a dead end.
			frontier := pinProductionFrontier(graph, []LedgerKind{LedgerKind(dep.Ledger)})
			fired++
			admissible := false
			for _, ledger := range frontier {
				for _, action := range allowed {
					if ProducesLedger(dataquery.DataActionKind(action), ledger) {
						admissible = true
						break
					}
				}
			}
			if !admissible {
				t.Fatalf("blocked ledger %s production frontier %v has no admissible producer in allowed=%v at next_stage=%s for facts=%+v",
					dep.Ledger, frontier, allowed, NextStage(facts), facts)
			}
			continue
		}
		hints := guard.Violations[0].RepairActionHints
		var typedHints []string
		for _, hint := range hints {
			if _, known := Capability(dataquery.DataActionKind(hint)); known {
				typedHints = append(typedHints, hint)
			}
		}
		if len(typedHints) == 0 {
			continue
		}
		fired++
		admissible := false
		for _, hint := range typedHints {
			if pinContains(allowed, hint) {
				admissible = true
				break
			}
		}
		if !admissible {
			t.Fatalf("ledger completion guard hints %v have no admissible member in allowed=%v at next_stage=%s for facts=%+v — guard demands work the gate is guaranteed to reject",
				typedHints, allowed, NextStage(facts), facts)
		}
	}
	if fired == 0 {
		t.Fatal("enumeration produced no guard-with-typed-hints states; the assertion above was never exercised")
	}
}

// TestTerminalRawMaterialGuardHintsStayInsideAllowedSet pins the witnessed
// contradictory hint pair of the GAP-3/G10 burn loop: the terminal
// custom_transform rejection at the emit stage told the repair planner to
// "continue with typed actions (… normalize_entities …)" while the stage gate
// only admitted [reconcile_artifacts, assemble_answer]; the planner obeyed the
// hint and was hard-rejected (witness rejected-r1 → rejected-r6). Both the
// typed RepairActionHints and the prose action list must stay inside the
// published allowed set.
func TestTerminalRawMaterialGuardHintsStayInsideAllowedSet(t *testing.T) {
	allowed := []string{
		string(dataquery.DataActionReconcile),
		string(dataquery.DataActionAssembleAnswer),
	}
	guard := TerminalRawMaterialCustomTransformGuardResult(TerminalRawMaterialCustomTransformGuardInput{
		RecordsPresent: true,
		Action: dataquery.DataAction{
			Kind:   dataquery.DataActionCustomTransform,
			Script: "emit_result(answer='17,0,5')",
		},
		ActionIndex:     0,
		RawInputAliases: []string{"instructions.md", "labels.csv", "observations.csv", "targets.csv"},
		ScriptLines:     10,
		State: WorkflowStateView{
			NextStage:          StageEmitOutputContractAnswer,
			AllowedNextActions: allowed,
		},
	})
	if guard.Empty() || len(guard.Violations) == 0 {
		t.Fatal("terminal raw-material custom_transform guard must fire for multi-material terminal scripts")
	}
	for _, hint := range guard.Violations[0].RepairActionHints {
		if !pinContains(allowed, hint) {
			t.Fatalf("typed repair hint %q outside allowed set %v", hint, allowed)
		}
	}
	message := guard.ErrorText()
	for _, action := range allowed {
		if !strings.Contains(message, action) {
			t.Fatalf("guard message must name the currently allowed action %q: %s", action, message)
		}
	}
	// The message must not recommend stage-inadmissible producers: that exact
	// wording sent the repair planner into a guaranteed rejection.
	for _, forbidden := range []string{
		string(dataquery.DataActionNormalizeEntities),
		string(dataquery.DataActionEnrichRecords),
		string(dataquery.DataActionJoinRecords),
		string(dataquery.DataActionComputeContribs),
		string(dataquery.DataActionDeriveFields),
	} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("guard message recommends %q which the emit-stage gate rejects: %s", forbidden, message)
		}
	}
}

// entityObligationSourcePinSpec names one function body and the textual
// obligations it must (and must not) contain.
type entityObligationSourcePinSpec struct {
	file        string
	function    string
	mustContain []string
	mustAvoid   []string
}

// TestEntityObligationSingleGatePredicate_SourcePin structurally pins "both
// read points, one function": every satisfaction read point must call the
// shared predicate (EntityResolutionObligationSatisfied /
// EntityLedgerSatisfied) and must not re-derive the raw boolean shape whose
// independent copies diverged into the GAP-3/G10 deadlock.
func TestEntityObligationSingleGatePredicate_SourcePin(t *testing.T) {
	rawDerivations := []string{
		"EntityResolutionRecords == 0 && !",
		"EntityResolutionRecords > 0 ||",
		"len(res.EntityResolutions) == 0",
		"len(result.EntityResolutions) == 0",
	}
	specs := []entityObligationSourcePinSpec{
		{file: "stage.go", function: "NextStage", mustContain: []string{"EntityLedgerSatisfied()"}, mustAvoid: rawDerivations},
		{file: "stage.go", function: "MissingValidationStages", mustContain: []string{"EntityLedgerSatisfied()"}, mustAvoid: rawDerivations},
		{file: "stage.go", function: "HasPostRuleProgress", mustContain: []string{"EntityLedgerSatisfied()"}, mustAvoid: rawDerivations},
		{file: "stage.go", function: "EntityLedgerSatisfied", mustContain: []string{"dataquery.EntityResolutionObligationSatisfied("}},
		{file: "ledger_graph.go", function: "BuildLedgerGraph", mustContain: []string{"EntityLedgerSatisfied()"}, mustAvoid: rawDerivations},
		{file: "output_projection.go", function: "ResultIsFinalAnswerCandidate", mustContain: []string{"dataquery.EntityResolutionObligationSatisfied("}, mustAvoid: rawDerivations},
		{file: "../dataquery/dataquery.go", function: "validateRequiredLedgers", mustContain: []string{"EntityResolutionObligationSatisfied("}, mustAvoid: []string{
			"EntityResolutionRecords == 0 && !",
			"EntityResolutionRecords > 0 ||",
			"len(res.EntityResolutions) == 0",
		}},
		{file: "../dataquery/entity_stage.go", function: "EntityResolutionObligationSatisfied", mustContain: []string{"records > 0 || materialized"}},
	}
	for _, spec := range specs {
		body := pinFunctionSource(t, spec.file, spec.function)
		for _, want := range spec.mustContain {
			if !strings.Contains(body, want) {
				t.Errorf("%s:%s must route entity satisfaction through %q (single gate predicate, GAP-3/G10); body:\n%s",
					spec.file, spec.function, want, body)
			}
		}
		for _, avoid := range spec.mustAvoid {
			if strings.Contains(body, avoid) {
				t.Errorf("%s:%s re-derives the raw obligation expression %q instead of calling the shared predicate; body:\n%s",
					spec.file, spec.function, avoid, body)
			}
		}
	}
}

func pinFunctionSource(t *testing.T, file, function string) string {
	t.Helper()
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		t.Fatalf("read %s: %v", file, readErr)
	}
	src := string(raw)
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != function {
			continue
		}
		start := fset.Position(fn.Pos()).Offset
		end := fset.Position(fn.End()).Offset
		return src[start:end]
	}
	t.Fatalf("function %s not found in %s — the source pin census is stale, update it together with the code", function, file)
	return ""
}
