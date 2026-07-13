package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EMIT-2 batch pins (账本 real_trace_campaign_20260705.md §21, 2026-07-07):
// NUM-2 = aggregate_facts cap 三修 (schema pre-announcement + routed rejection
// + optional-lane silent-wipe fix + payload-shape compaction gate + cap
// single-source) and CHAIN-SCOPE = per-artifact wakeup-chain completion-gate
// scoping (维度A③(b): one trace's chain must not exempt another trace's
// chain_required debt; one-shot key carries the artifact identity).

func emit2AggregateFacts(total, principal int) []map[string]any {
	facts := make([]map[string]any, 0, total)
	for i := 0; i < total; i++ {
		role := "audit_ledger"
		if i < principal {
			role = "principal_answer"
		}
		facts = append(facts, map[string]any{
			"kind":       "scalar_value",
			"label":      fmt.Sprintf("runtime fact %02d", i),
			"value":      fmt.Sprintf("%d", i),
			"unit":       "ms",
			"role":       role,
			"provenance": "trace_query.window_stats",
		})
	}
	return facts
}

func emit2TypedAggregateFacts(total, principal int) []types.AnswerAggregateFact {
	facts := make([]types.AnswerAggregateFact, 0, total)
	for i := 0; i < total; i++ {
		role := types.AnswerAggregateRoleAuditLedger
		if i < principal {
			role = types.AnswerAggregateRolePrincipalAnswer
		}
		facts = append(facts, types.AnswerAggregateFact{
			Kind:       types.AnswerAggregateScalar,
			Label:      fmt.Sprintf("runtime fact %02d", i),
			Value:      fmt.Sprintf("%d", i),
			Unit:       "ms",
			Role:       role,
			Provenance: "trace_query.window_stats",
		})
	}
	return facts
}

// TestEmitInvestigationCompleteSchema_PreAnnouncesAggregateFactsCap pins
// NUM-2 C (§21 维度C②): the tool schema must pre-announce the cap value and
// the preferred consolidation shape so the model never has to learn the cap
// by burning a rejected round; the number is single-sourced from
// types.MaxAnswerAggregateFacts (维度C④ hygiene).
func TestEmitInvestigationCompleteSchema_PreAnnouncesAggregateFactsCap(t *testing.T) {
	params := string((&EmitInvestigationComplete{}).Parameters())
	for _, want := range []string{
		fmt.Sprintf(`"maxItems": %d`, types.MaxAnswerAggregateFacts),
		fmt.Sprintf("HARD CAP: at most %d entries per call", types.MaxAnswerAggregateFacts),
		"merge same-family per-group scalars into ONE grouped_count fact",
		"truncated by role priority",
	} {
		if !strings.Contains(params, want) {
			t.Fatalf("schema must pre-announce the aggregate_facts cap; missing %q in:\n%s", want, params)
		}
	}
	if strings.Contains(params, "__AGG_FACTS_CAP__") {
		t.Fatalf("cap placeholder must be fully substituted in the published schema:\n%s", params)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(params), &decoded); err != nil {
		t.Fatalf("schema must stay valid JSON after cap substitution: %v", err)
	}
}

