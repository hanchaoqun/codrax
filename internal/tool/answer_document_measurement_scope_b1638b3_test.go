package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This test does not elect an unrestricted query over a filtered query. The
// current account winner may stay unchanged; its wait denominator must not
// borrow the other result's population, and both measured populations remain.
func TestB1638B3ActualDonghuWaitMeasurementScopes(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	run := func(filtered bool) (types.ToolResult, string) {
		params := map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "thread": "CompThread_0-2955", "time_start": 13762.791708, "time_end": 13763.024898}
		if filtered {
			params["line_start"], params["line_end"] = 1, 400
		}
		raw, _ := json.Marshal(params)
		result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, raw)
		if err != nil || !result.Success {
			t.Fatalf("actual query failed: %v %s", err, result.Summary)
		}
		count, total, scope := 0, 0.0, ""
		for _, record := range result.Observations {
			if record.Subject != "CompThread_0-2955" {
				continue
			}
			if record.Predicate == "target_window_states" {
				scope = record.SourceRef.QueryScopeID
			}
			if record.Predicate == "wakeup_causal_impact" && record.Object == "s_sleep" {
				var value float64
				if _, err := fmt.Sscan(record.Value, &value); err != nil || record.MeasurementSources == nil {
					t.Fatal("native source/value premise missing")
				}
				count++
				total += value
			}
		}
		wantCount, wantTotal := 8, 78.630
		if filtered {
			wantCount, wantTotal = 1, 230.286
		}
		if count != wantCount || math.Abs(total-wantTotal) > .0005 || scope == "" {
			t.Fatalf("actual fixture changed: filtered=%t count=%d S=%.3f scope=%q", filtered, count, total, scope)
		}
		return result, scope
	}
	main, mainScope := run(false)
	filtered, filteredScope := run(true)
	if mainScope == filteredScope {
		t.Fatal("two actual queries must retain their own result identities")
	}
	for _, tc := range []struct {
		name    string
		results []types.ToolResult
		mixed   bool
	}{
		{"main_only", []types.ToolResult{main}, false},
		{"same_result_repeat", []types.ToolResult{main, main}, false},
		{"main_then_filtered", []types.ToolResult{main, filtered}, true},
		{"filtered_then_main", []types.ToolResult{filtered, main}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := json.Marshal(tc.results)
			start, end := 13762.791708, 13763.024898
			request := "分析 CompThread_0-2955 在 13762.791708..13763.024898 的等待"
			rm := types.RequestModel{Scenario: types.ScenarioRootCause, Intent: types.IntentRootCause,
				RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, FrameCausalityRequested: false, Confidence: .9},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: request},
				RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 2955, Thread: "CompThread_0-2955", Source: "user_explicit"}}}
			mutable := types.NewMutableState(request)
			mutable.SetRequestModel(rm)
			for _, result := range tc.results {
				mutable.AppendDispatchToolResult(result)
			}
			bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "zh", Mutable: mutable, AnalysisIR: &types.AnalysisIR{RequestModel: rm}, ToolResults: tc.results}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
			projection := types.CompileTraceCausalProjection(ledger)
			if projection.TargetStateAccount == nil || len(projection.TargetStateAccount.MeasurementOrigins) == 0 {
				t.Fatal("actual selected account/source missing")
			}
			selectedScope := projection.TargetStateAccount.MeasurementOrigins[0].SourceRef.QueryScopeID
			if selectedScope != mainScope && selectedScope != filteredScope {
				t.Fatal("selected account is not either actual result")
			}
			byScope, seen := map[string]float64{}, map[string]bool{}
			for _, bucket := range [][]types.TraceCausalProjectionNode{projection.PrimaryRootCauses, projection.OnChainCauses, projection.AdjacentCauses, projection.BackgroundCauses, projection.SupportingHops} {
				for _, node := range bucket {
					if seen[node.EvidenceID] || node.Subject != "CompThread_0-2955" || node.Predicate != "wakeup_causal_impact" || node.Object != "s_sleep" {
						continue
					}
					seen[node.EvidenceID] = true
					parents := b1638b3OriginResultScopes(node.MeasurementOrigins)
					if len(parents) != 1 {
						t.Errorf("S occurrence merge mixed/erased result populations: value=%.3f scopes=%v", node.ImpactMS, parents)
					}
					for scope := range parents {
						byScope[scope] += node.ImpactMS
					}
				}
			}
			if math.Abs(byScope[mainScope]-78.630) > .0005 {
				t.Errorf("main eight local S windows lost their own 78.630ms sum: %.3f", byScope[mainScope])
			}
			if tc.mixed && math.Abs(byScope[filteredScope]-230.286) > .0005 {
				t.Errorf("filtered S must remain an independent 230.286ms fact: %.3f", byScope[filteredScope])
			}
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
			wait, admitted, _, _ := runtimeTraceProjTargetSymptomAdmission(model)
			if wait <= 0 {
				t.Error("do not fix mixing by removing the actual positive wait denominator")
			}
			if !tc.mixed && math.Abs(wait-116.963) > .0005 {
				t.Errorf("main-only wait ruler changed: %.3f, want 116.963", wait)
			}
			for i, included := range admitted {
				if !included {
					continue
				}
				for scope := range b1638b3OriginResultScopes(model.SelfRows[i].Node.MeasurementOrigins) {
					if scope != selectedScope {
						t.Errorf("wait denominator %.3f borrowed a foreign query: selected=%s row=%s", wait, selectedScope, scope)
					}
				}
			}
			t.Logf("selected_query=%s wait=%.3f mainS=%.3f filteredS=%.3f", selectedScope, wait, byScope[mainScope], byScope[filteredScope])
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "原模型结论保持。"}}}
			modelWire, err := modelOwnedAnswerBlockWire(doc)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ApplyAndPersistMutation(bus, "measurement-scope", types.NewReplaceAllMutation(doc), nil, time.Now())
			if err != nil || !result.Success {
				t.Fatalf("public mutation failed: %v %+v", err, result)
			}
			stored := mutable.AnswerDocumentV2()
			if err := requireModelOwnedAnswerBlockWirePreserved(modelWire, stored); err != nil {
				t.Fatal(err)
			}
			wire, _ := json.Marshal(stored)
			if strings.Contains(string(wire), "308.916ms") || strings.Contains(string(wire), "347.249ms") {
				t.Error("public answer still emits the cross-query sleep/wait sum")
			}
			if tc.mixed {
				for _, value := range []string{"78.630ms", "230.286ms"} {
					if !strings.Contains(string(wire), value) {
						t.Errorf("public answer lost independent measured fact %s", value)
					}
				}
			}
			after, _ := json.Marshal(tc.results)
			if string(before) != string(after) {
				t.Fatal("publication changed the original query results")
			}
		})
	}
}

