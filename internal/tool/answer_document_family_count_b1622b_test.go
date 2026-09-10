package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This engine-result seed has four CPU records containing eleven physical
// occurrences. Exercise the actual observation/compiler/publication path;
// neither the roster's occurrence counts nor MergedCount may replace four.
func TestB1622BPublicFamilyCountDoesNotClaimOccurrences(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			roster := []string{"cpu=0 count=5 6.000ms", "cpu=1 count=3 3.000ms", "cpu=2 count=2 2.000ms", "cpu=3 count=1 1.000ms"}
			result := tracequery.Result{View: "root_cause_rank", RootCauseRank: &tracequery.RootCauseRankResult{
				Target: tracequery.ThreadRef{Comm: "app", PID: 100}, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 10.1}, BoardParamsFingerprint: "same-board",
				Items: []tracequery.RootCauseRankItem{{
					Rank: 1, Tier: "primary", Type: "running", Thread: tracequery.ThreadRef{Comm: "worker", PID: 200},
					Source: "window_stats.thread_cpu_running", Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
					StartTs: 10.01, EndTs: 10.03, LineStart: 10, LineEnd: 40, Confidence: .9,
					ImpactMs: 12, CumulativeImpactMs: 12, EffectiveImpactMs: 12,
					MemberCount: 4, MemberMaxMs: 6, MemberMinMs: 1, MemberSumMs: 12, MemberFoldCaliber: tracequery.RootCauseMemberFoldCaliberSumDisjoint,
					MemberRoster: roster,
				}},
			}}
			observations := traceQueryTypedObservations(result, "/captures/query.ftrace", "/results/query.json", "/results/query.json", "", time.Unix(1, 0).UTC(), tracequery.Query{View: "root_cause_rank"})
			bus := &types.BusContext{Mutable: types.NewMutableState("B1622b record-count display"), Language: lang,
				AnalysisIR:  &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}, AnswerContract: types.AnswerContract{Language: lang}},
				ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: observations}},
			}
			projection := types.CompileTraceCausalProjection(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
			if len(projection.OnChainCauses) != 1 {
				t.Fatalf("expected the original one chain family, got %d", len(projection.OnChainCauses))
			}
			node := projection.OnChainCauses[0]
			if node.FamilyMemberCount != 4 || node.MergedCount != 0 || node.ImpactMS != 12 || node.EffectiveImpactMS != 12 ||
				node.FamilyMemberMaxMS != 6 || node.FamilyMemberMinMS != 1 || node.FamilyMemberSumMS != 12 ||
				node.FamilyFoldCaliber != tracequery.RootCauseMemberFoldCaliberSumDisjoint || !reflect.DeepEqual(node.FamilyMemberRoster, roster) || node.Rank != 1 {
				t.Fatalf("source/compiler changed record cardinality, value, roster or rank: %+v", node)
			}
			before, err := json.Marshal(bus.ToolResults)
			if err != nil {
				t.Fatal(err)
			}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned explanation stays unchanged. 模型原文不动。"}}}
			wire, err := modelOwnedAnswerBlockWire(doc)
			if err != nil {
				t.Fatal(err)
			}
			pub, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Unix(2, 0))
			if err != nil || !pub.Success {
				t.Fatalf("public publication failed: %+v %v", pub, err)
			}
			stored := bus.Mutable.AnswerDocumentV2()
			if err := requireModelOwnedAnswerBlockWirePreserved(wire, stored); err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(bus.ToolResults)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("display changed source facts")
			}

			want, forbidden := "4 records", "n=4"
			if lang == "zh" {
				want, forbidden = "4条记录", "4次"
			}
			foundTree, foundTable := false, false
			for _, block := range stored.Blocks {
				if !RuntimeTraceSystemBlock(block) {
					continue
				}
				for _, line := range strings.Split(block.Text, "\n") {
					if strings.Contains(line, "└─") && strings.Contains(line, "worker-200") && strings.Contains(line, "12.000ms") {
						foundTree = true
						if !strings.Contains(line, want) || strings.Contains(line, forbidden) {
							t.Errorf("public tree/name must label the family population as records: block=%s line=%q", block.ID, line)
						}
					}
				}
				for _, item := range block.Items {
					if len(item.Cells) > 0 && strings.Contains(item.Cells[0], "worker-200") {
						foundTable = true
						if !strings.Contains(item.Cells[0], want) || strings.Contains(item.Cells[0], forbidden) {
							t.Errorf("public table must label the family population as records: block=%s cell=%q", block.ID, item.Cells[0])
						}
					}
				}
			}
			if !foundTree || !foundTable {
				t.Fatalf("public tree and table must both publish the family: tree=%t table=%t", foundTree, foundTable)
			}
			row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowChain, HasData: true}
			for _, name := range []string{runtimeTraceProjRowName(row, lang == "zh"), runtimeTraceProjRowNameKeepSuffix(row, lang == "zh")} {
				if !strings.Contains(name, want) || strings.Contains(name, forbidden) {
					t.Errorf("the chain name and its reserved grammar tail must keep the record population: %q", name)
				}
			}
		})
	}
}