// TestNormalizeCompletionAggregateFacts_CapCompactsWithoutRuntimeScene pins
// NUM-2 A / N3 (§21 维度C①): the cap compaction is keyed on the payload shape
// (principal_answer facts fit the cap), NOT on an origin-lane scene gate. A
// plain non-runtime context — exactly the shape whose closed scene gate burned
// the cmp_01 rounds (line 106/161) — now self-heals by role-priority
// truncation with a disclosure note. The cap VALUE itself stays 16 per the
// 维度C self-heal assessment (预告+路由+自愈, 不动 cap 值).
func TestNormalizeCompletionAggregateFacts_CapCompactsWithoutRuntimeScene(t *testing.T) {
	ctx := &types.BusContext{
		Mutable:    types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{}},
	}
	facts, notes, err := normalizeCompletionAggregateFacts(ctx, "resolved",
		emit2TypedAggregateFacts(types.MaxAnswerAggregateFacts+1, 1))
	if err != nil {
		t.Fatalf("payload-shape compaction must self-heal a %d>%d overflow without a scene gate: %v",
			types.MaxAnswerAggregateFacts+1, types.MaxAnswerAggregateFacts, err)
	}
	if len(facts) != types.MaxAnswerAggregateFacts {
		t.Fatalf("expected %d compacted facts, got %d", types.MaxAnswerAggregateFacts, len(facts))
	}
	if facts[0].Label != "runtime fact 00" {
		t.Fatalf("principal fact must survive compaction first, got %+v", facts[0])
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, fmt.Sprintf("compacted from %d to %d", types.MaxAnswerAggregateFacts+1, types.MaxAnswerAggregateFacts)) {
		t.Fatalf("compaction must be disclosed in notes, got: %q", joined)
	}
}

// TestNormalizeCompletionAggregateFacts_PrincipalOverCapStaysHardReject pins
// the single precise carve-out of NUM-2 A: when the principal_answer slate
// ALONE exceeds the cap, truncation would silently drop parts of the ANSWER,
// so the payload stays a hard reject carrying the typed cap error (the routed
// rejection then teaches consolidation).
func TestNormalizeCompletionAggregateFacts_PrincipalOverCapStaysHardReject(t *testing.T) {
	ctx := &types.BusContext{
		Mutable:    types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{}},
	}
	overCap := types.MaxAnswerAggregateFacts + 1
	_, _, err := normalizeCompletionAggregateFacts(ctx, "resolved", emit2TypedAggregateFacts(overCap, overCap))
	if err == nil {
		t.Fatalf("a principal_answer slate over the cap must stay a hard reject")
	}
	var capErr *types.AggregateFactsCapExceededError
	if !errors.As(err, &capErr) || capErr.Count != overCap {
		t.Fatalf("cap reject must carry the typed cap error with the offending count, got: %v", err)
	}
}

// TestEmitInvestigationComplete_AggregateFactsCapRejectCarriesRouting pins
// NUM-2 N1 (§21 维度C②, NUM 根因2 孪生): the residual hard-reject path must
// teach the concrete edit — current count vs cap, per-role tally, and the
// merge/trim/move-to-reason routes — instead of only restating the limit.
func TestEmitInvestigationComplete_AggregateFactsCapRejectCarriesRouting(t *testing.T) {
	overCap := types.MaxAnswerAggregateFacts + 1
	bus := &types.BusContext{
		Mutable:    types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{}},
	}
	payload := map[string]any{
		"reason":          "cap overflow with an all-principal slate",
		"confidence":      "high",
		"result_kind":     "resolved",
		"aggregate_facts": emit2AggregateFacts(overCap, overCap),
	}
	params, _ := json.Marshal(payload)
	res, err := (&EmitInvestigationComplete{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("an over-cap principal slate must be rejected, got success: %s", res.Summary)
	}
	for _, want := range []string{
		fmt.Sprintf("has %d entries; max %d", overCap, types.MaxAnswerAggregateFacts),
		fmt.Sprintf("role tally: principal_answer=%d", overCap),
		fmt.Sprintf("the budget is %d entries", types.MaxAnswerAggregateFacts),
		"grouped_count",
		"move audit-only details into reason",
		"consolidate them into fewer aggregate rows",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("cap rejection must carry operation routing; missing %q in: %s", want, res.Summary)
		}
	}
}

