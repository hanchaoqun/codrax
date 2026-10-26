package tool

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func conflictPage294(t *testing.T, bus *types.BusContext, raw string) (types.ToolResult, map[string]json.RawMessage) {
	t.Helper()
	out, err := (&TraceCatalog{}).Execute(bus, []byte(raw))
	if err != nil || out.Success || len(out.Observations) != 0 || out.ReadCoverage != nil || out.TraceQuerySourceRead.Path() != "" {
		t.Fatalf("conflict must remain failed navigation without evidence: %v %+v", err, out)
	}
	var page map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
		t.Fatalf("failed Summary lacks structured recovery: %v: %s", err, out.Summary)
	}
	if string(page["reason_code"]) != `"trace_catalog_request_plan_conflict"` || string(page["navigation_only"]) != "true" {
		t.Fatalf("missing precise conflict boundary: %s", out.Summary)
	}
	return out, page
}

func TestHMC294RequestPlanConflictPublicRetryPreservesAcceptedSpecification(t *testing.T) {
	for _, action := range []string{"", "discover"} {
		t.Run("action_"+action, func(t *testing.T) {
			bus := requestPlanBus291(t)
			catalogCall285(t, bus, `{"recursive":false,"queries":[{"view":"event_search"}],"representatives":{"per_cluster":1}}`)
			c := bus.Mutable.TraceCatalogs()[0]
			before := c.CurrentExecution()
			rmBefore, _ := json.Marshal(bus.Mutable.RequestModel())
			raw, _ := json.Marshal(map[string]any{"action": action, "root": ".", "recursive": false, "request_plan": map[string]any{"view": "resource_stack"}, "queries": []any{map[string]any{"view": "event_search", "pid": 999}}, "representatives": map[string]any{"per_cluster": 1}})
			failure, page := conflictPage294(t, bus, string(raw))
			if len(failure.Summary) > 2048 {
				t.Fatalf("ordinary recovery exceeded compact tool-message budget: %d", len(failure.Summary))
			}
			if !reflect.DeepEqual(before, c.CurrentExecution()) {
				t.Fatal("failed call changed selection, plans or attempts")
			}
			rmAfter, _ := json.Marshal(bus.Mutable.RequestModel())
			if string(rmBefore) != string(rmAfter) {
				t.Fatal("conflicting parameters changed accepted work")
			}
			if string(page["request_plan_status"]) != `"ready"` || !strings.Contains(string(page["guidance"]), "not an equivalent rewrite") || !strings.Contains(string(page["guidance"]), "accepted") {
				t.Fatalf("candidate route must disclose accepted-field precedence: %v", page)
			}
			next := page["next_call"]
			if err := toolparam.Validate(next, (&TraceCatalog{}).Parameters()); err != nil {
				t.Fatalf("next_call not executable public JSON: %v %s", err, next)
			}
			var args map[string]json.RawMessage
			_ = json.Unmarshal(next, &args)
			if len(args) != 4 || string(args["root"]) != `"."` || string(args["recursive"]) != "false" || string(args["request_plan"]) != `{"view":"resource_stack"}` || string(args["action"]) != `"discover"` {
				t.Fatalf("recovery guessed a view, changed root/recursion or carried the conflicting lane: %s", next)
			}
			retry := catalogCall285(t, bus, string(next))
			s, ok := c.Representatives()
			if !ok || s.Coverage.Members != 8 || s.Coverage.Clusters != 2 || s.Coverage.Selected != 4 || s.Coverage.BudgetNotSelected != 4 || s.Options.PerCluster != 2 || len(s.ExcludedQueryIDs) != 4 || c.CurrentExecution().Coverage.Attempts != 0 {
				t.Fatalf("recovery did not preserve accepted 8/2/4/4 independently of old plans: %+v", s)
			}
			for _, cluster := range s.Clusters {
				p := cluster.Specification
				if p.View != "resource_stack" || p.Object.Kind != "thread" || (p.Object.ID != "101" && p.Object.ID != "202") || p.Window == nil || p.Window.StartNS != 10000000000 || p.Window.EndNS != 10050000000 {
					t.Fatalf("accepted specification changed: %+v", p)
				}
			}
			var calls map[string]json.RawMessage
			_ = json.Unmarshal(retry["query_calls"], &calls)
			if len(calls) != 4 {
				t.Fatalf("selected ordinary calls missing: %s", retry["query_calls"])
			}
			for _, call := range calls {
				out, err := (&TraceQuery{}).Execute(bus, call)
				if err != nil || !out.Success {
					t.Fatalf("public native execution: %v %s", err, out.Summary)
				}
				found := false
				for _, row := range out.Observations {
					if native, ok := DecodeTraceResourceStack(row); ok {
						found = true
						if (native.TargetPID != 101 && native.TargetPID != 202) || native.Window.StartTs != 10 || native.Window.EndTs != 10.05 || native.Window.EndInclusive {
							t.Fatalf("native query identity/window changed: %+v", native)
						}
					}
				}
				if !found {
					t.Fatal("retry did not reach native facts")
				}
			}
			s, _ = c.Representatives()
			if s.Coverage.SelectedOutcomes[tracecatalog.OutcomeSuccess] != 4 || s.Coverage.BudgetNotSelected != 4 {
				t.Fatal("execution conflated selected and unselected work")
			}
		})
	}
}

