package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func resourceProjectionFixtureHMC286(t *testing.T) (*types.BusContext, types.ToolResult) {
	t.Helper()
	bus, _, _ := resourceMeasurementContextHMC249(t, false, 0)
	// Query first without a task-window default: the observed inclusive
	// envelope is not the later half-open request or its completeness proof.
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": bus.RepoRoot + "/capture.data", "view": "resource_stack"})
	result, err := (&TraceQuery{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("native wide query: %v %s", err, result.Summary)
	}
	start, end := 10.0, 10.05
	bus.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"},
		{Kind: types.RuntimeTargetKindThread, PID: 202, Source: "user_explicit"},
	}
	bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, SourceQuote: "[10,10.05)", TimeStart: &start, TimeEnd: &end}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return bus, result
}

func TestHMC286ResourceProjectionCurrentReceiptAndLedger(t *testing.T) {
	bus, result := resourceProjectionFixtureHMC286(t)
	input := types.ObservationLedgerInputFromBusContext(bus, 0)
	original := resourceRecordHMC279(t, result)
	projection, ok := types.RuntimeResourceCurrentDisplayProjection(input, result, original)
	if !ok || projection.OriginalMatchedEvents != 5 || projection.RetainedEvents != 5 || projection.ProjectedEvents != 4 || projection.WindowExcludedEvents != 1 {
		t.Fatalf("native projection counts: %+v ok=%v", projection, ok)
	}
	ledgerFound := false
	for _, r := range types.CompileObservationLedger(input).Records {
		if r.ID == original.ID {
			ledgerFound = true
			if got, ok := types.RuntimeResourceCurrentDisplayProjection(input, result, r); !ok || !reflect.DeepEqual(got, projection) {
				t.Fatalf("ledger lost same native projection: %+v %v", got, ok)
			}
		}
	}
	if !ledgerFound {
		t.Fatal("missing ledger record")
	}
	for _, mode := range []string{"tool_call_navigation", "authority", "payload", "source", "json", "missing", "duplicate", "cancel", "memo", "failed"} {
		t.Run(mode, func(t *testing.T) {
			r, page := original, result
			page.Observations = append([]types.ObservationRecord(nil), result.Observations...)
			switch mode {
			case "tool_call_navigation":
				r.SourceRef.ToolCallID = "ledger-navigation"
			case "authority":
				r.ClaimAuthority = types.ObservationClaimAuthorityModelInference
			case "payload":
				r.RichNotes = append(append([]string(nil), r.RichNotes...), "changed")
			case "source":
				r.SourceRef.PayloadRef += "changed"
			case "json":
				wire, _ := json.Marshal(page)
				_ = json.Unmarshal(wire, &page)
				// A fresh value is required: unmarshal does not clear unexported fields.
				var restored types.ToolResult
				_ = json.Unmarshal(wire, &restored)
				page = restored
			case "missing":
				page.Observations = nil
			case "duplicate":
				page.Observations = append(page.Observations, original)
			case "cancel":
				page.TraceViewCancellation = &types.TraceViewCancellation{}
			case "memo":
				page.ReusedFromRunMemo = true
			case "failed":
				page.Success = false
			}
			_, ok := types.RuntimeResourceCurrentDisplayProjection(input, page, r)
			if ok != (mode == "tool_call_navigation") {
				t.Fatalf("receipt mode %s accepted=%v", mode, ok)
			}
		})
	}
}

func TestHMC286ResourceProjectionExactAndEmptyKeepOriginalPath(t *testing.T) {
	for _, pid := range []int{101, 202, 999} {
		t.Run(fmt.Sprint(pid), func(t *testing.T) {
			bus, result, _ := resourceMeasurementContextHMC249(t, false, pid)
			start, end := 10.0, 10.05
			bus.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"}}
			bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, SourceQuote: "window", TimeStart: &start, TimeEnd: &end}
			input := types.ObservationLedgerInputFromBusContext(bus, 0)
			r := resourceRecordHMC279(t, result)
			if _, ok := types.RuntimeResourceCurrentDisplayProjection(input, result, r); ok {
				t.Fatal("exact/empty query unnecessarily projected")
			}
			view := types.BuildAnswerSemanticViewForBusContext(bus)
			if pid == 101 {
				d := resourceDistributionHMC279(t, view)
				if strings.Contains(d.Label, "原查询总体统计") {
					t.Fatal("exact distribution downgraded")
				}
			} else if pid == 202 && !types.RuntimeResourceSupplementaryQueryCurrent(input, result, r) {
				t.Fatal("independent target supplementary path lost")
			}
		})
	}
}