// TestEmitInvestigationComplete_OptionalAggregateFactsCapOverflowNotWiped pins
// NUM-2 N2 (§21 维度C③, P1 静默清空 bug): an optional-lane payload whose
// principal slate exceeds the cap used to be WIPED ENTIRELY on the merge
// re-validation cap error — 17 individually valid facts silently discarded
// with a successful emit. The fix compacts by role priority to the cap and
// discloses; the facts must never come back empty.
func TestEmitInvestigationComplete_OptionalAggregateFactsCapOverflowNotWiped(t *testing.T) {
	overCap := types.MaxAnswerAggregateFacts + 1
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			Scenario:  types.ScenarioRootCause,
			LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "runtime trace"}}},
		}},
	}
	payload := map[string]any{
		"reason":          "runtime facts collected",
		"confidence":      "high",
		"result_kind":     "resolved",
		"aggregate_facts": emit2AggregateFacts(overCap, overCap),
	}
	params, _ := json.Marshal(payload)
	res, err := (&EmitInvestigationComplete{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("optional-lane cap overflow should truncate-and-disclose, not fail: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) == 0 {
		t.Fatalf("REGRESSION (silent wipe): %d optional aggregate facts must never be cleared to zero on cap overflow", overCap)
	}
	if len(got) != types.MaxAnswerAggregateFacts {
		t.Fatalf("expected %d kept facts after cap truncation, got %d", types.MaxAnswerAggregateFacts, len(got))
	}
	if !strings.Contains(res.Summary, fmt.Sprintf("kept %d by role priority", types.MaxAnswerAggregateFacts)) {
		t.Fatalf("cap truncation must be disclosed on the returned summary, got: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "ignored optional aggregate_facts payload after merge validation error") {
		t.Fatalf("the silent-wipe note branch must not fire for a cap-class merge error: %s", res.Summary)
	}
}

// --- CHAIN-SCOPE (§21 维度A③(b)) ---

func emit2WakeupGatePreflight(sources ...string) types.RuntimeArtifactPreflightProfile {
	artifacts := make([]types.RuntimeArtifactPreflightArtifact, 0, len(sources))
	for _, source := range sources {
		artifacts = append(artifacts, types.RuntimeArtifactPreflightArtifact{
			Kind: "trace", Source: source, Carrier: "request_path",
		})
	}
	return types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		SourceNavigationOptional: true,
		Artifacts:                artifacts,
		RepoSourceCensus:         types.RuntimeArtifactRepoSourceCensus{Completed: true, ArtifactFiles: len(sources)},
	})
}

func emit2ChainRequiredResult(path, subject, window string) types.ToolResult {
	return types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{{
			ID:        "trace_query:" + path + "#state_drilldown:1",
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "trace_query",
			Subject:   subject,
			Predicate: "state_drilldown",
			Object:    "s_sleep",
			RichNotes: []string{"source=top_sleep", "chain_required=true", "selected_window=" + window},
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				Path:         path,
				ArtifactKind: "trace",
			},
		}},
	}
}

func emit2WakeupFamilyResult(path string) types.ToolResult {
	return types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{{
			ID:        "trace_query:" + path + "#wakeup_causal_impact:1",
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "trace_query",
			Subject:   "waker-100",
			Predicate: "wakeup_causal_impact",
			Object:    "target-200",
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				Path:         path,
				ArtifactKind: "trace",
			},
		}},
	}
}

func emit2StateChurnSiblingResult(path, subject, window string) types.ToolResult {
	return types.ToolResult{
		ToolName:  "trace_query",
		Success:   true,
		Timestamp: time.Now(),
		Observations: []types.ObservationRecord{{
			ID:        "trace_query:" + path + "#state_drilldown:sibling",
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "trace_query",
			Subject:   subject,
			Predicate: "state_drilldown",
			Object:    "s_sleep",
			RichNotes: []string{"source=state_churn", "chain_required=false", "selected_window=" + window},
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				Path:         path,
				ArtifactKind: "trace",
			},
		}},
	}
}

