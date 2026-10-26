package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func requestPlanBus291(t *testing.T) *types.BusContext {
	t.Helper()
	bus := scopedRepresentativesBus285(t)
	start, end := 10., 10.05
	rm := types.RequestModel{
		RuntimeArtifactWorkProfile:  &types.RuntimeArtifactWorkProfile{Mode: types.RuntimeArtifactWorkMode("representative"), PerSpecificationLimit: 2, SourceQuote: "每类挑两份代表"},
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "线程101和202"},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"}, {Kind: types.RuntimeTargetKindThread, PID: 202, Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10到10.05秒"},
	}
	bus.Mutable.SetRequestModel(rm)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	return bus
}

func TestHMC291RequestPlanPublicExactSelectionAndExecution(t *testing.T) {
	bus := requestPlanBus291(t)
	catalogCall285(t, bus, `{}`)
	c := bus.Mutable.TraceCatalogs()[0]
	prior, err := (&TraceQuery{}).Execute(bus, []byte(`{"source":"path","path":"a.data","view":"resource_stack","pid":101,"time_start":0,"time_end":100}`))
	if err != nil || !prior.Success {
		t.Fatalf("ordinary auxiliary exploration: %v %s", err, prior.Summary)
	}
	before := c.CurrentExecution()
	page := catalogCall285(t, bus, `{"action":"discover","request_plan":{"view":"resource_stack"}}`)
	selected, ok := c.Representatives()
	if !ok || selected.Coverage.Members != 8 || selected.Coverage.Clusters != 2 || selected.Coverage.Selected != 4 || selected.Coverage.BudgetNotSelected != 4 || len(selected.ExcludedQueryIDs) != 1 {
		t.Fatalf("typed work did not produce exact task selection: %+v", selected)
	}
	if c.CurrentExecution().Coverage.Attempts != before.Coverage.Attempts || len(c.Snapshot().Queries) != 9 {
		t.Fatal("planning executed work or lost the independent query")
	}
	for _, cluster := range selected.Clusters {
		if cluster.Specification.Object.Kind != "thread" || (cluster.Specification.Object.ID != "101" && cluster.Specification.Object.ID != "202") || cluster.Specification.View != "resource_stack" ||
			cluster.Specification.Window == nil || cluster.Specification.Window.StartNS != 10000000000 || cluster.Specification.Window.EndNS != 10050000000 {
			t.Fatalf("task target/view/window changed: %+v", cluster.Specification)
		}
	}
	var calls map[string]json.RawMessage
	if err := json.Unmarshal(page["query_calls"], &calls); err != nil || len(calls) != 4 {
		t.Fatalf("selected executable calls missing: %v %s", err, page["query_calls"])
	}
	for _, call := range calls {
		out, err := (&TraceQuery{}).Execute(bus, call)
		if err != nil || !out.Success {
			t.Fatalf("normal selected native execution: %v %s", err, out.Summary)
		}
		found := false
		for _, row := range out.Observations {
			if p, ok := DecodeTraceResourceStack(row); ok {
				found = true
				if (p.TargetPID != 101 && p.TargetPID != 202) || p.Window.StartTs != 10 || p.Window.EndTs != 10.05 || p.Window.EndInclusive {
					t.Fatalf("ordinary query changed accepted target/window: %+v", p)
				}
			}
		}
		if !found {
			t.Fatal("selected call never reached native resource facts")
		}
	}
	status, _ := json.Marshal(map[string]any{"action": "representative_status", "catalog_id": c.ID()})
	catalogCall285(t, bus, string(status))
	current, _ := c.Representatives()
	if current.SelectionID != selected.SelectionID || current.Coverage.SelectedOutcomes[tracecatalog.OutcomeSuccess] != 4 || current.Coverage.Outcomes[tracecatalog.OutcomeNotExecuted] != 4 {
		t.Fatalf("selected execution and unselected state conflated: %+v", current)
	}
	// Selection never denies real work on an unselected member.
	for _, cluster := range current.Clusters {
		for _, member := range cluster.Members {
			if member.Selected {
				continue
			}
			call, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": member.QueryID}})
			out, err := (&TraceQuery{}).Execute(bus, call)
			if err != nil || !out.Success {
				t.Fatalf("unselected query refused: %v %s", err, out.Summary)
			}
			after, _ := c.Representatives()
			if after.SelectionID != selected.SelectionID || after.Coverage.Selected != 4 || after.Coverage.BudgetNotSelected != 4 || after.Coverage.Outcomes[tracecatalog.OutcomeSuccess] != 5 {
				t.Fatal("real unselected execution changed selection membership")
			}
			return
		}
	}
	t.Fatal("missing unselected control")
}

