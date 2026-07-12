package tool

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// donghuWitnessTracePathTool mirrors the tracequery-side ENG-1 pin's witness
// path (ledger §29.42.5 gold sample); the real-pipeline pin below skips
// wherever the file is absent.
const donghuWitnessTracePathTool = "/Users/han/opt/donghu/donghu.ftrace"

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
				// Engine-actual shape (traceQueryTypedPriorityRichNotes):
				// rank rows always carry their typed type= note — the R1
				// survivor adopts it as TypeToken (fixture 取引擎实铸形).
				"tier=context_only", "type=" + idleToken, "impact_ms=15.758",
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

// TestENG2IdleCadenceIndependentRow — CAL-1 件⑤ PACE-ROW (§29.47.4②,
// 2026-07-12; EVOLUTION RECORD: supersedes the ENG-2 「其中 …」 annotation
// pin — the annotation arm is now the fold FALLBACK): the same-fact-merged
// idle segment stands as its OWN self row — row 1 speaks 帧间空闲(等待下一帧)
// behind the dedicated cadence glyph, 行2 mints the typed
// 节拍吻合·上下文(不参与根因排序) word, the 「其中 …」 tag never
// double-speaks, and the teaching legend marks ride along (词条-图例双向).
func TestENG2IdleCadenceIndependentRow(t *testing.T) {
	model := eng2IdleCadenceModel(t, "pacing_idle", true)
	fence := runtimeTraceProjTreeFence(model, true)
	if got := strings.Count(fence, "自身·帧间空闲(等待下一帧) 15.758ms"); got != 1 {
		t.Fatalf("the idle segment must stand as ONE independent self row, got %d:\n%s", got, fence)
	}
	if !strings.Contains(fence, tracefence.GlyphPacing+" 自身·帧间空闲(等待下一帧)") {
		t.Fatalf("the independent idle row must lead with the dedicated cadence glyph:\n%s", fence)
	}
	if !strings.Contains(fence, "节拍吻合·上下文(不参与根因排序)") {
		t.Fatalf("the idle row must carry the typed cadence-fit context word:\n%s", fence)
	}
	if strings.Contains(fence, "其中 15.758ms 帧间空闲") {
		t.Fatalf("the annotation fallback must not double-speak on an independent idle row:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkPacingIdle) || !model.Marks.has(runtimeTraceProjMarkIconPacing) {
		t.Fatalf("the pacing type-word and cadence-glyph legend marks must ride the row (词条-图例双向)")
	}
	// EN face: raw token verbatim (D2 discipline) + the EN cadence-fit word.
	enFence := eng2IdleCadenceFence(t, "pacing_idle", false)
	if !strings.Contains(enFence, "own·pacing_idle 15.758ms") ||
		!strings.Contains(enFence, "cadence fit · context (not ranked)") {
		t.Fatalf("EN independent idle row must speak the raw token + cadence-fit word:\n%s", enFence)
	}
}

// TestENG2IdleCadenceAnnotationPeriodicFork — the generic periodic fork's
// independent row speaks the periodic word, never the frame promise words
// (CAL-1 件⑤: same PACE-ROW form as the pacing lane).
func TestENG2IdleCadenceAnnotationPeriodicFork(t *testing.T) {
	fence := eng2IdleCadenceFence(t, "periodic_idle", true)
	if !strings.Contains(fence, "自身·周期空闲(等待下一周期信号) 15.758ms") {
		t.Fatalf("the periodic fork must stand as its independent row:\n%s", fence)
	}
	if !strings.Contains(fence, "节拍吻合·上下文(不参与根因排序)") {
		t.Fatalf("the periodic row must carry the typed cadence-fit context word:\n%s", fence)
	}
	if strings.Contains(fence, "帧间空闲") {
		t.Fatalf("the frame promise words must never render for the periodic fork:\n%s", fence)
	}
}

// TestENG2IdleCadenceDonghuRealPipelineFamilyFold — ENG-2 追修 pin (复核冷读
// CP1-③ 第三折叠机, 2026-07-12): the ENGINE-REAL donghu pipeline (BuildIndex
// → root_cause_rank → typed observations → projection compile → tree render)
// must carry the 15.758ms frame-pacing reclassification onto the rendered ×N
// self-sleep family seat. Pre-fix, the idle rows published under raw
// sleep/wakeup lines (23091-24430) while the segment's impact record
// published its evidence span, so the same-fact fold never fired and the
// standalone idle rows fell off the capped supporting-hop bucket — the whole
// report carried ZERO 帧间空闲 wording (84618 replay witness). Skip-gated on
// the witness trace like the ENG-1 pin.
func TestENG2IdleCadenceDonghuRealPipelineFamilyFold(t *testing.T) {
	if _, err := os.Stat(donghuWitnessTracePathTool); err != nil {
		t.Skipf("witness trace unavailable: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), donghuWitnessTracePathTool)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898})
	records := traceQueryTypedObservations(result, "donghu.ftrace", "", "", "", time.Now())
	projection := types.TraceCausalProjectionFromObservationRecords(records)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// CAL-1 件⑤ PACE-ROW (EVOLUTION RECORD, 2026-07-12 — inverts the ENG-2
	// fold pin): the 15.758ms frame-pacing segment stands as its OWN self row
	// (one seat, never double-published), OUTSIDE the ×N sleep family; the
	// 「其中 …」 annotation no longer renders on the family seat (it is the
	// fold fallback now, and the member left the fold).
	if got := strings.Count(fence, "自身·帧间空闲(等待下一帧) 15.758ms"); got != 1 {
		t.Fatalf("the donghu frame-pacing segment must stand as ONE independent self row, got %d:\n%s", got, fence)
	}
	if !strings.Contains(fence, "节拍吻合·上下文(不参与根因排序)") {
		t.Fatalf("the donghu pacing row must carry the typed cadence-fit context word:\n%s", fence)
	}
	if strings.Contains(fence, "其中 15.758ms 帧间空闲") {
		t.Fatalf("the family-seat annotation must not double-speak after the member left the fold:\n%s", fence)
	}
	// The remaining sleep family must not swallow the pacing member back:
	// its ×N range top stays BELOW the pacing segment length.
	if strings.Contains(fence, "–15.758ms)") {
		t.Fatalf("the ×N sleep family range must exclude the pacing member:\n%s", fence)
	}
}

// TestENG2IdleCadenceSurvivorAdoptedTypeToken — P2-4 synthetic pin (no
// witness file): a same-fact SURVIVOR that kept its scheduler-state word
// (Object=s_sleep) while ADOPTING the folded idle view's TypeToken
// (pacing_idle) must still speak the annotation — the redundancy exclusion
// keys on the canonical Object ONLY (the ×8 donghu seat lost its wording to
// a TypeToken-based exclusion pre-fix). A standalone idle row (Object IS
// the idle lane) stays excluded: its display word already says it.
func TestENG2IdleCadenceSurvivorAdoptedTypeToken(t *testing.T) {
	survivor := types.TraceCausalProjectionNode{
		Object:          "s_sleep",
		TypeToken:       "pacing_idle",
		IdleCadenceMS:   15.758,
		IdleCadenceKind: "pacing_idle",
	}
	tag, mark, ok := runtimeTraceProjIdleCadenceTag(survivor, true)
	if !ok || !strings.Contains(tag, "帧间空闲") {
		t.Fatalf("the survivor-adopted-TypeToken shape must carry the idle annotation, got ok=%v tag=%q", ok, tag)
	}
	if mark != runtimeTraceProjMarkPacingIdle {
		t.Fatalf("the pacing legend mark must ride the annotation, got %v", mark)
	}
	standalone := survivor
	standalone.Object = "pacing_idle"
	if _, _, ok := runtimeTraceProjIdleCadenceTag(standalone, true); ok {
		t.Fatalf("a standalone idle row must not double-speak the annotation")
	}
}
