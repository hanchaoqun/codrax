package tool

// AUTOREPAIR-1 (§29.175 用户裁定①) completion-layer pins:
//
//   件2 T2-DISCLOSE-SILENT-BACKFILLS — every Tier2 backfill arm surfaces a
//     single-wording-point zh note in the accepted summary; an
//     already-canonical fact yields zero notes; and when the post-repair
//     payload still rejects, the reject carries BOTH the applied-repair
//     notes and the accumulated violation list (组合合同 with EMITBURN-1
//     件1).
//   件3② T2-UNIT-SUFFIX-SPLIT — three arms: split repair with disclosure /
//     IsCountQuestion carve-out keeps the legacy reject verbatim /
//     unit-conflict keeps the reject; plus the fixed point.
//   件3③ — the types-layer kind fold is disclosed through the differ note.
//   件4 — structural call-order pin: transport repair → decode compat →
//     Tier2 normalize → types validation → collection → 件1 full-error
//     report; normalize always sits before the validation gate.

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func autoRepairNoteCount(notes []string, marks ...string) int {
	count := 0
	for _, note := range notes {
		for _, mark := range marks {
			if strings.Contains(note, mark) {
				count++
				break
			}
		}
	}
	return count
}

// 件2 positive arm (accepted ToolResult.Summary carries the disclosure):
// origin backfilled from the model's own provenance slot, scope /
// result_count / searched_at appended by the types layer — every fired
// backfill named in the accepted summary with the single-wording-point
// formats.
func TestAutoRepairTier2DisclosureInAcceptedSummary(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut, AttachedHitrace: "systrace excerpt"}
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"reason":"autorepair tier2 disclosure positive arm",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{"kind":"negative_observation","label":"no wakeup in window","value":"0","provenance":"runtime_artifact",
			 "dimensions":[{"name":"target","value":"wakeup"},{"name":"artifact_id","value":"t1"}]}]
	}`))
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if !res.Success {
		t.Fatalf("backfilled negative_observation must accept, got: %s", res.Summary)
	}
	for _, want := range []string{
		"aggregate_facts[0] 已补注 origin=runtime_artifact(由 provenance 推导)",
		"aggregate_facts[0] 已补注 result_count=0(由 value 推导)",
		"aggregate_facts[0] 已补注 scope=t1(由 同事实维度 推导)",
		"aggregate_facts[0] 已补注 searched_at=current_investigation(默认值,非时间戳断言)",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("accepted summary must disclose %q, got: %s", want, res.Summary)
		}
	}
}

// 件2 negative arm (delta=0 → zero notes): an already-canonical
// negative_observation produces NO backfill disclosure.
func TestAutoRepairTier2ZeroNotesOnCanonicalFact(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("q")}
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeObservation,
		Label: "no wakeup in window",
		Value: "0",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "runtime_artifact"},
			{Name: "target", Value: "wakeup"},
			{Name: "scope", Value: "trace window 10.0..10.2"},
			{Name: "searched_at", Value: "current_investigation"},
			{Name: "result_count", Value: "0"},
		},
	}}
	_, notes, err := normalizeCompletionAggregateFacts(bus, "resolved", facts)
	if err != nil {
		t.Fatalf("canonical fact must accept, got: %v", err)
	}
	if n := autoRepairNoteCount(notes, "已补注", "已按", "词形归一"); n != 0 {
		t.Fatalf("canonical fact must yield zero repair notes, got: %v", notes)
	}
}

// 件2 arm coverage at the unit level: target from excluded[0] and the
// value-zero normalization note.
func TestAutoRepairTier2TargetFromExcludedAndValueZeroNotes(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("q")}
	facts := []types.AnswerAggregateFact{{
		Kind:     types.AnswerAggregateNegativeObservation,
		Label:    "no binder reply in window",
		Excluded: []string{"binder transaction reply"},
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "runtime_artifact"},
			{Name: "scope", Value: "trace window"},
		},
	}}
	_, notes, err := normalizeCompletionAggregateFacts(bus, "resolved", facts)
	if err != nil {
		t.Fatalf("fact must accept after backfills, got: %v", err)
	}
	joined := strings.Join(notes, "; ")
	for _, want := range []string{
		"aggregate_facts[0] 已补注 target=binder transaction reply(由 excluded[0] 推导)",
		"aggregate_facts[0] 已按 result_count 归一 value=0",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("notes must contain %q, got: %v", want, notes)
		}
	}
}

// 件2 恰一门 arm: external-only runtime artifact run backfills origin with
// the 附加运行时工件 source token; when the fact carries its own provenance
// the provenance arm keeps precedence and exactly one origin note fires.
func TestAutoRepairTier2ExternalOnlyArtifactOriginArm(t *testing.T) {
	externalOnly := &types.BusContext{
		Mutable: types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{}}},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeObservation,
		Label: "no wakeup in window",
		Value: "0",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "target", Value: "wakeup"},
		},
	}}
	_, notes := normalizeCompletionNegativeObservationFacts(externalOnly, facts)
	joined := strings.Join(notes, "; ")
	if !strings.Contains(joined, "aggregate_facts[0] 已补注 origin=runtime_artifact(由 附加运行时工件 推导)") {
		t.Fatalf("恰一门 arm must disclose with the 附加运行时工件 source, got: %v", notes)
	}

	withProvenance := []types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateNegativeObservation,
		Label:      "no wakeup in window",
		Value:      "0",
		Provenance: "runtime_artifact",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "target", Value: "wakeup"},
		},
	}}
	_, notes = normalizeCompletionNegativeObservationFacts(externalOnly, withProvenance)
	originNotes := autoRepairNoteCount(notes, "已补注 origin=")
	if originNotes != 1 || !strings.Contains(strings.Join(notes, "; "), "(由 provenance 推导)") {
		t.Fatalf("provenance arm must keep precedence with exactly one origin note, got: %v", notes)
	}
}

// 件2 组合合同 pin (硬纪律 3): when the post-repair payload STILL rejects,
// the reject summary carries the applied-repair notes alongside the
// accumulated violation list — the model must not re-fix what the system
// fixed.
func TestAutoRepairTier2RejectCarriesAppliedRepairNotes(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("q"), AttachedHitrace: "systrace excerpt"}
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"reason":"autorepair composition arm: repairs applied yet payload still rejects",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{"kind":"negative_observation","label":"no binder reply in window","value":"0",
			 "excluded":["binder transaction reply"]}]
	}`))
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success {
		t.Fatalf("origin-less fact must still reject, got: %s", res.Summary)
	}
	for _, want := range []string{
		"requires a non-repo origin dimension",
		"system repairs already applied (keep them in the re-emit): ",
		"aggregate_facts[0] 已补注 target=binder transaction reply(由 excluded[0] 推导)",
		"aggregate_facts[0] 已补注 scope=attached_runtime_trace(由 附加运行时工件 推导)",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("reject must carry %q, got: %s", want, res.Summary)
		}
	}
}