func TestHMC291RequestPlanPendingHasNoPartialMutation(t *testing.T) {
	for _, name := range []string{"profile", "view", "window", "multi_window", "cursor", "invalid_target", "comparison", "budget", "over_capacity_budget", "too_many_targets"} {
		t.Run(name, func(t *testing.T) {
			bus := requestPlanBus291(t)
			catalogCall285(t, bus, `{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}],"representatives":{"per_cluster":1}}`)
			c := bus.Mutable.TraceCatalogs()[0]
			before := c.CurrentExecution()
			rm := *bus.Mutable.RequestModel()
			raw := `{"request_plan":{"view":"resource_stack"}}`
			switch name {
			case "profile":
				rm.RuntimeArtifactWorkProfile = nil
			case "view":
				raw = `{"request_plan":{}}`
			case "window":
				rm.RuntimeArtifactScopeProfile = nil
			case "multi_window":
				w := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindows()[0]
				rm.RuntimeArtifactScopeProfile.TimeStart, rm.RuntimeArtifactScopeProfile.TimeEnd = nil, nil
				rm.RuntimeArtifactScopeProfile.TimeWindows = []types.RuntimeArtifactTimeWindow{w, w}
			case "cursor":
				rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: types.RuntimeTargetSourceExplicitToolCall}}
			case "invalid_target":
				rm.RuntimeTargets = append(rm.RuntimeTargets, types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, Source: "user_explicit"})
			case "comparison":
				rm.RuntimeArtifactScopeProfile.Comparison = &types.RuntimeArtifactComparisonScope{}
			case "budget":
				rm.RuntimeArtifactWorkProfile = &types.RuntimeArtifactWorkProfile{Mode: types.RuntimeArtifactWorkMode("representative"), SourceQuote: "每类代表"}
			case "over_capacity_budget":
				rm.RuntimeArtifactWorkProfile = &types.RuntimeArtifactWorkProfile{Mode: types.RuntimeArtifactWorkMode("representative"), PerSpecificationLimit: 6, SourceQuote: "每组6份"}
			case "too_many_targets":
				for i := 0; i < 17; i++ {
					rm.RuntimeTargets = append(rm.RuntimeTargets, types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 300 + i, Source: "user_explicit"})
				}
			}
			bus.Mutable.SetRequestModel(rm)
			page := catalogCall285(t, bus, raw)
			if string(page["status"]) != `"pending"` || len(page["missing_fields"]) == 0 || len(page["query_calls"]) != 0 {
				t.Fatalf("missing typed input became a plan: %v", page)
			}
			if name == "over_capacity_budget" && !strings.Contains(string(page["missing_fields"]), "per_specification_limit_capacity") {
				t.Fatal("tool capacity silently changed the accepted user budget")
			}
			if !reflect.DeepEqual(before, c.CurrentExecution()) {
				t.Fatal("pending plan mutated current plans, attempts or selection")
			}
		})
	}
}

