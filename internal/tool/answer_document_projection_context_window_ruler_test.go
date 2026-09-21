package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Public queries retain both the requested window and an exploratory expanded
// window. A background state's 20ms in the latter is not 100% of the former.
func TestContextWindowRulerPublicExpandedQuery(t *testing.T) {
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_background_demotion.case")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("missing trace fixture")
	}
	body, _, ok = strings.Cut(body, "\n'\n")
	if !ok {
		t.Fatal("unterminated trace fixture")
	}
	for _, queryWindow := range []struct {
		name         string
		start, end   float64
		percent, bar string
		idle         bool
	}{
		{"expanded", 2, 2.025, "80%", "▒▒▒▒▒▒▒▒░░", false},
		{"same_length_shifted", 2.0005, 2.0205, "100%", "▒▒▒▒▒▒▒▒▒▒", true},
	} {
		t.Run(queryWindow.name, func(t *testing.T) {
			for _, renamed := range []bool{false, true} {
				t.Run(fmt.Sprintf("renamed=%t", renamed), func(t *testing.T) {
					trace, subject := body, "logger-900"
					if renamed {
						trace = strings.NewReplacer("logger", "recorder", "app", "client", "cookie", "session", "network", "transport", "threadpool", "executor").Replace(trace)
						subject = "recorder-900"
					}
					path := hmc081WriteTrace(t, trace)
					bus, _, _ := hmc17NamedPathContext(t)
					var records []types.ObservationRecord
					var results []types.ToolResult
					for _, q := range []struct {
						view string
						end  float64
					}{
						{"wakeup_chain", 2.020}, {"window_stats", 2.020},
						{"root_cause_rank", 2.025}, {"critical_blocking_calls", 2.020},
					} {
						start := 2.0
						if q.view == "root_cause_rank" {
							start = queryWindow.start
							q.end = queryWindow.end
						}
						result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": q.view, "pid": 100, "time_start": start, "time_end": q.end, "trace_flavor": "harmony_hitrace"})
						if !result.Success {
							t.Fatalf("public %s: %s", q.view, result.Summary)
						}
						records = append(records, result.Observations...)
						results = append(results, result)
					}
					before, _ := json.Marshal(results)
					for _, reversed := range []bool{false, true} {
						ordered := append([]types.ToolResult(nil), results...)
						if reversed {
							for a, b := 0, len(ordered)-1; a < b; a, b = a+1, b-1 {
								ordered[a], ordered[b] = ordered[b], ordered[a]
							}
						}
						for _, lang := range []string{"zh", "en"} {
							md := contextWindowRulerRender(t, ordered, lang, renamed)
							_, fence, hasFence := strings.Cut(md, "```text trace-causal-projection\n")
							if !hasFence {
								t.Fatalf("actual trace tree missing: %s", md)
							}
							fence, _, _ = strings.Cut(fence, "```")
							want := fmt.Sprintf("本行满格/占比基于查询窗 %.6f~%.6fs", queryWindow.start, queryWindow.end)
							if lang == "en" {
								want = fmt.Sprintf("row bar/share base: query window %.6f~%.6fs", queryWindow.start, queryWindow.end)
							}
							found := false
							lines := strings.Split(fence, "\n")
							for i, line := range lines {
								if strings.Contains(line, subject) && strings.Contains(line, "20.000ms") && !strings.HasPrefix(line, "|") {
									found = true
									if !strings.Contains(line, queryWindow.percent) || !strings.Contains(line, queryWindow.bar) {
										t.Errorf("logger must keep 20ms and exact own-query bar/%% (%s): %s", lang, line)
									}
									stanza := line
									for j := i + 1; j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "·"); j++ {
										stanza += "\n" + lines[j]
									}
									if !strings.Contains(stanza, want) {
										t.Errorf("ruler not attached to same native logger stanza (%s): %s", lang, stanza)
									}
								}
							}
							if !found {
								t.Fatalf("native 20ms background row missing (%s):\n%s", lang, md)
							}
							detailFound := false
							for _, block := range strings.Split(md, "**[") {
								heading, _, _ := strings.Cut(block, "\n")
								if strings.Contains(heading, subject) && strings.Contains(block, want) {
									detailFound = true
									idle := strings.Contains(block, "整窗等待") || strings.Contains(block, "whole-window wait")
									if idle != queryWindow.idle {
										t.Errorf("whole-window annotation must use same source ruler (%s): %s", lang, block)
									}
								}
							}
							if !detailFound {
								t.Errorf("own-query detail disclosure missing (%s): %q", lang, want)
							}
							if !strings.Contains(md, "19.500ms") {
								t.Errorf("request-window native 19.5ms disappeared (%s)", lang)
							}
						}
					}
					after, _ := json.Marshal(results)
					if string(before) != string(after) {
						t.Fatal("render rewrote native measurements or identities")
					}
				})
			}
		})
	}
}

