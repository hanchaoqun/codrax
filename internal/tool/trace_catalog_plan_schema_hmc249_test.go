package tool

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func hmc249SchemaObject(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func hmc249WithoutDescriptions(value any) any {
	switch item := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range item {
			if key != "description" {
				out[key] = hmc249WithoutDescriptions(child)
			}
		}
		return out
	case []any:
		out := make([]any, len(item))
		for i, child := range item {
			out[i] = hmc249WithoutDescriptions(child)
		}
		return out
	default:
		return value
	}
}

func TestHMC249CatalogPlanSchemaMirrorsOrdinaryQueryFields(t *testing.T) {
	query := hmc249SchemaObject(t, (&TraceQuery{}).Parameters())
	properties := query["properties"].(map[string]any)
	// A new field or top-level combinator needs explicit plan-scope review.
	known := strings.Fields("archive_member sampling_depth sampling_root comparison source path view thread pid target_scope time_start time_end line_start line_end event_types event_names trace_mark_actions pattern patterns event_field_filters span_name business_span_ref interaction_direction recipe_name max_depth max_branches max_chain_nodes via_thread min_duration_ms include_window_stats limit bucket_ms core_topology trace_flavor platform catalog_query_ref")
	actual := make([]string, 0, len(properties))
	for key := range properties {
		actual = append(actual, key)
	}
	sort.Strings(known)
	sort.Strings(actual)
	if !reflect.DeepEqual(known, actual) {
		t.Fatalf("review new/removed trace_query field for planning: want=%v got=%v", known, actual)
	}
	for key := range query {
		if key != "type" && key != "properties" && key != "additionalProperties" && key != "if" && key != "then" {
			t.Fatalf("review new ordinary-query schema constraint: %s", key)
		}
	}
	if !reflect.DeepEqual(query["if"], map[string]any{"required": []any{"catalog_query_ref"}}) ||
		!reflect.DeepEqual(hmc249WithoutDescriptions(query["then"]), map[string]any{
			"additionalProperties": false,
			"properties":           map[string]any{"catalog_query_ref": hmc249WithoutDescriptions(properties["catalog_query_ref"])},
		}) {
		t.Fatal("review changed alternate-route constraint before projecting the ordinary plan branch")
	}
	catalog := hmc249SchemaObject(t, (&TraceCatalog{}).Parameters())
	item := catalog["properties"].(map[string]any)["queries"].(map[string]any)["items"].(map[string]any)
	planProperties, ok := item["properties"].(map[string]any)
	if !ok || item["additionalProperties"] != false || !reflect.DeepEqual(item["required"], []any{"view"}) {
		t.Fatalf("plan is an opaque object instead of executable ordinary schema: %v", item)
	}
	for _, key := range []string{"source", "path", "comparison", "business_span_ref", "catalog_query_ref"} {
		delete(properties, key)
		if _, advertised := planProperties[key]; advertised {
			t.Errorf("capture-local or alternate route exposed in per-candidate plan: %s", key)
		}
	}
	if !reflect.DeepEqual(hmc249WithoutDescriptions(properties), hmc249WithoutDescriptions(planProperties)) {
		t.Fatal("plan fields changed ordinary query types/enums/limits/compatibility metadata")
	}
	encoded, _ := json.Marshal(item)
	if len(encoded) > 12<<10 {
		t.Fatalf("plan schema copied the giant query teaching: %d bytes", len(encoded))
	}
	t.Logf("plan schema bytes=%d; ordinary query schema bytes=%d", len(encoded), len((&TraceQuery{}).Parameters()))
}

