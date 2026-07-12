package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_projection_idle_cadence_eng2_test.go — ENG-2 pins
// (复核冷读 CP1-③, 2026-07-12): the P9 arm-c idle reclassification must
// survive onto a RENDERED seat. Production witness (donghu 84618 replay):
// the engine minted root_cause_context_only:pacing_idle for the 15.758ms
// frame-pacing segment, but the R1 same-fact fold absorbed it into the
// co-located self-sleep twin and shunted the token to SecondaryObjects —
// which no display face reads — so the whole report carried ZERO
// 帧间空闲 wording while the audit face still published the row.

// eng2IdleCadenceRecords reproduces the donghu emit geometry through the
// REAL record→projection compile (the fold lives there): the target's own
// s_sleep hop view, the SAME segment's pacing_idle rank view (context_only,
// adjacent channel) and its root_evidence witness — all three share
// subject + %.3f impact + line span (the R1 same-fact key).
func eng2IdleCadenceRecords(idleToken string) []types.ObservationRecord {
	return []types.ObservationRecord{
		{
			ID:              "eng2-state-hop",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "wakeup_causal_impact",
			ClaimKey:        "wakeup_causal_impact:s_sleep",
			Subject:         ".ugc.aweme.lite-17267",
			Object:          "s_sleep",
			Value:           "15.758",
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: 23091, LineEnd: 24430},
			RichNotes: []string{
				"impact_ms=15.758", "cumulative_impact_ms=15.758",
				"chain_relevance=on_chain", "causality=on_wakeup_chain",
				"chain_depth=0", "dominant_state=s_sleep",
			},
			Confidence: 0.82,
		},
		{
			ID:              "eng2-idle-rank",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_context_only",
			ClaimKey:        "root_cause_context_only",
			Subject:         ".ugc.aweme.lite-17267",
			Object:          idleToken,
			Value:           "15.758",
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: 23091, LineEnd: 24430},
			RichNotes: []string{
				"tier=context_only", "impact_ms=15.758",
				"cumulative_impact_ms=15.758", "effective_impact_ms=0.000",
				"chain_relevance=adjacent", "causality=adjacent_to_wakeup_chain",
			},
			Confidence: 0.85,
		},
		{
			ID:              "eng2-idle-rootev",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       idleToken,
			ClaimKey:        "root_evidence:" + idleToken,
			Subject:         ".ugc.aweme.lite-17267",
			Value:           "15.758",
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: 23091, LineEnd: 24430},
			RichNotes:       []string{"tier=context_only", "effective_impact_ms=0.000"},
			Confidence:      0.85,
		},
		{
			ID:              "eng2-cause",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_primary",
			ClaimKey:        "root_cause_primary:d_state_or_io_wait",
			Subject:         "CompThread_0-2955",
			Object:          "d_state_or_io_wait",
			Value:           "36.757",
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: 17695, LineEnd: 25862},
			RichNotes: []string{
				"tier=primary", "rank=1", "impact_ms=36.757",
				"cumulative_impact_ms=36.757", "effective_impact_ms=36.757",
				"chain_relevance=on_chain", "causality=on_wakeup_chain",
				"dominant_state=d_sleep",
			},
			Confidence: 0.8,
		},
	}
}

func eng2IdleCadenceModel(t *testing.T, idleToken string, zh bool) runtimeTraceProjTreeModel {
	t.Helper()
	projection := types.TraceCausalProjectionFromObservationRecords(eng2IdleCadenceRecords(idleToken))
	projection.WakeupPath = []string{"app-9511", ".ugc.aweme.lite-17267"}
	return buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
}

func eng2IdleCadenceFence(t *testing.T, idleToken string, zh bool) string {
	t.Helper()
	return runtimeTraceProjTreeFence(eng2IdleCadenceModel(t, idleToken, zh), zh)
}

// TestENG2IdleCadenceAnnotationSurvivesSameFactFold — the folded seat must
// SPEAK the idle reclassification: the 「其中 …帧间空闲(等待下一帧)」 tag
// renders once with the one-fact ms (the rank view and the root_evidence
// witness of the SAME segment never sum), and the teaching legend entry
// rides along (词条-图例双向).
func TestENG2IdleCadenceAnnotationSurvivesSameFactFold(t *testing.T) {
	model := eng2IdleCadenceModel(t, "pacing_idle", true)
	fence := runtimeTraceProjTreeFence(model, true)
	if got := strings.Count(fence, "其中 15.758ms 帧间空闲(等待下一帧)"); got != 1 {
		t.Fatalf("the folded seat must carry the idle-cadence annotation exactly once (one-fact MAX, never summed), got %d:\n%s", got, fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkPacingIdle) {
		t.Fatalf("the pacing legend mark must ride with the annotation (词条-图例双向)")
	}
	// EN face: raw token verbatim (D2 discipline).
	if enFence := eng2IdleCadenceFence(t, "pacing_idle", false); !strings.Contains(enFence, "of which 15.758ms pacing_idle") {
		t.Fatalf("EN folded seat must carry the raw-token annotation:\n%s", enFence)
	}
}

// TestENG2IdleCadenceAnnotationPeriodicFork — the generic periodic fork's
// annotation speaks the periodic word, never the frame promise words.
func TestENG2IdleCadenceAnnotationPeriodicFork(t *testing.T) {
	fence := eng2IdleCadenceFence(t, "periodic_idle", true)
	if !strings.Contains(fence, "其中 15.758ms 周期空闲(等待下一周期信号)") {
		t.Fatalf("the periodic fork must carry the periodic annotation:\n%s", fence)
	}
	if strings.Contains(fence, "帧间空闲") {
		t.Fatalf("the frame promise words must never render for the periodic fork:\n%s", fence)
	}
}