func b1638b3OriginResultScopes(origins []types.TraceSchedulerMeasurementOrigin) map[string]bool {
	out := map[string]bool{}
	for _, origin := range origins {
		if origin.SourceRef.QueryScopeID != "" {
			out[origin.SourceRef.QueryScopeID] = true
		}
	}
	return out
}

func b1638b3TestOrigins(scope string) []types.TraceSchedulerMeasurementOrigin {
	if scope == "legacy" {
		return nil
	}
	domain := types.TraceSchedulerMeasurementDomain{Version: 1, Status: "constructed_partition", Method: "thread_timeline", TargetTID: 55,
		WindowStartTs: 10, WindowEndTs: 10.1, PartitionID: "partition:" + scope}
	if scope == "filtered" {
		domain.QueryLineStart, domain.QueryLineEnd = 1, 400
	}
	sources := types.TraceSchedulerMeasurementSourcesFromDomain(&domain)
	if scope == "mixed" {
		sources = types.MergeTraceSchedulerMeasurementSources(sources, nil)
	}
	return []types.TraceSchedulerMeasurementOrigin{{SourceRef: types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/a.ftrace", PayloadRef: "/results/" + scope + ".json", QueryScopeID: "query:" + scope},
		ObservedAt: "2026-09-10T00:00:00Z", MeasurementSources: sources}}
}

func TestB1638B3TwoOccurrenceRestrictionBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, mainScope, extraScope string
		merge                       bool
	}{
		{"legacy", "legacy", "legacy", true},
		{"local_windows_and_results", "main", "main", true},
		{"different_line_filter", "main", "filtered", false},
		{"legacy_cannot_borrow_known", "legacy", "main", false},
		{"mixed_cannot_borrow_known", "mixed", "main", false},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", tc.name, reverse), func(t *testing.T) {
				main := types.TraceCausalProjectionNode{EvidenceID: "first", Subject: "worker-55", Role: types.TraceCausalRoleCausalHop,
					Predicate: "wakeup_causal_impact", Object: "s_sleep", StateKind: "s_sleep", ImpactMS: 10,
					StartTs: 10, EndTs: 10.01, QueryWindowStartTs: 10, QueryWindowEndTs: 10.1,
					MeasurementOrigins: b1638b3TestOrigins(tc.mainScope)}
				extra := main
				extra.EvidenceID, extra.ImpactMS, extra.StartTs, extra.EndTs = "second", 20, 10.03, 10.05
				extra.MeasurementOrigins = b1638b3TestOrigins(tc.extraScope)
				if tc.name == "local_windows_and_results" {
					extra.MeasurementOrigins[0].SourceRef.QueryScopeID = "another-view"
					extra.MeasurementOrigins[0].MeasurementSources.Domains[0].WindowStartTs = 10.03
					extra.MeasurementOrigins[0].MeasurementSources.Domains[0].PartitionID = "another-local-partition"
				}
				if reverse {
					main, extra = extra, main
				}
				before, _ := json.Marshal([]types.TraceCausalProjectionNode{main, extra})
				merged, kept := runtimeTraceProjTrunkFoldSameStateOccurrences(main, []types.TraceCausalProjectionNode{extra})
				if tc.merge {
					if len(kept) != 0 || merged.MergedCount != 2 || math.Abs(merged.ImpactMS-30) > .0005 {
						t.Fatalf("same restriction must retain old two-occurrence arithmetic: %+v kept=%d", merged, len(kept))
					}
				} else if len(kept) != 1 || !reflect.DeepEqual(merged, main) || !reflect.DeepEqual(kept[0], extra) {
					t.Fatal("incompatible/unknown measurement must remain two unchanged rows")
				}
				after, _ := json.Marshal([]types.TraceCausalProjectionNode{main, extra})
				if string(before) != string(after) {
					t.Fatal("two-row fold mutated native origins")
				}
			})
		}
	}
}