func TestHMC291RequestPlanAuthorizationAndExplicitBoundaries(t *testing.T) {
	bus := requestPlanBus291(t)
	rm := *bus.Mutable.RequestModel()
	rm.RuntimeArtifactWorkProfile = nil
	bus.Mutable.SetRequestModel(rm)
	raw, _ := json.Marshal(map[string]any{"root": t.TempDir(), "request_plan": map[string]any{}})
	out, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil || out.Success || out.Repair == nil || out.Repair.Code != TraceCatalogScopeRefusedCode || strings.Contains(out.Summary, `"status":"pending"`) {
		t.Fatalf("pending fields bypassed root authorization: %v %+v", err, out)
	}
	for _, raw := range []string{
		`{"action":"status","request_plan":{"view":"resource_stack"}}`,
		`{"request_plan":{"view":"resource_stack"},"queries":[{"view":"event_search"}]}`,
		`{"request_plan":{"view":"resource_stack"},"representatives":{"per_cluster":1}}`,
		`{"request_plan":{"view":"resource_stack"},"offset":1}`,
		`{"request_plan":{"view":"invented"}}`,
	} {
		out, _ := (&TraceCatalog{}).Execute(bus, []byte(raw))
		if out.Success {
			t.Fatalf("ambiguous/invalid explicit plan accepted: %s", raw)
		}
	}
	if len(bus.Mutable.TraceCatalogs()) != 0 {
		t.Fatal("denied request installed a catalog")
	}
	bus = requestPlanBus291(t)
	catalogCall285(t, bus, `{}`)
	if c := bus.Mutable.TraceCatalogs()[0]; len(c.Snapshot().Queries) != 0 || c.CurrentExecution().Selection != nil {
		t.Fatal("plain discovery implicitly planned or selected")
	}
}

func TestHMC291RequestPlanExhaustiveDoesNotSelect(t *testing.T) {
	bus := requestPlanBus291(t)
	rm := *bus.Mutable.RequestModel()
	rm.RuntimeArtifactWorkProfile = &types.RuntimeArtifactWorkProfile{Mode: types.RuntimeArtifactWorkMode("exhaustive"), SourceQuote: "全部检查"}
	bus.Mutable.SetRequestModel(rm)
	catalogCall285(t, bus, `{"request_plan":{"view":"resource_stack"}}`)
	c := bus.Mutable.TraceCatalogs()[0]
	if len(c.Snapshot().Queries) != 8 || c.CurrentExecution().Selection != nil || c.CurrentExecution().Coverage.Attempts != 0 {
		t.Fatal("exhaustive plan selected representatives or executed queries")
	}
}

func TestHMC291RequestPlanPreservesOrdinaryProcessViewBoundary(t *testing.T) {
	for _, view := range []string{"resource_stack", "frame_timeline"} {
		t.Run(view, func(t *testing.T) {
			bus := requestPlanBus291(t)
			rm := *bus.Mutable.RequestModel()
			for i := range rm.RuntimeTargets {
				rm.RuntimeTargets[i].Kind = types.RuntimeTargetKindProcess
			}
			bus.Mutable.SetRequestModel(rm)
			raw, _ := json.Marshal(map[string]any{"request_plan": map[string]string{"view": view}})
			page := catalogCall285(t, bus, string(raw))
			if view == "resource_stack" {
				if string(page["status"]) != `"pending"` || !strings.Contains(string(page["missing_fields"]), "unsupported_target_scope_for_view") || len(bus.Mutable.TraceCatalogs()) != 0 {
					t.Fatal("compiler registered a combination the ordinary query refuses")
				}
				return
			}
			selected, ok := bus.Mutable.TraceCatalogs()[0].Representatives()
			if !ok || selected.Coverage.Members != 8 || selected.Coverage.Selected != 4 {
				t.Fatal("supported process query lost exact task plans")
			}
			for _, cluster := range selected.Clusters {
				if cluster.Specification.Object.Kind != "process" || cluster.Specification.View != view {
					t.Fatal("process query silently became a thread or another view")
				}
			}
		})
	}
}