func contextWindowRulerRender(t *testing.T, results []types.ToolResult, lang string, renamed bool) string {
	t.Helper()
	bus := newBusForMutationTest()
	start, end := 2.0, 2.020
	subject := "app-100"
	if renamed {
		subject = "client-100"
	}
	rm := types.RequestModel{Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "2.000s to 2.020s"},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: subject, Source: "user_explicit"}}}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}}
	bus.Mutable.SetRequestModel(rm)
	bus.ToolResults = results
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	projection := types.CompileTraceCausalProjection(ledger)
	if projection.WindowStartTs != start || projection.WindowEndTs != end {
		t.Fatalf("explicit principal window not retained: %.6f..%.6f", projection.WindowStartTs, projection.WindowEndTs)
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "Query ruler regression."}}}
	result, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Unix(1753000000, 0).UTC())
	if err != nil || !result.Success {
		t.Fatalf("public mutation: %v %s", err, result.Summary)
	}
	return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), lang)
}

func TestContextWindowRulerTypedBoundaries(t *testing.T) {
	base := types.TraceCausalProjectionNode{Subject: "worker-900", StateKind: "d_state", ImpactMS: 20,
		StartTs: 2.0005, EndTs: 2.0205, ActualImpactMS: 20,
		QueryWindowStartTs: 2, QueryWindowEndTs: 2.025}
	for _, tc := range []struct {
		name    string
		mutate  func(*types.TraceCausalProjectionNode)
		percent string
		idle    bool
	}{
		{"source_not_actual_extent", func(n *types.TraceCausalProjectionNode) {}, "80%", false},
		{"same_length_different_position", func(n *types.TraceCausalProjectionNode) { n.QueryWindowStartTs = 3; n.QueryWindowEndTs = 3.020 }, "100%", true},
		{"same_window", func(n *types.TraceCausalProjectionNode) { n.QueryWindowEndTs = 2.020 }, "100%", true},
		{"zero_start", func(n *types.TraceCausalProjectionNode) { n.QueryWindowStartTs = 0; n.QueryWindowEndTs = .025 }, "80%", false},
		{"missing_query", func(n *types.TraceCausalProjectionNode) { n.QueryWindowStartTs = 0; n.QueryWindowEndTs = 0 }, "", false},
		{"rank_window_is_not_value_window", func(n *types.TraceCausalProjectionNode) {
			n.QueryWindowStartTs = 0
			n.QueryWindowEndTs = 0
			n.RankQueryWindowStartTs = 2
			n.RankQueryWindowEndTs = 2.020
		}, "", false},
		{"multi_window", func(n *types.TraceCausalProjectionNode) {
			n.MergedCount = 2
			n.MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{{StartTs: 2, EndTs: 2.025}, {StartTs: 3, EndTs: 3.020}}
		}, "", false},
		{"known_roster_does_not_restore_missing_scalar", func(n *types.TraceCausalProjectionNode) {
			n.QueryWindowStartTs = 0
			n.QueryWindowEndTs = 0
			n.MergedCount = 2
			n.MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{{StartTs: 2, EndTs: 2.020}}
		}, "", false},
		{"actual_fallback_not_projection", func(n *types.TraceCausalProjectionNode) { n.ImpactMS = 0; n.QueryWindowEndTs = 2.020 }, "", false},
		{"infinite_window", func(n *types.TraceCausalProjectionNode) { n.QueryWindowEndTs = math.Inf(1) }, "", false},
		{"nan_window", func(n *types.TraceCausalProjectionNode) { n.QueryWindowStartTs = math.NaN() }, "", false},
	} {
		for _, kind := range []string{runtimeTraceProjTreeRowBackground, runtimeTraceProjTreeRowAdjacent} {
			for _, zh := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/zh=%t", tc.name, kind, zh), func(t *testing.T) {
					node := base
					tc.mutate(&node)
					row := runtimeTraceProjTreeRow{Node: node, Kind: kind, HasData: true, EvidenceTag: "E9"}
					metric, tags := runtimeTraceProjRowMetricParts(row, 20, true, zh)
					if !strings.Contains(metric, "20.000ms") {
						t.Fatalf("lost original value: %s", metric)
					}
					if tc.percent == "" {
						if strings.Contains(metric, "%") || !strings.HasPrefix(metric, strings.Repeat(" ", runtimeTraceProjTreeBarWidth)) {
							t.Errorf("unknown/nonprojection ruler borrowed anchor: %q", metric)
						}
					} else if !strings.Contains(metric, tc.percent) {
						t.Errorf("own query percentage want %s: %s", tc.percent, metric)
					}
					var text string
					for _, tag := range tags {
						text += " " + tag.Text
					}
					if tc.percent != "" {
						want := fmt.Sprintf("本行满格/占比基于查询窗 %.6f~%.6fs", node.QueryWindowStartTs, node.QueryWindowEndTs)
						if !zh {
							want = fmt.Sprintf("row bar/share base: query window %.6f~%.6fs", node.QueryWindowStartTs, node.QueryWindowEndTs)
						}
						if !strings.Contains(text, want) {
							t.Errorf("source query coordinate/meaning not attached: %s", text)
						}
					}
					idle := strings.Contains(text, "整窗等待") || strings.Contains(text, "whole-window wait")
					if idle != (tc.idle && kind == runtimeTraceProjTreeRowBackground) {
						t.Errorf("whole-window annotation=%t want=%t: %s", idle, tc.idle && kind == runtimeTraceProjTreeRowBackground, text)
					}
				})
			}
		}
	}
}