func TestHMC249CatalogPlanPublicSchemaRejectsInvalidAndLocalRoutes(t *testing.T) {
	for name, plan := range map[string]string{
		"view_missing":  `{"pid":101}`,
		"unknown_field": `{"view":"resource_stack","invented":true}`,
		"member_type":   `{"view":"perf_stats","archive_member":42}`,
		"unknown_view":  `{"view":"invented"}`,
		"wrong_pid":     `{"view":"resource_stack","pid":[]}`,
		"wrong_scope":   `{"view":"resource_stack","target_scope":"all_processes"}`,
		"wrong_depth":   `{"view":"wakeup_chain","max_depth":10000}`,
		"local_path":    `{"view":"resource_stack","path":"one.data"}`,
		"local_source":  `{"view":"resource_stack","source":"attached_trace"}`,
		"local_ref":     `{"view":"resource_stack","business_span_ref":"old"}`,
		"reference":     `{"catalog_query_ref":{"catalog_id":"old","query_id":"old"}}`,
		"comparison":    `{"comparison":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := json.RawMessage(`{"queries":[` + plan + `]}`)
			if err := toolparam.Validate(raw, (&TraceCatalog{}).Parameters()); err == nil {
				t.Fatalf("public plan schema accepted %s", raw)
			}
			// The model-facing contract is strict; direct Execute retains its
			// pre-existing defaults/enums/clamping compatibility. Do not turn a
			// schema teaching repair into a new runtime validation policy.
			if name == "view_missing" || name == "wrong_scope" || name == "wrong_depth" {
				return
			}
			ctx, _ := catalogPublicFixture(t)
			out, _ := (&TraceCatalog{}).Execute(ctx, raw)
			if out.Success || len(ctx.Mutable.TraceCatalogs()) != 0 {
				t.Fatal("invalid planning input installed work or was accepted")
			}
		})
	}
	for _, plan := range []string{
		`{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}`,
		`{"view":"event_search","pid":101,"patterns":["AllocEvent","FreeEvent"],"event_names":["native_hook"],"limit":5}`,
		`{"view":"wakeup_chain","pid":101,"max_depth":2,"include_window_stats":false,"target_scope":"thread"}`,
		`{"view":"perf_sampling_tree","sampling_depth":8,"time_start":0,"time_end":1}`,
		// Exact member coordinates can be declared per candidate; they do not
		// execute preparation or select a physical capture during planning.
		`{"view":"perf_stats","archive_member":"captures/device capture.data","time_start":0,"time_end":1}`,
	} {
		raw := json.RawMessage(`{"queries":[` + plan + `]}`)
		if err := toolparam.Validate(raw, (&TraceCatalog{}).Parameters()); err != nil {
			t.Fatalf("ordinary plan rejected: %s: %v", raw, err)
		}
	}
}

func TestHMC249CatalogPlanNavigationAndRediscoveryKeepAttempts(t *testing.T) {
	ctx, root := catalogPublicFixture(t)
	out, err := (&TraceCatalog{}).Execute(ctx, []byte(`{}`))
	if err != nil || !out.Success {
		t.Fatalf("discovery: %v %s", err, out.Summary)
	}
	var page map[string]any
	if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
		t.Fatal(err)
	}
	execution, _ := page["execution"].(string)
	if !strings.Contains(execution, "before selecting representatives") || !strings.Contains(execution, "queries") || !strings.Contains(execution, "view") || !strings.Contains(execution, "ordinary trace_query") {
		t.Errorf("zero-plan navigation did not explain the missing planning step without banning ordinary queries: %s", execution)
	}
	if page["query_calls"] != nil || page["representatives_call"] != nil {
		t.Error("zero-plan discovery invented executable queries or representative work")
	}
	c := ctx.Mutable.TraceCatalogs()[0]
	id := c.ID()
	result := catalogPublicQuery(t, ctx, root+"/capture.data", 101)
	if !result.Success {
		t.Fatal(result.Summary)
	}
	before := c.Snapshot()
	if len(before.Queries) != 1 || len(before.Queries[0].Attempts) != 1 {
		t.Fatalf("ordinary query did not remain available: %+v", before.Queries)
	}
	queryID := before.Queries[0].ID
	attemptJSON, _ := json.Marshal(before.Queries[0].Attempts)
	plans := []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":999,"time_start":10,"time_end":10.05}]}`)
	if err := toolparam.Validate(plans, (&TraceCatalog{}).Parameters()); err != nil {
		t.Fatal(err)
	}
	out, err = (&TraceCatalog{}).Execute(ctx, plans)
	if err != nil || !out.Success {
		t.Fatalf("plan declaration after discovery: %v %s", err, out.Summary)
	}
	if len(ctx.Mutable.TraceCatalogs()) != 1 || ctx.Mutable.TraceCatalogs()[0].ID() != id {
		t.Fatal("adding plans replaced catalog identity")
	}
	c = ctx.Mutable.TraceCatalogs()[0]
	q, ok := c.Query(queryID)
	afterJSON, _ := json.Marshal(q.Attempts)
	if !ok || q.Outcome != tracecatalog.OutcomeSuccess || string(attemptJSON) != string(afterJSON) || len(c.Snapshot().Queries) != 6 {
		t.Fatal("adding expected plans lost/replaced the actual query attempt")
	}
	if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
		t.Fatal(err)
	}
	selection, ok := page["representatives_call"].(map[string]any)
	if !ok || selection["action"] != "representatives" || selection["catalog_id"] != id || len(selection) != 2 {
		t.Fatalf("missing optional exact selection navigation: %+v", page["representatives_call"])
	}
	if _, selected := c.Representatives(); selected {
		t.Fatal("planning selected representatives on behalf of the model")
	}
	raw, _ := json.Marshal(selection)
	if err := toolparam.Validate(raw, (&TraceCatalog{}).Parameters()); err != nil {
		t.Fatalf("selection action not executable under actual schema: %v", err)
	}
}
