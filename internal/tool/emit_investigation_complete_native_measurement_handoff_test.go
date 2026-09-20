package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Native measurements have their own accepted, scoped observation carrier.
// The completion cap bounds model-authored aggregate rows, not the number of
// independent measurements that a query can preserve for answer writing.
func nativeMeasurementHandoffContext(t *testing.T) *types.BusContext {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_io_request_latency_distribution", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 14.0
	ctx := &types.BusContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("Report the measured distributions"),
		TurnRouteHint: types.TurnRouteHint{Route: "repo", Source: "external_tool", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentReturnValue, Predicates: types.SemanticPredicates{IsScalarAnswer: true},
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1..14 seconds"},
		}},
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("public native query failed: %+v %v", result, err)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	return ctx
}

func TestNativeMeasurementHandoffSchemaAndCapTeachScopedReuse(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
			MaxItems    int    `json:"maxItems"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&EmitInvestigationComplete{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	ctx := nativeMeasurementHandoffContext(t)
	before, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
	count := types.MaxAnswerAggregateFacts + 1
	params, _ := json.Marshal(map[string]any{"reason": "Measured values are independently retained by the native query", "confidence": "high", "result_kind": "resolved", "aggregate_facts": emit2AggregateFacts(count, count)})
	result, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
	if err != nil || result.Success || ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("native observations must not waive the existing aggregate cap: %+v %v", result, err)
	}
	if schema.Properties["aggregate_facts"].MaxItems != 16 || len(ctx.Mutable.StableInvestigationAggregateFacts()) != 0 {
		t.Fatal("teaching changed the cap or published rejected aggregates")
	}
	if !strings.Contains(schema.Properties["reason"].Description, "under the scoped reuse rules in aggregate_facts") {
		t.Error("reason teaching must not independently demand duplicate native measurements")
	}
	for surface, text := range map[string]string{"schema": schema.Properties["aggregate_facts"].Description, "public cap rejection": result.Summary} {
		for _, want := range []string{
			"accepted native typed query observations",
			"do not need a duplicate aggregate_facts entry",
			"same source, query receipt, target/window scope, measurement identity, value and unit",
			"raw prose, inferred values, and another scope are not substitutes",
			"Preserve unique principal facts and required member-set handoffs",
			"do not relabel them as audit_ledger to evade the cap",
			"does not guarantee that bounded observation views display every measurement",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s omitted native reuse boundary %q", surface, want)
			}
		}
		if strings.Contains(text, "a deleted entry's typed value leaves the answer permanently") {
			t.Errorf("%s incorrectly treats accepted native measurements as owned only by the rejected aggregate", surface)
		}
	}
	after, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
	if string(before) != string(after) {
		t.Fatal("cap rejection changed accepted native observations")
	}
}

func TestNativeMeasurementHandoffClosesWithoutDuplicatingNativeScalars(t *testing.T) {
	ctx := nativeMeasurementHandoffContext(t)
	before := ctx.Mutable.DispatchToolResults()
	beforeJSON, _ := json.Marshal([]any{before, ctx.Mutable.TurnAArtifacts()})
	groups := map[string]string{
		"event=block_rq dev=12,80 op=R":  "samples=11 min=1.000 mean=6.000 max=11.000 p50=6.000 p90=10.000 p95=10.500 p99=10.900",
		"event=block_rq dev=12,80 op=W":  "samples=3 min=5.000 mean=15.000 max=25.000 p50=15.000 p90=23.000 p95=24.000 p99=24.800",
		"event=block_bio dev=12,80 op=R": "samples=3 min=2.000 mean=4.000 max=6.000 p50=4.000 p90=5.600 p95=5.800 p99=5.960",
	}
	for group, numbers := range groups {
		found := false
		for _, result := range before {
			for _, observation := range result.Observations {
				if observation.Predicate == "storage_latency_by_layer" && strings.Contains(observation.Summary, group) && strings.Contains(observation.Summary, numbers) {
					found = true
					if observation.Producer != "trace_query" || observation.GroundingPolicy != types.ClaimGroundingHard || observation.SourceRef.Path == "" || observation.SourceRef.QueryScopeID == "" {
						t.Fatalf("measured group lost its native scope receipt: %+v", observation)
					}
				}
			}
		}
		if !found {
			t.Fatalf("public native query did not retain all eight values for %s", group)
		}
	}
	params := json.RawMessage(`{"reason":"Use the accepted query measurements within their recorded scope; no duplicate scalar handoff is needed", "confidence":"high", "result_kind":"resolved"}`)
	result, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
	if err != nil || !result.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("native measurements must remain usable without 24 duplicate aggregate rows: %+v %v", result, err)
	}
	if len(ctx.Mutable.StableInvestigationAggregateFacts()) != 0 {
		t.Fatal("completion manufactured aggregate copies")
	}
	if !reflect.DeepEqual(before, ctx.Mutable.DispatchToolResults()) || !reflect.DeepEqual(before, ctx.Mutable.TurnAArtifacts().ToolResults) {
		t.Fatal("completion changed the original query measurements or TurnA handoff")
	}
	afterJSON, _ := json.Marshal([]any{ctx.Mutable.DispatchToolResults(), ctx.Mutable.TurnAArtifacts()})
	if string(beforeJSON) != string(afterJSON) {
		t.Fatal("completion mutated an aliased native measurement or TurnA field")
	}
}