func TestContextWindowRulerCaliberAndNoWindowMode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*types.TraceCausalProjectionNode)
		window bool
	}{
		{"cross_thread", func(n *types.TraceCausalProjectionNode) {
			n.SubjectKind = types.TraceCausalSubjectKindAggregateMetric
			n.TypeToken = "supply_pressure"
		}, true},
		{"composite", func(n *types.TraceCausalProjectionNode) { n.Unit = types.TraceObservationUnitCompositeScore }, true},
		{"no_window_mode", func(n *types.TraceCausalProjectionNode) {}, false},
	} {
		for _, zh := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/zh=%t", tc.name, zh), func(t *testing.T) {
				node := types.TraceCausalProjectionNode{Subject: "worker-900", StateKind: "sleep", ImpactMS: 20, QueryWindowStartTs: 2, QueryWindowEndTs: 2.020}
				tc.mutate(&node)
				marks := &runtimeTraceProjMarkSet{}
				row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowBackground, HasData: true, EvidenceTag: "E9", marks: marks}
				before, _ := json.Marshal(row)
				metric, tags := runtimeTraceProjRowMetricParts(row, 20, tc.window, zh)
				wantBar := strings.Repeat(" ", runtimeTraceProjTreeBarWidth)
				if !tc.window {
					wantBar = "▒▒▒▒▒▒▒▒▒▒"
				}
				if !strings.HasPrefix(metric, wantBar) || strings.Contains(metric, "%") {
					t.Errorf("non-wallclock/no-window relative ruler mismatch: %q", metric)
				}
				var text string
				for _, tag := range tags {
					text += " " + tag.Text
				}
				if !tc.window && (strings.Contains(text, "本行满格/占比") || strings.Contains(text, "row bar/share base")) {
					t.Errorf("relative-max fallback invented query ruler: %s", text)
				}
				if !tc.window && !marks.has(runtimeTraceProjMarkBarScaleFallback) {
					t.Error("relative bar lost matching fallback legend mark")
				}
				model := runtimeTraceProjTreeModel{Background: []runtimeTraceProjTreeRow{row}}
				if tc.window {
					model.WindowMS = 20
				}
				text += "\n" + runtimeTraceProjDetailFullText(model, zh)
				if strings.Contains(text, "整窗等待") || strings.Contains(text, "whole-window wait") {
					t.Errorf("no wallclock query projection nevertheless acquired whole-window label: %s", text)
				}
				after, _ := json.Marshal(row)
				if string(before) != string(after) {
					t.Fatal("display rewrote node")
				}
			})
		}
	}
}