// 件3② three arms + fixed point.
func TestAutoRepairUnitSuffixSplitThreeArms(t *testing.T) {
	plain := &types.BusContext{Mutable: types.NewMutableState("q")}

	// Positive: exact lexical split with disclosure.
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateGroupedCount,
		Label: "vsync 到 doFrame 延迟",
		Value: "10.503ms",
	}}
	out, notes, err := normalizeCompletionAggregateFacts(plain, "resolved", facts)
	if err != nil {
		t.Fatalf("unit-suffixed decimal must repair, got: %v", err)
	}
	if len(out) != 1 || out[0].Kind != types.AnswerAggregateScalar || out[0].Value != "10.503" || out[0].Unit != "ms" {
		t.Fatalf("split must land scalar_value/10.503/ms, got %+v", out)
	}
	if !strings.Contains(strings.Join(notes, "; "), `kind normalized grouped_count→scalar_value;值 "10.503ms" 拆为 value=10.503 unit=ms(计数类须整数)`) {
		t.Fatalf("split must disclose with the matrix wording, got: %v", notes)
	}

	// Mutation 1: IsCountQuestion carve-out keeps the legacy reject verbatim.
	countQuestion := &types.BusContext{
		Mutable: types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{IsCountQuestion: true},
		}},
	}
	_, _, err = normalizeCompletionAggregateFacts(countQuestion, "resolved", facts)
	if err == nil {
		t.Fatalf("IsCountQuestion run must keep the hard reject")
	}
	if !strings.Contains(err.Error(), `has non-integer count value "10.503ms"; for a measurement like this use kind=scalar_value, keep value numeric, and put the unit text in unit`) {
		t.Fatalf("carve-out must keep the legacy reject wording, got: %v", err)
	}

	// Mutation 2: a conflicting unit disqualifies the repair — the
	// contradiction is the model's to resolve.
	conflict := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateGroupedCount,
		Label: "vsync 到 doFrame 延迟",
		Value: "10.503ms",
		Unit:  "s",
	}}
	_, _, err = normalizeCompletionAggregateFacts(plain, "resolved", conflict)
	if err == nil {
		t.Fatalf("unit-conflicting suffix must keep the hard reject")
	}
	if !strings.Contains(err.Error(), "has non-integer count value") {
		t.Fatalf("unit conflict must keep the count reject family, got: %v", err)
	}

	// Fixed point: the repaired fact re-enters the compat pass unchanged.
	repairedOnce, onceNotes := normalizeCompletionAggregateFactCompat(plain, facts)
	if len(onceNotes) == 0 {
		t.Fatalf("first pass must repair and note")
	}
	repairedTwice, twiceNotes := normalizeCompletionAggregateFactCompat(plain, repairedOnce)
	if len(twiceNotes) != 0 {
		t.Fatalf("second pass must be a no-op, got notes: %v", twiceNotes)
	}
	if !reflect.DeepEqual(repairedTwice[0], repairedOnce[0]) {
		t.Fatalf("second pass must not change the fact:\n once: %+v\ntwice: %+v", repairedOnce[0], repairedTwice[0])
	}
}

