package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Four CPU records contain eleven underlying occurrences. Publication
// may display the existing family cardinality, but cannot call it a span count
// or replace it with a number parsed from the model-facing member roster.
func TestB1622FamilyRecordCountActualPublication(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, caliber := range []string{tracequery.RootCauseMemberFoldCaliberSumDisjoint, tracequery.RootCauseMemberFoldCaliberIntervalUnion, tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback} {
			t.Run(lang+"/"+caliber, func(t *testing.T) {
				value := 12.0
				if caliber == tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback {
					value = 6
				}
				roster := []string{"cpu=0 count=5 6.000ms", "cpu=1 count=3 3.000ms", "cpu=2 count=2 2.000ms", "cpu=3 count=1 1.000ms"}
				result := tracequery.Result{View: "root_cause_rank", RootCauseRank: &tracequery.RootCauseRankResult{
					Target: tracequery.ThreadRef{Comm: "app", PID: 100},
					Window: tracequery.TimeWindow{StartTs: 10, EndTs: 10.1}, BoardParamsFingerprint: "b1622-params",
					Items: []tracequery.RootCauseRankItem{{
						Rank: 1, Tier: "primary", Type: "running", Thread: tracequery.ThreadRef{Comm: "worker", PID: 200},
						Source: "window_stats.thread_cpu_running", Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
						StartTs: 10.01, EndTs: 10.03, LineStart: 10, LineEnd: 40, Confidence: .9,
						ImpactMs: value, CumulativeImpactMs: value, EffectiveImpactMs: value,
						MemberCount: 4, MemberMaxMs: 6, MemberMinMs: 1, MemberSumMs: 12,
						MemberFoldCaliber: caliber, MemberRoster: roster,
					}},
				}}
				observations := traceQueryTypedObservations(result, "/captures/trace.ftrace", "/results/query.json", "/results/query.json", "", time.Unix(1, 0).UTC(), tracequery.Query{View: "root_cause_rank"})
				bus := &types.BusContext{Mutable: types.NewMutableState("B1622 record count"), Language: lang,
					AnalysisIR:  &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}, AnswerContract: types.AnswerContract{Language: lang}},
					ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: observations}},
				}
				ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
				projection := types.CompileTraceCausalProjection(ledger)
				if len(projection.OnChainCauses) != 1 {
					t.Fatalf("actual producer must retain its single chain family: %d", len(projection.OnChainCauses))
				}
				node := projection.OnChainCauses[0]
				if node.FamilyMemberCount != 4 || node.FamilyFoldCaliber != caliber || node.ImpactMS != value || node.Rank != 1 ||
					node.FamilyMemberSumMS != 12 || node.FamilyMemberMaxMS != 6 || node.FamilyMemberMinMS != 1 || !reflect.DeepEqual(node.FamilyMemberRoster, roster) {
					t.Fatalf("producer/compiler changed the original family: %+v", node)
				}
				before, err := json.Marshal(bus.ToolResults)
				if err != nil {
					t.Fatal(err)
				}
				doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model-summary", Kind: types.BlockSummary, Text: "Model-owned explanation stays unchanged. 模型原文保持。"}}}
				modelWire, err := modelOwnedAnswerBlockWire(doc)
				if err != nil {
					t.Fatal(err)
				}
				published, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Unix(2, 0))
				if err != nil || !published.Success {
					t.Fatalf("public mutation failed: %+v %v", published, err)
				}
				stored := bus.Mutable.AnswerDocumentV2()
				if err := requireModelOwnedAnswerBlockWirePreserved(modelWire, stored); err != nil {
					t.Fatal(err)
				}
				after, err := json.Marshal(bus.ToolResults)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("display must not alter source observations or numeric authority")
				}
				text := render.RenderAnswerDocument(stored, lang)
				want, forbidden := "4 records", "4 segments"
				if lang == "zh" {
					want, forbidden = "共4条记录", "共4段"
				}
				if !strings.Contains(text, want) {
					t.Errorf("public output must count records, not spans: missing %q (old span label present=%t)", want, strings.Contains(text, forbidden))
				}
				legendScope := "each may aggregate multiple physical intervals"
				if lang == "zh" {
					legendScope = "每条可汇总多个物理区间"
				}
				if !strings.Contains(text, legendScope) {
					t.Errorf("the shared legend must explain the compact record label: missing %q", legendScope)
				}
				if strings.Contains(text, forbidden) || strings.Contains(text, "共11段") || strings.Contains(text, "11 segments") {
					t.Error("family membership must not invent or import a physical span count")
				}
				if !strings.Contains(text, fmt.Sprintf("%.3fms", value)) {
					t.Error("published participation value was lost")
				}
			})
		}
	}
}

