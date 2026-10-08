package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceCapabilitiesPagedCatalogPreservesEveryContract(t *testing.T) {
	want, err := tracequery.TraceCapabilities("", true)
	if err != nil {
		t.Fatal(err)
	}
	input := traceCapabilitiesInput{Detail: true}
	var combined []tracequery.MetricCapability
	pages := 0
	for {
		params, _ := json.Marshal(input)
		registry := NewRegistry()
		RegisterDefaults(registry)
		result, err := registry.Execute(nil, "trace_capabilities", params)
		if err != nil || !result.Success {
			t.Fatalf("page %d unavailable: %v %+v", pages, err, result)
		}
		pages++
		if len(result.Summary) > types.ToolDocumentationMaxBytes || result.Handoff == nil || result.Handoff.Documentation == nil || string(result.Handoff.Documentation.Content) != result.Summary || result.Handoff.Documentation.Selection.Cursor != input.Cursor {
			t.Fatal("page did not retain bounded, exact typed handoff")
		}
		var got traceCapabilitiesPayload
		if err := json.Unmarshal([]byte(result.Summary), &got); err != nil {
			t.Fatal(err)
		}
		page := got.MetricPage
		if page == nil || page.Offset != len(combined) || page.Returned != len(got.Metrics) || page.Total != len(want.Metrics) || page.AllMetricsInResponse || len(got.Metrics) == 0 {
			t.Fatalf("incorrect page inventory: %+v", page)
		}
		// Compare every non-metric field too: moving a contract onto another
		// page must not remove its input-format or catalog-wide qualifications.
		combined = append(combined, got.Metrics...)
		got.CapabilityCatalog.Metrics = want.Metrics
		if !reflect.DeepEqual(got.CapabilityCatalog, want) {
			t.Fatal("pagination changed catalog context or qualifications")
		}
		if !page.HasMore {
			if page.NextCall != nil || len(combined) != len(want.Metrics) {
				t.Fatal("last page silently omitted contracts")
			}
			break
		}
		if page.NextCall == nil || !page.NextCall.Detail || page.NextCall.View != input.View || pages >= len(want.Metrics) {
			t.Fatal("invalid forward continuation")
		}
		input = *page.NextCall
	}
	// Compare the published JSON contract: omitempty can legitimately turn an
	// engine's empty slice into nil on a wire round trip, without losing data.
	gotMetrics, _ := json.Marshal(combined)
	wantMetrics, _ := json.Marshal(want.Metrics)
	if pages < 2 || string(gotMetrics) != string(wantMetrics) {
		t.Fatal("paged detail lost, duplicated or changed a metric contract")
	}
}

func TestTraceCapabilitiesPageCursorSelectionAndCatalogBinding(t *testing.T) {
	catalog, _ := tracequery.TraceCapabilities("", true)
	body, err := marshalTraceCapabilitiesPage(catalog, nil, traceCapabilitiesInput{Detail: true}, types.ToolDocumentationMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	var first traceCapabilitiesPayload
	if err := json.Unmarshal(body, &first); err != nil || first.MetricPage == nil || first.MetricPage.NextCall == nil {
		t.Fatal("fixture needs a continuation")
	}
	next := *first.MetricPage.NextCall
	for _, mode := range []string{"overview", "view", "catalog", "aliases", "negative", "past_end", "noncanonical", "garbage"} {
		t.Run(mode, func(t *testing.T) {
			selected, _ := tracequery.TraceCapabilities("", true)
			input, aliases := next, map[string]string(nil)
			switch mode {
			case "overview":
				input.Detail = false
			case "view":
				selected, _ = tracequery.TraceCapabilities("window_stats", true)
				input.View = "window_stats"
			case "catalog":
				selected.Metrics[0].Limitations[0] += " changed"
			case "aliases":
				aliases = map[string]string{"new": "window_stats"}
			case "negative", "past_end", "noncanonical":
				parts := strings.Split(input.Cursor, ".")
				parts[2] = map[string]string{"negative": "-1", "past_end": "99999", "noncanonical": "01"}[mode]
				input.Cursor = strings.Join(parts, ".")
			case "garbage":
				input.Cursor = "not-a-catalog-cursor"
			}
			if _, err := marshalTraceCapabilitiesPage(selected, aliases, input, types.ToolDocumentationMaxBytes); err == nil {
				t.Fatal("invalid continuation silently changed selection")
			}
		})
	}
	result, err := (&TraceCapabilities{}).Execute(nil, json.RawMessage(`{"detail":true,"cursor":"invalid"}`))
	if err != nil || result.Success || result.Summary == "" || result.Handoff != nil {
		t.Fatalf("invalid public continuation should be explanatory, not an empty failed handoff: %v %+v", err, result)
	}
}

func TestTraceCapabilitiesPageNeverClipsOneContract(t *testing.T) {
	catalog, _ := tracequery.TraceCapabilities("cpu_state_frequency", true)
	catalog.Metrics[0].Limitations[0] = strings.Repeat("qualification ", types.ToolDocumentationMaxBytes)
	if _, err := marshalTraceCapabilitiesPage(catalog, nil, traceCapabilitiesInput{View: "cpu_state_frequency", Detail: true}, types.ToolDocumentationMaxBytes); err == nil {
		t.Fatal("oversized single metric was clipped")
	}
	compact, _ := tracequery.TraceCapabilities("", false)
	if _, err := marshalTraceCapabilitiesPage(compact, nil, traceCapabilitiesInput{}, 100); err == nil {
		t.Fatal("oversized catalog descriptors were clipped")
	}
}