// 件3③ disclosure: the types-layer lexical kind fold surfaces through the
// completion differ note.
func TestAutoRepairKindFoldDisclosureNote(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("q")}
	facts := []types.AnswerAggregateFact{{
		Kind:    "Member_Set",
		Label:   "blocking chain principal threads",
		Value:   "2",
		Members: []string{"ThreadA-1", "ThreadB-2"},
	}}
	out, notes, err := normalizeCompletionAggregateFacts(bus, "resolved", facts)
	if err != nil {
		t.Fatalf("folded kind variant must accept, got: %v", err)
	}
	if len(out) != 1 || out[0].Kind != types.AnswerAggregateMemberSet {
		t.Fatalf("kind must canonicalize, got %+v", out)
	}
	if !strings.Contains(strings.Join(notes, "; "), `aggregate_facts[0] kind 词形归一 "Member_Set"→member_set`) {
		t.Fatalf("fold must disclose through the differ note, got: %v", notes)
	}
}

// 件4 structural call-order pin: normalize sits before the validation gate
// and the 件1 full-error report reads the post-repair payload. Source-order
// probe — a refactor that moves a backfill behind the gate (re-opening the
// burn) trips this before any behavioral test notices.
func TestAutoRepairToolCallOrderStructuralPin(t *testing.T) {
	src, err := os.ReadFile("emit_investigation_complete.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	text := string(src)

	body := autoRepairExtractFuncBody(t, text, "func normalizeCompletionAggregateFacts(")
	order := []string{
		"normalizeCompletionNegativeObservationFacts(",
		"normalizeCompletionAggregateFactCompat(",
		"types.NormalizeAnswerAggregateFacts(",
		"aggregateFactTypesLayerBackfillNotes(",
	}
	last := -1
	for _, name := range order {
		idx := strings.Index(body, name)
		if idx < 0 {
			t.Fatalf("call %q not found in normalizeCompletionAggregateFacts", name)
		}
		if idx <= last {
			t.Fatalf("call %q out of order: Tier2 normalize must run before types validation", name)
		}
		last = idx
	}

	decodeBody := autoRepairExtractFuncBody(t, text, "func decodeEmitInvestigationCompleteParamsStrict(")
	compatIdx := strings.Index(decodeBody, "applyStructuredPayloadCompat(")
	strictIdx := strings.Index(decodeBody, "decodeStrictNormalizedToolParams(")
	if compatIdx < 0 || strictIdx < 0 || compatIdx >= strictIdx {
		t.Fatalf("decode compat must run before the strict decode (compat=%d strict=%d)", compatIdx, strictIdx)
	}

	execBody := autoRepairExtractFuncBody(t, text, "func (t *EmitInvestigationComplete) Execute(")
	normIdx := strings.Index(execBody, "normalizeCompletionAggregateFacts(")
	reportIdx := strings.Index(execBody, "completionAggregateFactsCollectViolations(")
	if normIdx < 0 || reportIdx < 0 || normIdx >= reportIdx {
		t.Fatalf("the 件1 full-error report must read the post-repair residue (normalize=%d report=%d)", normIdx, reportIdx)
	}
}

func autoRepairExtractFuncBody(t *testing.T, src, decl string) string {
	t.Helper()
	start := strings.Index(src, decl)
	if start < 0 {
		t.Fatalf("declaration %q not found", decl)
	}
	rest := src[start+len(decl):]
	end := strings.Index(rest, "\nfunc ")
	if end < 0 {
		return rest
	}
	return rest[:end]
}
