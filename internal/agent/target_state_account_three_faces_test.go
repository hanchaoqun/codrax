package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// target_state_account_three_faces_test.go — V3-1 acceptance pin
// (colleague_merge_audit §40.20): the observation-ledger scope section, the
// bounded-runtime reader handoff and the final reader decision card all
// render ONE compiled TraceTargetStateScopeAuthority. Before V3-1 each face
// hand-formatted it with its own "uninterruptible wait" caliber (D only /
// D+IO / D "including" IO), so one finalizer prompt could carry
// "uninterruptible 0.500 incl. 0.500" and "uninterruptible 1.000 incl.
// 0.500" for the same account. The three faces now emit exactly
// `- <types.FormatTargetStateAccount(authority, lang)>` and the extracted
// lines are byte-equal.
func targetStateAccountThreeFacesContext() *types.AgentContext {
	const window = "selected_window=10.000000..10.100000"
	start, end := 10.0, 10.1
	ref := types.ObservationSourceRef{
		Kind:       types.ObservationSourceRuntimeArtifact,
		Path:       "/tmp/customer.systrace",
		ArtifactID: "attached_trace",
	}
	observations := []types.ObservationRecord{
		{
			ID: "rank:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRolePrincipalAnswer, GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane: types.ObservationProvenanceObservedDirectCause, SourceRef: ref,
			ClaimKey: "root_cause_primary", Subject: "worker-200", Predicate: "root_cause_primary",
			Object: "priority_inversion_candidate", Value: "8.300", Unit: "ms",
			RichNotes: []string{
				"rank=1", "tier=primary", "effective_impact_ms=8.300", "fix_direction=lock_priority",
				"chain_relevance=on_chain", window,
			},
		},
		{
			ID: "target-state", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: ref,
			ClaimKey:  "target_window_states:app-100", Subject: "app-100", Predicate: "target_window_states",
			Object: "state_partition", Value: "100.000", Unit: "ms",
			RichNotes: []string{
				"running=20.000", "runnable=10.000", "sleep=69.000", "d_state=0.500", "io_wait=0.500",
				"sleep_io_wait=3.000", "total=100.000", window,
			},
		},
	}
	mut := types.NewMutableState("分析目标线程状态")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: observations,
	}}})
	return &types.AgentContext{
		Language: "zh",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "zh",
			Intent:   types.IntentTrace,
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope:        types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState},
			},
			RuntimeTargets: []types.RuntimeTarget{{
				Kind: types.RuntimeTargetKindThread, Thread: "app-100", Source: "user_explicit",
			}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart:      &start, TimeEnd: &end, SourceQuote: "10.0 到 10.1",
			},
		}},
	}
}

func targetStateAccountLine(t *testing.T, face, rendered string) string {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "不可中断等待") && strings.Contains(line, "app-100") {
			lines = append(lines, line)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("%s must publish the target-state account exactly once (got %d lines):\n%s", face, len(lines), rendered)
	}
	return lines[0]
}

func TestTargetStateAccountThreeFacesRenderOneSentence(t *testing.T) {
	ctx := targetStateAccountThreeFacesContext()
	ledger := answerDocObservationLedger(ctx)
	authorities := types.BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
	if len(authorities) != 1 {
		t.Fatalf("fixture must compile exactly one target-state authority, got %d", len(authorities))
	}

	ledgerFace := targetStateAccountLine(t, "observation ledger", renderAnswerDocObservationLedger(ctx))
	handoffFace := targetStateAccountLine(t, "bounded-runtime reader handoff", renderAnswerDocBoundedRuntimeFinalReaderHandoff(ctx))
	cardFace := targetStateAccountLine(t, "final reader decision card", renderTraceFinalReaderDecisionCards(
		types.CompileTraceCausalProjectionSet(ledger), nil, "zh", nil, types.TraceWakeupTargetCPUIntegrity{}, false,
	))
	if ledgerFace != handoffFace || handoffFace != cardFace {
		t.Fatalf("the three prompt faces must render one byte-equal target-state sentence:\nledger : %s\nhandoff: %s\ncard   : %s", ledgerFace, handoffFace, cardFace)
	}
	// FORMATTER-ASSERTIONS-BEGIN (stripped for the red run on the untouched tree)
	if got := authorities[0].UninterruptibleWaitMS(); got != 1.0 {
		t.Fatalf("uninterruptible fold must be D+IO = 1.000, got %.3f", got)
	}
	want := "- " + types.FormatTargetStateAccount(authorities[0], "zh")
	if ledgerFace != want {
		t.Fatalf("the shared line must be the types-level formatter output:\n got %s\nwant %s", ledgerFace, want)
	}
	for _, need := range []string{
		"不可中断等待 1.000 毫秒（其中调度器标记的 IO 等待 0.500 毫秒）",
		"可中断睡眠 69.000 毫秒（其中带 IO 等待标记的可中断睡眠 3.000 毫秒，已含在睡眠内）",
	} {
		if !strings.Contains(want, need) {
			t.Fatalf("formatter sentence missing %q:\n%s", need, want)
		}
	}
	for _, forbidden := range []string{"io_wait", "d_state", "sleep_io_wait"} {
		if strings.Contains(want, forbidden) {
			t.Fatalf("formatter sentence leaked machine token %q:\n%s", forbidden, want)
		}
	}
	// FORMATTER-ASSERTIONS-END
}
