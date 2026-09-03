package types

import (
	"reflect"
	"strings"
	"testing"
)

// write_behavior_contract_ledger_test.go — §40.46 pins (fold-in of the
// V5-3/V5-4 review): retirement is monotonic within a run (C3/C0), the planner
// supersession lane's accept-set equals its retire-set (C2/C5), the ledger
// rides every carrier installed on MutableState, and the context-pack view
// renders retired ids as retired (C1). Red witnesses were run against an
// untouched copy of the staged tree before construction.

func ledgerTestBase() []WriteBehaviorContract {
	return []WriteBehaviorContract{
		{ID: "hard-api", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpEquals, Expected: "public API remains compatible", Required: true, Source: "write_analyzer"},
		{ID: "stale-soft", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "the rejected implementation shape", Required: true, Source: "write_analyzer"},
		{ID: "sibling-soft", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "unrelated soft expectation", Required: true, Source: "write_analyzer"},
		{ID: "planning-only", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "quality-repaired planning prose", Required: false, Source: "write_analyzer;" + WriteBehaviorContractSourcePlanningOnlyUngrounded},
		{ID: "outcome-1", Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "line 16 remains unchanged", Required: false, Source: WriteBehaviorContractSourceExpectedOutcomeFallback + ";" + WriteBehaviorContractSourcePlanningOnlyUngrounded},
	}
}

