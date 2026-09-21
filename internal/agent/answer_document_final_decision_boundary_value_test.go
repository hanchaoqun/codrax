package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

func readerValueCompositionFixture() types.TraceCausalProjectionSet {
	var nodes []types.TraceCausalProjectionNode
	for i, sleep := range []float64{17, 14} {
		nodes = append(nodes, types.TraceCausalProjectionNode{
			EvidenceID: fmt.Sprintf("rank-%d", i+1), Subject: fmt.Sprintf("worker-%d", i+1),
			TypeToken: "priority_inversion_candidate", StateKind: "s_sleep", Rank: i + 1,
			ImpactMS: 1, CumulativeImpactMS: 1, EffectiveImpactMS: 1, EffectiveImpactPublished: true,
			GatedRunnableMS: 1, SleepMS: sleep, RunnableMS: 1, RunningMS: 1,
			ChainRelevance: "on_chain", RankQueryWindowStartTs: 2, RankQueryWindowEndTs: 2.02,
		})
	}
	return types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		ArtifactLabel: "capture.systrace", WindowStartTs: 2, WindowEndTs: 2.02,
		RankedSeats: nodes, OnChainCauses: nodes,
	}}}
}

func assertReaderValueComposition(t *testing.T, got, lang string, nodes []types.TraceCausalProjectionNode) {
	t.Helper()
	for _, node := range nodes {
		prefix := fmt.Sprintf("  - 第 %d 位，%s：", node.Rank, node.Subject)
		raw := fmt.Sprintf("%s：睡眠等待，已测 %.3f 毫秒", node.Subject, node.SleepMS)
		account := fmt.Sprintf("另账：睡眠等待，实测占用 %.3f 毫秒", node.SleepMS)
		composition := "组成：就绪等待全额 1.000 ms + 运行供给折算缺口 0.000 ms"
		if lang == "en" {
			prefix = fmt.Sprintf("  - Rank %d, %s:", node.Rank, node.Subject)
			raw = fmt.Sprintf("%s: sleep wait, measured %.3f ms", node.Subject, node.SleepMS)
			account = fmt.Sprintf("separate measured state account (sleep wait): %.3f ms", node.SleepMS)
			composition = "composition: runnable wait counted in full 1.000 ms + folded running-supply deficit 0.000 ms"
		}
		if !strings.Contains(got, raw) {
			t.Errorf("original sleep account disappeared: want %q\n%s", raw, got)
		}
		_, tail, found := strings.Cut(got, prefix)
		if !found {
			t.Fatalf("reader rank row missing %q:\n%s", prefix, got)
		}
		row, _, _ := strings.Cut(tail, "\n")
		for _, want := range []string{account, composition} {
			if !strings.Contains(row, want) {
				t.Errorf("rank %d lost its own value meaning %q:\n%s", node.Rank, want, row)
			}
		}
		if strings.Contains(row, "对应已测") || strings.Contains(row, "35.000") {
			t.Errorf("rank row implies a conversion or borrows another account: %s", row)
		}
	}
}

func TestTraceReaderValueCompositionRetainsEachCandidateState(t *testing.T) {
	set := readerValueCompositionFixture()
	before, _ := json.Marshal(set)
	finding, err := tracefinding.CompileCandidateContract(types.ObservationLedger{}, set, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil || len(finding.Candidates) != 2 {
		t.Fatalf("candidate fixture: %v %+v", err, finding)
	}
	for _, lang := range []string{"zh", "en"} {
		got := renderTraceFinalReaderDecisionCards(set, nil, lang, nil, types.TraceWakeupTargetCPUIntegrity{}, false)
		assertReaderValueComposition(t, got, lang, set.Projections[0].RankedSeats)
		if lang == "zh" {
			for _, candidate := range finding.Candidates {
				if !strings.Contains(got, tracefinding.RootCauseValueDescription(candidate.Decision)) {
					t.Error("reader must reuse the selector/sidecar's existing precise descriptor")
				}
			}
		}
	}
	after, _ := json.Marshal(set)
	if string(before) != string(after) {
		t.Fatal("reader changed original type/state/value facts")
	}
}

func TestTraceReaderValueCompositionBuildInitialInstruction(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := answerDocCausalCeilingTestContext(false)
			ctx.Language, ctx.AnalysisIR.RequestModel.Language = lang, lang
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FrameCausalityRequested = false
			var records []types.ObservationRecord
			nodes := readerValueCompositionFixture().Projections[0].RankedSeats
			for _, node := range nodes {
				records = append(records, types.ObservationRecord{
					ID: node.EvidenceID, Origin: types.AnswerEvidenceOriginRuntimeArtifact,
					Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
					SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact,
						ArtifactID: "capture.systrace", ArtifactKind: "trace", QueryScopeID: "same-query"},
					Span:      types.ObservationSpan{StartTs: 2, EndTs: 2.02, LineStart: node.Rank, LineEnd: node.Rank + 1},
					Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:" + node.Subject,
					Subject: node.Subject, Object: node.TypeToken, Value: "1.000", Unit: "ms",
					RichNotes: []string{
						fmt.Sprintf("rank=%d", node.Rank), "tier=primary", "chain_relevance=on_chain",
						"type=priority_inversion_candidate", "dominant_state=s_sleep", "impact_ms=1.000",
						"effective_impact_ms=1.000", "gated_runnable=1.000", "gated_running_deficit=0.000",
						fmt.Sprintf("sleep=%.3f", node.SleepMS), "runnable=1.000", "running=1.000",
						"selected_window=2.000000..2.020000", "rank_board_target=app-100",
						"rank_board_params=same-board", "fix_direction=scheduling_priority",
					},
				})
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
				ToolName: "trace_query", Success: true, Observations: records,
				TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "root_cause_rank"},
			}}})
			before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			marker := "## 面向读者的 Trace 成文事实卡"
			if lang == "en" {
				marker = "## Reader-ready Trace facts"
			}
			_, card, found := strings.Cut(prompt, marker)
			if !found {
				t.Fatalf("production finalizer prompt has no reader card: %s", prompt)
			}
			assertReaderValueComposition(t, card, lang, nodes)
			if strings.LastIndex(prompt, marker) < strings.LastIndex(prompt, "## Submission Checklist") {
				t.Error("row-local value meaning must reach the high-salience final reader card")
			}
			after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
			if string(before) != string(after) {
				t.Fatal("display changed the original observations")
			}
		})
	}
}