// TestEmitInvestigationComplete_WakeupChainGatePerArtifactScope pins
// CHAIN-SCOPE (§21 维度A③(b), cmp_01 dual-trace specimen) on the §29.60
// advisory surface: the wakeup-family exemption is scoped to the observing
// artifact. One trace's completed chain must NOT exempt the other trace's
// chain_required=true debt — the owed side draws the one-shot advisory note,
// named by its own artifact basename; the same artifact carrying its own
// chain never draws the note. Completion always passes (§29.60 flip).
func TestEmitInvestigationComplete_WakeupChainGatePerArtifactScope(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	const traceNew = "/tmp/cmp/7.0B30SP22_7315.systrace"
	const traceOld = "/tmp/cmp/6.0B138_3900.sys.systrace"
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"双 trace 对比:7.0 已有唤醒链,6.0 侧 sleep 主导",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	preflight := emit2WakeupGatePreflight("7.0B30SP22_7315.systrace", "6.0B138_3900.sys.systrace")
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}}

	// ① Dual-artifact debt: the 7.0 trace has a full wakeup chain, the 6.0
	// trace owes one — the 7.0 chain must not waive the 6.0 note.
	mut := types.NewMutableState("对比 bindApplication 双 trace")
	mut.AppendDispatchToolResult(emit2WakeupFamilyResult(traceNew))
	mut.AppendDispatchToolResult(emit2ChainRequiredResult(traceOld, "com.xs.fm.lite-21538", "8144.608000..8144.708000"))
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RuntimeArtifactPreflight: preflight}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("①: completion must pass (§29.60 — the arm never blocks), got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("①: the other artifact's chain must not exempt this artifact's chain_required debt — note must fire, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "com.xs.fm.lite-21538") ||
		!strings.Contains(res.Summary, "6.0B138_3900.sys.systrace") {
		t.Fatalf("①: note must name the owed subject and its own trace artifact, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "DIFFERENT trace artifact") {
		t.Fatalf("①: dual-trace guidance must explain the per-artifact mirror scope, got: %s", res.Summary)
	}
	// One-shot: the second attempt passes without repeating the note (6.0's
	// note is burned).
	second, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !second.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("①: second attempt must pass, got: %s", second.Summary)
	}
	if strings.Contains(second.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("①: one note per artifact — second attempt must not repeat it, got: %s", second.Summary)
	}

	// ② Same artifact with its own chain: never noted.
	mut2 := types.NewMutableState("单 trace 已有链")
	mut2.AppendDispatchToolResult(emit2WakeupFamilyResult(traceOld))
	mut2.AppendDispatchToolResult(emit2ChainRequiredResult(traceOld, "com.xs.fm.lite-21538", "8144.608000..8144.708000"))
	bus2 := &types.BusContext{Mutable: mut2, AnalysisIR: ir, RuntimeArtifactPreflight: preflight}
	res2, err := tool.Execute(bus2, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res2.Success || strings.TrimSpace(mut2.InvestigationCompleteReason()) == "" {
		t.Fatalf("②: an artifact carrying its own wakeup chain must not be gated, got: %s", res2.Summary)
	}
	if strings.Contains(res2.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("②: no note expected for a self-covered artifact, got: %s", res2.Summary)
	}
}

// TestEmitInvestigationComplete_WakeupChainGateOneShotPerArtifact pins the
// CHAIN-SCOPE one-shot semantics (§21 维度A③(b)) on the §29.60 advisory
// surface: each artifact gets AT MOST one note per run, and the one-shots are
// independent — burning artifact A's note must not consume artifact B's. The
// scan-order pick means successive completions surface successive artifacts'
// notes; completion passes from the FIRST attempt (§29.60 flip).
func TestEmitInvestigationComplete_WakeupChainGateOneShotPerArtifact(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	const traceA = "/tmp/cmp/first.systrace"
	const traceB = "/tmp/cmp/second.systrace"
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"两侧均 sleep 主导且都未跑链",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	mut := types.NewMutableState("双 trace 双欠账")
	mut.AppendDispatchToolResult(emit2ChainRequiredResult(traceA, "thread-a-1", "100.000000..160.000000"))
	mut.AppendDispatchToolResult(emit2ChainRequiredResult(traceB, "thread-b-2", "200.000000..260.000000"))
	bus := &types.BusContext{
		Mutable:                  mut,
		AnalysisIR:               &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
		RuntimeArtifactPreflight: emit2WakeupGatePreflight("first.systrace", "second.systrace"),
	}

	first, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !first.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("attempt 1 must pass (§29.60 — the arm never blocks), got: %s", first.Summary)
	}
	if !strings.Contains(first.Summary, "thread-a-1") || !strings.Contains(first.Summary, "first.systrace") {
		t.Fatalf("attempt 1 must note the first owed artifact, got: %s", first.Summary)
	}
	second, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !second.Success {
		t.Fatalf("attempt 2 must pass, got: %s", second.Summary)
	}
	if !strings.Contains(second.Summary, "thread-b-2") || !strings.Contains(second.Summary, "second.systrace") {
		t.Fatalf("attempt 2 must note the second artifact independently (per-artifact one-shot), got: %s", second.Summary)
	}
	third, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !third.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("attempt 3 must pass, got: %s", third.Summary)
	}
	if strings.Contains(third.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("attempt 3 must not repeat any note — every artifact's single note is spent, got: %s", third.Summary)
	}
}