func ledgerTestHandoff(planID string, attempt int, kind FailureKind, hits ...string) *VerifyFailureHandoff {
	h := &VerifyFailureHandoff{PlanID: planID, BatchID: "batch-1", Attempt: attempt, FailureKind: kind,
		ContractRelevance: &VerifyFailureContractRelevance{Status: VerifyFailureContractRelevanceAvailable, ReasonCode: "typed_failed_rows_joined"}}
	for _, id := range hits {
		h.ContractRelevance.Hits = append(h.ContractRelevance.Hits, VerifyFailureContractHit{ContractID: id, Reason: WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:" + id + "_probe"}})
	}
	return h
}

func TestProjectWriteBehaviorContractGenerationIsMonotonicAcrossHandoffs(t *testing.T) {
	base := ledgerTestBase()
	round1 := ProjectWriteBehaviorContractGeneration(base, nil, ledgerTestHandoff("plan-1", 1, FailureKindTestsFailed, "stale-soft"), []string{"repaired shape"}, nil)
	if status, _ := round1.Lookup("stale-soft"); status != WriteBehaviorContractIDRetired {
		t.Fatalf("round 1 must retire the hit id: %+v", round1)
	}
	for _, round2 := range []*VerifyFailureHandoff{
		ledgerTestHandoff("plan-2", 2, FailureKindBuildFailure),
		ledgerTestHandoff("plan-2", 2, FailureKindTestsFailed, "sibling-soft"),
		nil, // green verify / batch switch: the handoff is gone, the ledger is not
	} {
		got := ProjectWriteBehaviorContractGeneration(base, round1.Tombstones, round2, []string{"repaired again"}, nil)
		status, tombstone := got.Lookup("stale-soft")
		if status != WriteBehaviorContractIDRetired || tombstone == nil {
			t.Fatalf("round 2 (%v) reinstated stale-soft: %+v", round2, got)
		}
		if tombstone.PlanID != "plan-1" || tombstone.Attempt != 1 || !reflect.DeepEqual(tombstone.EvidenceRefs, []string{"probe:stale-soft_probe"}) {
			t.Fatalf("round 2 must keep the ORIGINAL retirement evidence: %+v", tombstone)
		}
		if _, ok := RequiredWriteBehaviorContractIDs(got.Contracts, true)["stale-soft"]; ok {
			t.Fatalf("retired id advertised as required in round 2: %+v", got.Contracts)
		}
		if got.Generation != WriteBehaviorContractGenerationPlanAcceptanceRebase {
			t.Fatalf("a generation under a non-empty ledger is a rebased generation: %q", got.Generation)
		}
		if round2 != nil && round2.FailureKind == FailureKindTestsFailed {
			if status, _ := got.Lookup("sibling-soft"); status != WriteBehaviorContractIDRetired {
				t.Fatalf("round-2 hit must retire the sibling too: %+v", got)
			}
		} else if status, _ := got.Lookup("sibling-soft"); status != WriteBehaviorContractIDActive {
			t.Fatalf("an unrelated failure must retain the sibling: %+v", got)
		}
		// Every emitted generation carries the ledger as of its emission.
		for _, prior := range round1.Tombstones {
			if s, _ := got.Lookup(prior.ID); s != WriteBehaviorContractIDRetired {
				t.Fatalf("prior tombstone %s dropped from the round-2 generation", prior.ID)
			}
		}
	}
	// Planner supersession declared in round 1 survives round 2 unchanged.
	supersede := ProjectWriteBehaviorContractGeneration(base, nil, ledgerTestHandoff("plan-1", 1, FailureKindBuildFailure), nil, []string{"sibling-soft"})
	after := ProjectWriteBehaviorContractGeneration(base, supersede.Tombstones, ledgerTestHandoff("plan-2", 2, FailureKindBuildFailure), nil, nil)
	status, tombstone := after.Lookup("sibling-soft")
	if status != WriteBehaviorContractIDRetired || tombstone.Reason != WriteBehaviorContractRetiredPlannerSupersession || tombstone.PlanID != "plan-1" {
		t.Fatalf("round-1 planner supersession evaporated: %+v", after)
	}
	// The only reinstatement: a new snapshot with no ledger.
	if fresh := ProjectWriteBehaviorContractGeneration(base, nil, nil, nil, nil); len(fresh.Tombstones) != 0 || fresh.Generation != "" || !reflect.DeepEqual(fresh.Contracts, base) {
		t.Fatalf("empty ledger + no handoff must be the snapshot copy: %+v", fresh)
	}
}

func TestWriteBehaviorContractTombstoneLedgerFirstRetirementWins(t *testing.T) {
	var ledger WriteBehaviorContractTombstoneLedger
	if n := ledger.Merge(WriteBehaviorContractTombstone{ID: " a ", Reason: WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:p2", "probe:p1", "probe:p1"}, Attempt: 1}); n != 1 {
		t.Fatalf("first merge added %d", n)
	}
	if n := ledger.Merge(WriteBehaviorContractTombstone{ID: "a", Reason: WriteBehaviorContractRetiredPlannerSupersession, Attempt: 2}, WriteBehaviorContractTombstone{ID: ""}, WriteBehaviorContractTombstone{ID: "b", Attempt: 2}); n != 1 {
		t.Fatalf("re-retirement must not count or overwrite: %d", n)
	}
	rows := ledger.Rows()
	if len(rows) != 2 || rows[0].ID != "a" || rows[0].Attempt != 1 || !reflect.DeepEqual(rows[0].EvidenceRefs, []string{"probe:p1", "probe:p2"}) || rows[1].ID != "b" {
		t.Fatalf("rows = %+v", rows)
	}
	rows[0].EvidenceRefs[0] = "mutated"
	if ledger.Rows()[0].EvidenceRefs[0] != "probe:p1" {
		t.Fatal("Rows must return a copy")
	}
	if merged := MergeWriteBehaviorContractTombstones(rows, WriteBehaviorContractTombstone{ID: "0"}); len(merged) != 3 || merged[0].ID != "0" {
		t.Fatalf("pure union = %+v", merged)
	}
	ledger.Reset()
	if ledger.Len() != 0 {
		t.Fatal("reset")
	}
}

// TestPlannerSupersessionAcceptSetEqualsRetireSet: for every contract shape
// the ONE predicate decides both halves — the validator's accept and the
// rebase's tombstone — so an accepted declaration is never a silent no-op
// (§40.46 C2/C5; the reachable class is the quality-repaired planning-only row).
func TestPlannerSupersessionAcceptSetEqualsRetireSet(t *testing.T) {
	shapes := map[string]WriteBehaviorContract{
		"observed":       {ID: "x", Polarity: WriteBehaviorPolarityObserved, Operator: WriteBehaviorOpSatisfies, Expected: "was failing", Source: "write_analyzer"},
		"hard":           {ID: "x", Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpEquals, Expected: "v", Required: true, Source: "write_analyzer"},
		"grounded":       {ID: "x", Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "v", EvidenceRef: "file:a.go:1", Required: true, Source: "write_analyzer"},
		"grounded_step":  {ID: "x", Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "v", Required: true, Source: "write_analyzer", Transition: &WriteBehaviorTransition{Steps: []WriteBehaviorTransitionStep{{Expected: "s", EvidenceRef: "file:a.go:2"}}}},
		"ungrounded":     {ID: "x", Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "v", Required: true, Source: "write_analyzer"},
		"planning_only":  {ID: "x", Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "v", Required: false, Source: "write_analyzer;" + WriteBehaviorContractSourcePlanningOnlyUngrounded},
		"planning_equal": {ID: "x", Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpEquals, Expected: "v", Required: false, Source: "write_analyzer;" + WriteBehaviorContractSourcePlanningOnlyUngrounded},
		"fallback":       {ID: "x", Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "v", Required: false, Source: WriteBehaviorContractSourceExpectedOutcomeFallback + ";" + WriteBehaviorContractSourcePlanningOnlyUngrounded},
		"optional_soft":  {ID: "x", Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "v", Required: false, Source: "write_analyzer"},
	}
	wantAccept := map[string]bool{"ungrounded": true, "planning_only": true, "planning_equal": true, "fallback": true, "optional_soft": true}
	for name, contract := range shapes {
		accept, class := PlannerSupersedableWriteBehaviorContract(contract)
		if accept != wantAccept[name] {
			t.Fatalf("%s: supersedable=%v (class %q), want %v", name, accept, class, wantAccept[name])
		}
		if !accept && class == "" {
			t.Fatalf("%s: refusal must name the row class", name)
		}
		decision := WriteBehaviorContractRetirementDecision{Lane: FailureKindContractRetainAll, PlannerSupersededIDs: []string{"x"}, PlanID: "plan-1", Attempt: 1, FailureKind: FailureKindBuildFailure}
		rebased, tombstones := RebaseVerifyFailureWriteBehaviorContracts([]WriteBehaviorContract{contract}, nil, decision)
		var planner *WriteBehaviorContractTombstone
		for i := range tombstones {
			if tombstones[i].ID == "x" && tombstones[i].Reason == WriteBehaviorContractRetiredPlannerSupersession {
				planner = &tombstones[i]
			}
		}
		if accept && planner == nil {
			t.Fatalf("%s: accepted declaration minted no planner_supersession tombstone (rebased=%+v tombstones=%+v)", name, rebased, tombstones)
		}
		if accept && (!reflect.DeepEqual(planner.EvidenceRefs, []string{"plan:plan-1"}) || planner.Attempt != 1 || planner.FailureKind != FailureKindBuildFailure) {
			t.Fatalf("%s: planner tombstone lacks evidence/attempt: %+v", name, planner)
		}
		if !accept {
			if planner != nil {
				t.Fatalf("%s: refused row was tombstoned by the planner lane: %+v", name, planner)
			}
			if len(rebased) != 1 || rebased[0].ID != "x" {
				t.Fatalf("%s: refused row must be retained: %+v", name, rebased)
			}
		}
	}
}

func TestMutableStateLedgerCarriersAndPackProjection(t *testing.T) {
	mu := NewMutableState("repair")
	mu.SetWriteAnalysisIR(&WriteAnalysisIR{Request: WriteRequestModel{BehaviorContracts: ledgerTestBase()}})
	// (1) An installed plan seeds the ledger (hydration / import lane).
	mu.SetChangePlan(&ChangePlan{ID: "plan-2", SupersededBehaviorContracts: []WriteBehaviorContractTombstone{{ID: "stale-soft", Reason: WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:shape_probe"}, PlanID: "plan-1", Attempt: 1, FailureKind: FailureKindTestsFailed}}, SupersededBehaviorContractIDs: []string{"legacy-id"}})
	if ids := retiredIDsOf(mu.BehaviorContractTombstoneLedger()); !reflect.DeepEqual(ids, []string{"legacy-id", "stale-soft"}) {
		t.Fatalf("SetChangePlan did not seed the ledger: %v", ids)
	}
	mu.ResetChangePlan()
	// (2) The projection reads the ledger with no handoff at all.
	gen := mu.ProjectBehaviorContractGeneration(nil, nil)
	if status, tombstone := gen.Lookup("stale-soft"); status != WriteBehaviorContractIDRetired || tombstone.PlanID != "plan-1" {
		t.Fatalf("projection ignored the ledger: %+v", gen)
	}
	// (3) The pack view renders the analyzer contract as retired (C1) with
	// the typed reason/evidence and never as a live soft_required row.
	mu.SetWriteContextPack(nil)
	mu.MergeWriteContextPack(WriteContextPackFromWriteAnalysisIR(mu.WriteAnalysisIR()))
	pack := mu.WriteContextPack()
	var retired, live []string
	for _, item := range pack.Items {
		if item.SourceID != "stale-soft" {
			continue
		}
		switch item.Kind {
		case "behavior_contract_retired":
			retired = append(retired, item.Text)
		default:
			live = append(live, item.Kind+": "+item.Text)
		}
	}
	if len(live) != 0 {
		t.Fatalf("retired id still rendered live in the pack view: %v", live)
	}
	if len(retired) != 1 || retired[0] != "id=stale-soft reason=failed_verification_probe evidence=probe:shape_probe failed_plan_id=plan-1 attempt=1" {
		t.Fatalf("retired pack item = %v", retired)
	}
	for _, item := range pack.Items {
		if item.Kind == "behavior_contract" && item.SourceID == "sibling-soft" {
			goto siblingKept
		}
	}
	t.Fatalf("active sibling dropped from the pack view: %+v", pack.Items)
siblingKept:
	// A merged rebased-plan pack already carrying the retired item is not
	// duplicated by the view projection.
	mu.MergeWriteContextPack(WriteContextPackFromChangePlan(&ChangePlan{ID: "plan-2", BehaviorContractGeneration: WriteBehaviorContractGenerationPlanAcceptanceRebase,
		SupersededBehaviorContracts: []WriteBehaviorContractTombstone{{ID: "stale-soft", Reason: WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:shape_probe"}, PlanID: "plan-1", Attempt: 1}}}))
	count := 0
	for _, item := range mu.WriteContextPack().Items {
		if item.Kind == "behavior_contract_retired" && item.SourceID == "stale-soft" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("retired item rendered %d times", count)
	}
	// (4) The run envelope carries the ledger both ways.
	run := NormalizeWriteWorkflowRun(WriteWorkflowRun{RunID: "run-1", BehaviorContractTombstones: []WriteBehaviorContractTombstone{{ID: "from-run", Attempt: 3}, {ID: " from-run "}, {ID: ""}}})
	if len(run.BehaviorContractTombstones) != 1 || run.BehaviorContractTombstones[0].Attempt != 3 {
		t.Fatalf("normalize did not dedupe the envelope ledger: %+v", run.BehaviorContractTombstones)
	}
	mu.SetWriteWorkflowRun(&run)
	if ids := retiredIDsOf(mu.BehaviorContractTombstoneLedger()); !reflect.DeepEqual(ids, []string{"from-run", "legacy-id", "stale-soft"}) {
		t.Fatalf("SetWriteWorkflowRun did not merge the envelope ledger: %v", ids)
	}
	// (5) The only reinstatement lane.
	mu.ResetBehaviorContractTombstoneLedger()
	if mu.BehaviorContractTombstoneLedger() != nil {
		t.Fatal("reset")
	}
	if gen := mu.ProjectBehaviorContractGeneration(nil, nil); len(gen.Tombstones) != 0 {
		t.Fatalf("ledger reset must reinstate: %+v", gen)
	}
	// Read mode / nothing retired: the pack view is byte-identical.
	plain := NewMutableState("read")
	plain.MergeWriteContextPack(WriteContextPackFromWriteAnalysisIR(&WriteAnalysisIR{Request: WriteRequestModel{BehaviorContracts: ledgerTestBase()}}))
	before := plain.WriteContextPack()
	if !reflect.DeepEqual(*before, *plain.WriteContextPack()) || strings.Contains(renderPackKinds(before), "behavior_contract_retired") {
		t.Fatal("view projection must be identity with an empty ledger")
	}
}

// TestExploreForkPackViewIsSameSourcedWithParent: an explorer dispatch fork
// inherits the ledger and the current handoff, so its pack view renders a
// retired contract id exactly as the parent's does (§40.46 C1 — no second
// snapshot of the id space on any rendering surface).
func TestExploreForkPackViewIsSameSourcedWithParent(t *testing.T) {
	mu := NewMutableState("repair")
	ir := &WriteAnalysisIR{Request: WriteRequestModel{BehaviorContracts: ledgerTestBase()}}
	mu.SetWriteAnalysisIR(ir)
	mu.MergeWriteContextPack(WriteContextPackFromWriteAnalysisIR(ir))
	mu.MergeBehaviorContractTombstones(WriteBehaviorContractTombstone{ID: "stale-soft", Reason: WriteBehaviorContractRetiredPlannerSupersession, EvidenceRefs: []string{"plan:plan-1"}, PlanID: "plan-1", Attempt: 1})
	mu.SetVerifyFailureHandoff(ledgerTestHandoff("plan-2", 2, FailureKindTestsFailed, "sibling-soft"))
	fork := mu.ForkForExploreDispatch()
	if !reflect.DeepEqual(fork.BehaviorContractTombstoneLedger(), mu.BehaviorContractTombstoneLedger()) || fork.VerifyFailureHandoff() == nil {
		t.Fatalf("fork did not inherit the ledger/handoff: %+v", fork.BehaviorContractTombstoneLedger())
	}
	if !reflect.DeepEqual(renderPackKinds(fork.WriteContextPack()), renderPackKinds(mu.WriteContextPack())) {
		t.Fatalf("fork pack view diverged from parent:\nfork=%s\nparent=%s", renderPackKinds(fork.WriteContextPack()), renderPackKinds(mu.WriteContextPack()))
	}
	for _, item := range fork.WriteContextPack().Items {
		if item.Kind == "behavior_contract" && (item.SourceID == "stale-soft" || item.SourceID == "sibling-soft") {
			t.Fatalf("fork renders a retired id live: %+v", item)
		}
	}
	// The fork's ledger is a copy: a retirement recorded on the fork never
	// reaches the parent except through the typed carriers.
	fork.MergeBehaviorContractTombstones(WriteBehaviorContractTombstone{ID: "hard-api"})
	if len(mu.BehaviorContractTombstoneLedger()) != 1 {
		t.Fatalf("fork ledger aliased the parent's: %+v", mu.BehaviorContractTombstoneLedger())
	}
}

func retiredIDsOf(rows []WriteBehaviorContractTombstone) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func renderPackKinds(pack *WriteContextPack) string {
	var b strings.Builder
	for _, item := range pack.Items {
		b.WriteString(item.Kind + "\n")
	}
	return b.String()
}