func TestHMC286ResourceProjectionInvalidTargetCannotMeanAll(t *testing.T) {
	bus, result := resourceProjectionFixtureHMC286(t)
	r := resourceRecordHMC279(t, result)
	for _, target := range []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread},
		{Kind: types.RuntimeTargetKindThread, PID: types.RuntimeTargetMaxPID + 1},
		{Kind: types.RuntimeTargetKindProcess},
		{Kind: types.RuntimeTargetKindProcess, PID: types.RuntimeTargetMaxPID + 1},
		{Kind: types.RuntimeTargetKind("unknown"), PID: 101},
	} {
		target.Source = "user_explicit"
		bus.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{target}
		if _, ok := types.RuntimeResourceCurrentDisplayProjection(types.ObservationLedgerInputFromBusContext(bus, 0), result, r); ok {
			t.Fatalf("invalid formal selector became all: %+v", target)
		}
	}
}

func TestHMC286ResourceProjectionWindowUnionAndComparisonIsolation(t *testing.T) {
	bus, result := resourceProjectionFixtureHMC286(t)
	r := resourceRecordHMC279(t, result)
	p, _ := DecodeTraceResourceStack(r)
	first := float64(p.Events[0].TimestampNS) / 1e9
	last := float64(p.Events[len(p.Events)-1].TimestampNS) / 1e9
	firstEnd, lastEnd := first+0.000000001, last+0.000000001
	w1 := types.RuntimeArtifactTimeWindow{TimeStart: &first, TimeEnd: &firstEnd, SourceQuote: "first"}
	w2 := types.RuntimeArtifactTimeWindow{TimeStart: &last, TimeEnd: &lastEnd, SourceQuote: "last"}
	scope := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, SourceQuote: "union", TimeWindows: []types.RuntimeArtifactTimeWindow{w1, w1, w2}}
	bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = scope
	input := types.ObservationLedgerInputFromBusContext(bus, 0)
	got, ok := types.RuntimeResourceCurrentDisplayProjection(input, result, r)
	if !ok || !reflect.DeepEqual(got.SelectedEventIndexes, []int{0, len(p.Events) - 1}) {
		t.Fatalf("union did not preserve unique whole events: %+v %v", got, ok)
	}
	// The actual source owns only first; peer's last window cannot lend scope.
	scope.TimeWindows = nil
	scope.Comparison = &types.RuntimeArtifactComparisonScope{Baseline: types.RuntimeArtifactComparisonSide{Path: bus.RepoRoot + "/capture.data", RuntimeArtifactTimeWindow: w1}, Current: types.RuntimeArtifactComparisonSide{Path: bus.RepoRoot + "/peer.data", RuntimeArtifactTimeWindow: w2}}
	got, ok = types.RuntimeResourceCurrentDisplayProjection(input, result, r)
	if !ok || !reflect.DeepEqual(got.SelectedEventIndexes, []int{0}) {
		t.Fatalf("borrowed comparison peer window: %+v %v", got, ok)
	}
	// No window on this side must not inherit the peer's narrow window.
	scope.Comparison.Baseline.RuntimeArtifactTimeWindow = types.RuntimeArtifactTimeWindow{SourceQuote: "whole baseline"}
	got, ok = types.RuntimeResourceCurrentDisplayProjection(input, result, r)
	if !ok || got.ProjectedEvents != 5 {
		t.Fatalf("unbounded side inherited peer window: %+v %v", got, ok)
	}
}

func TestHMC286ResourceWideQueryProjectsNativeTargetWindow(t *testing.T) {
	bus, result := resourceProjectionFixtureHMC286(t)
	before, _ := json.Marshal(result.Observations)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	tables := resourceNativeTablesHMC249(view)
	if len(tables) != 2 {
		t.Fatalf("wide native members lost before display projection: tables=%d", len(tables))
	}
	for _, table := range tables {
		want := 4
		if table.View == types.RuntimeMeasurementMembers {
			want = 11
		}
		if len(table.Rows) != want || table.MemberSet != nil || table.DisplayPopulation != nil || table.HasCompleteNativeDisplayPopulation() {
			t.Fatalf("projected rows/authority: %s rows=%d want=%d %+v", table.View, len(table.Rows), want, table)
		}
	}
	after, _ := json.Marshal(result.Observations)
	if string(before) != string(after) {
		t.Fatal("display projection rewrote original observation")
	}
}