// TestEmitInvestigationComplete_WakeupChainSiblingReconcileArtifactScoped pins
// the CHAIN-SCOPE secondary fix (§21 维度A③(b) 次级): the state_churn sibling
// reconcile is artifact-scoped — a same-name thread's churn face from the
// OTHER trace must not waive this trace's drilldown obligation (subject labels
// are noisy across artifacts); the same artifact's overlapping churn face
// still reconciles exactly as CHAIN-RECONCILE pinned.
func TestEmitInvestigationComplete_WakeupChainSiblingReconcileArtifactScoped(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	const traceNew = "/tmp/cmp/7.0B30SP22_7315.systrace"
	const traceOld = "/tmp/cmp/6.0B138_3900.sys.systrace"
	const subject = "com.xs.fm.lite-21538" // same-name thread on BOTH traces
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"同名线程跨 trace churn 面",
		"confidence":"high",
		"result_kind":"resolved"
	}`)
	preflight := emit2WakeupGatePreflight("7.0B30SP22_7315.systrace", "6.0B138_3900.sys.systrace")
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}}

	// ① Cross-artifact sibling: overlapping window, same subject label, but
	// observed on the OTHER trace → reconciles nothing, the note fires (and
	// completion passes — §29.60 flip).
	mut := types.NewMutableState("跨工件假兄弟")
	mut.AppendDispatchToolResult(emit2ChainRequiredResult(traceOld, subject, "8144.608000..8144.708000"))
	mut.AppendDispatchToolResult(emit2StateChurnSiblingResult(traceNew, subject, "8144.000000..8145.000000"))
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RuntimeArtifactPreflight: preflight}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Summary, wakeupAdvisoryNoteMarker) {
		t.Fatalf("①: a same-name thread's churn face from ANOTHER artifact must not reconcile — note must fire, got: %s", res.Summary)
	}
	if !res.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("①: completion must pass (§29.60 — the arm never blocks), got: %s", res.Summary)
	}

	// ② Same-artifact sibling: identical shape observed on the SAME trace →
	// reconciles, no note (CHAIN-RECONCILE behavior preserved in-scope).
	mut2 := types.NewMutableState("同工件真兄弟")
	mut2.AppendDispatchToolResult(emit2ChainRequiredResult(traceOld, subject, "8144.608000..8144.708000"))
	mut2.AppendDispatchToolResult(emit2StateChurnSiblingResult(traceOld, subject, "8144.000000..8145.000000"))
	bus2 := &types.BusContext{Mutable: mut2, AnalysisIR: ir, RuntimeArtifactPreflight: preflight}
	res2, err := tool.Execute(bus2, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(res2.Summary, wakeupAdvisoryNoteMarker) || !res2.Success ||
		strings.TrimSpace(mut2.InvestigationCompleteReason()) == "" {
		t.Fatalf("②: the same artifact's overlapping churn face must still reconcile, got: %s", res2.Summary)
	}
}