func TestB1638B3WaitAdmissionRestrictionMatrix(t *testing.T) {
	type rowSpec struct {
		scope, state string
		ms           float64
		hop          bool
	}
	for _, tc := range []struct {
		name, account string
		rows          []rowSpec
		wait          float64
		limited       bool
	}{
		{"pure_legacy", "legacy", []rowSpec{{"legacy", "runnable", 5, false}, {"legacy", "s_sleep", 10, false}}, 15, false},
		{"known_own_states", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, false}}, 15, false},
		{"known_own_hop_not_larger_foreign", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, true}, {"filtered", "s_sleep", 50, true}}, 15, true},
		{"foreign_smaller_max_loser", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, true}, {"filtered", "s_sleep", 4, true}}, 15, false},
		{"unknown_smaller_max_loser", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, true}, {"legacy", "s_sleep", 4, true}}, 15, false},
		{"partial_smaller_max_loser", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, true}, {"mixed", "s_sleep", 4, true}}, 15, false},
		{"unknown_larger_max_winner", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, true}, {"legacy", "s_sleep", 50, true}}, 15, true},
		{"partial_larger_max_winner", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, true}, {"mixed", "s_sleep", 50, true}}, 15, true},
		{"zero_hops_never_enter_max", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 0, true}, {"legacy", "s_sleep", 0, true}}, 5, false},
		{"positive_unknown_without_known_max", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 0, true}, {"legacy", "s_sleep", 4, true}}, 5, true},
		{"native_sleep_state_already_owns_wait", "main", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, false}, {"filtered", "s_sleep", 50, true}}, 15, false},
		{"legacy_hop_scopes_no_window_fallback", "legacy", []rowSpec{{"main", "s_sleep", 10, true}, {"filtered", "s_sleep", 50, true}}, 0, true},
		{"known_foreign_state", "main", []rowSpec{{"main", "runnable", 5, false}, {"filtered", "s_sleep", 50, false}}, 5, true},
		{"known_legacy_independent", "main", []rowSpec{{"main", "runnable", 5, false}, {"legacy", "s_sleep", 50, false}}, 5, true},
		{"known_partial_independent", "main", []rowSpec{{"main", "runnable", 5, false}, {"mixed", "s_sleep", 50, false}}, 5, true},
		{"legacy_keeps_legacy_lane", "legacy", []rowSpec{{"legacy", "runnable", 5, false}, {"main", "s_sleep", 50, false}}, 5, true},
		{"legacy_only_one_known_scope", "legacy", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, false}}, 15, false},
		{"legacy_two_known_scopes_no_election", "legacy", []rowSpec{{"main", "runnable", 5, false}, {"filtered", "s_sleep", 50, false}}, 0, true},
		{"legacy_mixed_no_election", "legacy", []rowSpec{{"main", "runnable", 5, false}, {"mixed", "s_sleep", 50, false}}, 0, true},
		{"mixed_account_no_election", "mixed", []rowSpec{{"main", "runnable", 5, false}, {"main", "s_sleep", 10, false}}, 0, true},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", tc.name, reverse), func(t *testing.T) {
				model := runtimeTraceProjTreeModel{Target: "worker-55", WindowMS: 100, WindowStartTs: 10, WindowEndTs: 10.1,
					TargetMeasurementOrigins: b1638b3TestOrigins(tc.account), Marks: &runtimeTraceProjMarkSet{}}
				for i, row := range tc.rows {
					role := types.TraceCausalRoleRootCauseContext
					if row.hop {
						role = types.TraceCausalRoleCausalHop
					}
					model.SelfRows = append(model.SelfRows, runtimeTraceProjTreeRow{HasData: true, Node: types.TraceCausalProjectionNode{
						EvidenceID: fmt.Sprint(i), Subject: model.Target, Role: role, StateKind: row.state, Object: row.state,
						ImpactMS: row.ms, QueryWindowStartTs: 10, QueryWindowEndTs: 10.1, MeasurementOrigins: b1638b3TestOrigins(row.scope)}})
				}
				if reverse {
					for i, j := 0, len(model.SelfRows)-1; i < j; i, j = i+1, j-1 {
						model.SelfRows[i], model.SelfRows[j] = model.SelfRows[j], model.SelfRows[i]
					}
				}
				model.TreeRows = []runtimeTraceProjTreeRow{{Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
					Node: types.TraceCausalProjectionNode{Subject: "waker-99", ImpactMS: 3, CumulativeImpactMS: 3, QueryWindowStartTs: 10, QueryWindowEndTs: 10.1}}}
				before, _ := json.Marshal(model)
				got, admitted, residueCount, residueMax := runtimeTraceProjTargetSymptomAdmission(model)
				if got != tc.wait {
					t.Fatalf("wait=%v want=%v admitted=%v", got, tc.wait, admitted)
				}
				if strings.Contains(tc.name, "smaller_max_loser") {
					if residueCount != 1 || residueMax != 4 {
						t.Errorf("excluded MAX loser must remain in the old residue census: count=%d max=%f", residueCount, residueMax)
					}
					count, maximum, _ := runtimeTraceProjSymptomDenominatorCensus(types.TraceCausalProjection{}, model)
					if count != 1 || maximum != 4 {
						t.Errorf("excluded source row lost from independent census: count=%d max=%f", count, maximum)
					}
				}
				projection := types.TraceCausalProjection{WindowStartTs: 10, WindowEndTs: 10.1}
				coverage := runtimeTraceProjCoverageVerdictFor(projection, model)
				if !coverage.HasData || coverage.WaitMeasurementScopeLimited != tc.limited || (tc.limited && (coverage.Comparable || coverage.DenominatorMS != 0)) {
					t.Fatalf("scope cannot fall back to a whole-window arithmetic verdict: %+v", coverage)
				}
				for _, zh := range []bool{false, true} {
					text := runtimeTraceProjWindowLine(projection, model, zh)
					if tc.limited {
						word := "event-filter scope"
						if zh {
							word = "事件筛选范围"
						}
						if !strings.Contains(text, word) {
							t.Fatalf("missing scope disclosure: %s", text)
						}
						if strings.Contains(text, "(0%)") || strings.Contains(text, "/0%") || strings.Contains(text, "等待 0.000ms") || strings.Contains(text, "0.000ms wait") {
							t.Fatalf("unavailable denominator became measured zero: %s", text)
						}
					}
				}
				after, _ := json.Marshal(model)
				if string(before) != string(after) {
					t.Fatal("scope admission changed original rows or account origins")
				}
			})
		}
	}
}