func TestB1622ReaderRecordScopeRequiresFamilyMark(t *testing.T) {
	for _, zh := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			mark runtimeTraceProjMark
			want bool
		}{
			{"sum_or_union", runtimeTraceProjMarkFamilyTotal, true},
			{"member_max", runtimeTraceProjMarkFamilyMemberMax, true},
			{"chain_intersection", runtimeTraceProjMarkFamilyChainIntersection, true},
			{"physical_span_detail", runtimeTraceProjMarkFamilySpanTop, false},
			{"count_sum", runtimeTraceProjMarkFamilyCountSum, false},
			{"unrelated_state", runtimeTraceProjMarkIconRunning, false},
		} {
			t.Run(fmt.Sprintf("zh=%t/%s", zh, tc.name), func(t *testing.T) {
				marks := &runtimeTraceProjMarkSet{}
				marks.mark(tc.mark)
				text := strings.Join(runtimeTraceProjReaderLegendLines(marks, zh, false), "\n")
				if got := strings.Contains(text, runtimeTraceProjFamilyRecordCountScope(zh)); got != tc.want {
					t.Fatalf("record population disclosure must follow only exact family marks: got %t want %t", got, tc.want)
				}
			})
		}
		if len(runtimeTraceProjReaderLegendLines(nil, zh, false)) != 0 {
			t.Fatal("missing marks must not create a count population")
		}
	}
}

func TestB1622FamilyRecordCountCaliberBoundaries(t *testing.T) {
	for _, zh := range []bool{false, true} {
		for _, count := range []int{2, 4, 17} {
			node := types.TraceCausalProjectionNode{FamilyMemberCount: count, FamilyFoldCaliber: tracequery.RootCauseMemberFoldCaliberCountSum, ImpactMS: 12, FamilyMemberSumMS: 12}
			before := node
			chainWord := runtimeTraceProjSemanticChainIntersectionWord(node, zh)
			wantChain := fmt.Sprintf("on-chain counted (%d records, same thread)", count)
			if zh {
				wantChain = fmt.Sprintf("链上计入(共%d条记录,同线程)", count)
			}
			if chainWord != wantChain || !reflect.DeepEqual(node, before) {
				t.Fatalf("chain intersection must use the same unchanged record count: %q", chainWord)
			}
			word, mark, ok := runtimeTraceProjFamilyCaliberWord(node, zh)
			want := fmt.Sprintf("count total (%d items, same thread)", count)
			if zh {
				want = fmt.Sprintf("计数合计(共%d项,同线程)", count)
			}
			if !ok || mark != runtimeTraceProjMarkFamilyCountSum || word != want {
				t.Fatalf("existing count-sum semantics changed: %q", word)
			}
			node.FamilyFoldCaliber = "future_caliber"
			if word, _, ok := runtimeTraceProjFamilyCaliberWord(node, zh); ok || word != "" {
				t.Fatal("unknown caliber must not gain a new claim")
			}
			node = before
			node.FamilyMemberCount = 1
			if runtimeTraceProjFamilyRow(node) || runtimeTraceProjFamilyValuePrefix(node, zh) != "" {
				t.Fatal("a single record must keep its existing non-family form")
			}
		}
	}
}