func TestHMC294RequestPlanConflictPendingAndAmbiguousRoutes(t *testing.T) {
	for _, name := range []string{"profile", "view", "invalid_view", "window", "multi_window", "targets", "capacity", "action", "catalog_id", "snapshot_path", "offset", "export"} {
		t.Run(name, func(t *testing.T) {
			bus := requestPlanBus291(t)
			catalogCall285(t, bus, `{"queries":[{"view":"resource_stack","pid":101}],"representatives":{"per_cluster":1}}`)
			c := bus.Mutable.TraceCatalogs()[0]
			before := c.CurrentExecution()
			rm := *bus.Mutable.RequestModel()
			args := map[string]any{"request_plan": map[string]any{"view": "resource_stack"}, "representatives": map[string]any{"per_cluster": 1}}
			missing := ""
			switch name {
			case "profile":
				rm.RuntimeArtifactWorkProfile = nil
				missing = "runtime_artifact_work_profile"
			case "view":
				args["request_plan"] = map[string]any{}
				missing = "request_plan.view"
			case "invalid_view":
				args["request_plan"] = map[string]any{"view": "invented_view"}
			case "window":
				rm.RuntimeArtifactScopeProfile = nil
				missing = "single_common_explicit_time_window"
			case "multi_window":
				w := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindows()[0]
				rm.RuntimeArtifactScopeProfile.TimeStart, rm.RuntimeArtifactScopeProfile.TimeEnd = nil, nil
				rm.RuntimeArtifactScopeProfile.TimeWindows = []types.RuntimeArtifactTimeWindow{w, w}
				missing = "single_common_explicit_time_window"
			case "targets":
				rm.RuntimeTargets = nil
				missing = "named_user_targets"
			case "capacity":
				rm.RuntimeArtifactWorkProfile.PerSpecificationLimit = 6
				missing = "per_specification_limit_capacity"
			case "action":
				args["action"] = "status"
			case "catalog_id":
				args["catalog_id"] = c.ID()
			case "snapshot_path":
				args["snapshot_path"] = "/no-history-permission.json"
			case "offset":
				args["offset"] = 1
			case "export":
				args["export"] = map[string]any{"target": "not-a-discovery"}
			}
			bus.Mutable.SetRequestModel(rm)
			raw, _ := json.Marshal(args)
			_, page := conflictPage294(t, bus, string(raw))
			if len(page["next_call"]) != 0 || !reflect.DeepEqual(before, c.CurrentExecution()) {
				t.Fatalf("pending or mixed action silently changed route/state: %v", page)
			}
			if missing != "" && (!strings.Contains(string(page["missing_fields"]), missing) || string(page["request_plan_status"]) != `"pending"`) {
				t.Fatalf("typed pending reason lost: %v", page)
			}
		})
	}
}

func TestHMC294RequestPlanConflictAuthorizationAndCancellation(t *testing.T) {
	for _, name := range []string{"outside_root", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			bus := requestPlanBus291(t)
			args := map[string]any{"request_plan": map[string]any{"view": "resource_stack"}, "representatives": map[string]any{"per_cluster": 1}}
			if name == "outside_root" {
				args["root"] = t.TempDir()
				rm := *bus.Mutable.RequestModel()
				rm.RuntimeArtifactWorkProfile = nil
				bus.Mutable.SetRequestModel(rm)
			} else {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				bus.Ctx = ctx
			}
			raw, _ := json.Marshal(args)
			out, err := (&TraceCatalog{}).Execute(bus, raw)
			if err != nil || out.Success || strings.Contains(out.Summary, "next_call") || len(bus.Mutable.TraceCatalogs()) != 0 {
				t.Fatalf("unauthorized/canceled recovery: %v %+v", err, out)
			}
			if name == "outside_root" && (out.Repair == nil || out.Repair.Code != TraceCatalogScopeRefusedCode) {
				t.Fatalf("root authorization must precede missing typed fields: %+v", out)
			}
			if name == "cancelled" && !strings.Contains(out.Summary, "context canceled") {
				t.Fatalf("cancellation lost: %s", out.Summary)
			}
		})
	}
}

func TestHMC294ExplicitManualRouteRemainsIndependent(t *testing.T) {
	bus := requestPlanBus291(t)
	catalogCall285(t, bus, `{"queries":[{"view":"event_search","pid":303,"time_start":11,"time_end":12}],"representatives":{"per_cluster":1}}`)
	s, ok := bus.Mutable.TraceCatalogs()[0].Representatives()
	if !ok || s.Coverage.Members != 4 || s.Coverage.Clusters != 1 || s.Coverage.Selected != 1 || s.Options.PerCluster != 1 || s.Clusters[0].Specification.Object.ID != "303" {
		t.Fatalf("accepted task improperly replaced legal manual exploration: %+v", s)
	}
}

func TestHMC294RecoveryCandidateDoesNotRestoreDirectoryPermission(t *testing.T) {
	bus := requestPlanBus291(t)
	root := t.TempDir()
	bus.UserPinnedFiles = []string{root}
	raw, _ := json.Marshal(map[string]any{"root": root, "request_plan": map[string]any{"view": "resource_stack"}, "representatives": map[string]any{"per_cluster": 1}})
	_, page := conflictPage294(t, bus, string(raw))
	if len(page["next_call"]) == 0 || len(bus.Mutable.TraceCatalogs()) != 0 {
		t.Fatal("expected navigation without discovery")
	}
	bus.UserPinnedFiles = nil
	out, err := (&TraceCatalog{}).Execute(bus, page["next_call"])
	if err != nil || out.Success || out.Repair == nil || out.Repair.Code != TraceCatalogScopeRefusedCode || len(bus.Mutable.TraceCatalogs()) != 0 {
		t.Fatalf("candidate restored prior permission: %v %+v", err, out)
	}
}