// Check all five family-count consumers and both table-token branches. The
// typed population gate and unknown caliber boundary are deliberately unchanged.
func TestB1622BFamilyCountConsumersKeepTheirPopulation(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, caliber := range []string{tracequery.RootCauseMemberFoldCaliberSumDisjoint, "future_caliber"} {
			t.Run(fmt.Sprintf("zh=%t/%s", zh, caliber), func(t *testing.T) {
				node := rcm2CmpSemanticFamilyProjection().SemanticSpans[0]
				node.FamilyFoldCaliber = caliber
				before := node
				row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowSemantic, HasData: true}
				want, forbidden := "14 records", "n=14"
				if zh {
					want, forbidden = "14条记录", "14次"
				}
				semanticName, semanticValue := runtimeTraceProjSemanticCellParts(node, 7.124, zh)
				background, ok := runtimeTraceProjCompareBackgroundTopRowCell(runtimeTraceProjTreeModel{Background: []runtimeTraceProjTreeRow{row}}, zh)
				if !ok || !strings.Contains(background, "7.124ms") || !strings.Contains(semanticValue, "7.124ms") {
					t.Fatal("original positive values must still publish")
				}
				for label, text := range map[string]string{
					"row_name":          runtimeTraceProjRowName(row, zh),
					"semantic_cell":     semanticName,
					"elimination_class": runtimeTraceProjElimClassWord(row, zh, true, nil),
					"background_cell":   background,
					"table_token":       runtimeTraceProjFamilyTableToken(node, zh),
				} {
					if !strings.Contains(text, want) || strings.Contains(text, forbidden) {
						t.Errorf("%s must use the record count: %q", label, text)
					}
				}
				if caliber == "future_caliber" && runtimeTraceProjFamilyTableToken(node, zh) != want {
					t.Fatal("unknown caliber may publish record cardinality, but no aggregation claim")
				}
				if !reflect.DeepEqual(node, before) {
					t.Fatal("formatters mutated the source node")
				}
				for _, count := range []int{0, 1} {
					row.Node.FamilyMemberCount = count
					name := runtimeTraceProjRowName(row, zh)
					if runtimeTraceProjFamilyTableToken(row.Node, zh) != "" || strings.Contains(name, "条记录") || strings.Contains(name, " records") {
						t.Fatal("non-family rows must not acquire a family token")
					}
				}
			})
		}
	}
}

func TestB1622BExistingWireOccurrenceControl(t *testing.T) {
	node := elimChainNode("wire", "worker-77", "io_latency", "", 3, 5, 100)
	node.MergedCount, node.MergedMinMS, node.MergedMaxMS, node.MergedWireFold = 3, 1, 5, true
	row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowChain, HasData: true}
	if node.FamilyMemberCount != 0 || !runtimeTraceProjCauseEventFoldRow(row) {
		t.Fatal("old typed wire-fold occurrence premise must remain true")
	}
	for _, zh := range []bool{true, false} {
		want := " n=3"
		if zh {
			want = " 3次"
		}
		if got := runtimeTraceProjMergeCountChip(node.MergedCount, zh); got != want {
			t.Fatalf("ordinary occurrence chip changed: %q", got)
		}
		if got := runtimeTraceProjRowName(row, zh); !strings.Contains(got, want) {
			t.Fatalf("ordinary occurrence row lost its count: %q", got)
		}
		// Preserve the pre-existing event-fold precedence even on mixed metadata.
		mixed := row
		mixed.Node.FamilyMemberCount = 14
		if got := runtimeTraceProjRowName(mixed, zh); !strings.Contains(got, want) || strings.Contains(got, "14") {
			t.Fatalf("family wording must not supersede a real wire-fold lane: %q", got)
		}
	}
	orphan := row
	orphan.Node.MergedWireFold = false
	if runtimeTraceProjCauseEventFoldRow(orphan) {
		t.Fatal("numeric coincidence must not become an occurrence proof")
	}
}