func TestB1638B3OnlyOriginalBoardContributorsLimitCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, scope string
		otherMS     float64
		limited     bool
	}{
		{"unknown_losing_board", "legacy", 10, false},
		{"foreign_losing_board", "filtered", 10, false},
		{"unknown_winning_board", "legacy", 80, true},
		{"foreign_winning_board", "filtered", 80, true},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", tc.name, reverse), func(t *testing.T) {
				model := runtimeTraceProjTreeModel{Target: "worker-55", WindowMS: 100, WindowStartTs: 10, WindowEndTs: 10.1,
					TargetMeasurementOrigins: b1638b3TestOrigins("main"), Marks: &runtimeTraceProjMarkSet{}}
				for i, item := range []struct {
					scope string
					ms    float64
				}{{"main", 40}, {tc.scope, tc.otherMS}} {
					model.SelfRows = append(model.SelfRows, runtimeTraceProjTreeRow{HasData: true, Node: types.TraceCausalProjectionNode{
						EvidenceID: fmt.Sprint(i), Subject: model.Target, Role: types.TraceCausalRoleRootCauseContext,
						StateKind: "runnable", Object: "runnable", ImpactMS: item.ms, QueryWindowStartTs: 10, QueryWindowEndTs: 10.1,
						RankBoardTarget: model.Target, RankBoardParamsFingerprint: fmt.Sprint(i), MeasurementOrigins: b1638b3TestOrigins(item.scope)}})
				}
				if reverse {
					model.SelfRows[0], model.SelfRows[1] = model.SelfRows[1], model.SelfRows[0]
				}
				model.TreeRows = []runtimeTraceProjTreeRow{{Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
					Node: types.TraceCausalProjectionNode{Subject: "waker-99", ImpactMS: 3, CumulativeImpactMS: 3, QueryWindowStartTs: 10, QueryWindowEndTs: 10.1}}}
				before, _ := json.Marshal(model)
				wait, admitted, _, _ := runtimeTraceProjTargetSymptomAdmission(model)
				if wait != 40 {
					t.Fatalf("selected scope's original board must retain 40ms: %f %v", wait, admitted)
				}
				coverage := runtimeTraceProjCoverageVerdictFor(types.TraceCausalProjection{WindowStartTs: 10, WindowEndTs: 10.1}, model)
				if coverage.WaitMeasurementScopeLimited != tc.limited {
					t.Fatalf("only removing the old board winner changes scope coverage: %+v", coverage)
				}
				after, _ := json.Marshal(model)
				if string(before) != string(after) {
					t.Fatal("board coverage calculation rewrote original rows")
				}
			})
		}
	}
}
