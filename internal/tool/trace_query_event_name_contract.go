package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const traceQueryEventNameTeaching = "event_types selects normalized categories (legacy aliases may span multiple raw event names, including RQ/BIO). For exact parser-retained event names use event_names, a case-sensitive OR list; it is ANDed with other filters. For trace markers the event name is the carrier (for example tracing_mark_write), not the span label; use pattern/span_name for labels."

func traceQueryEventNameSchema(schema string) string {
	// Keep the existing category aliases stable; do not silently reinterpret a
	// category as a raw name. Both parameter faces carry their distinct meaning.
	categoryTeaching, _ := json.Marshal(traceQueryEventNameTeaching + " Normalized category filters such as ")
	schema = strings.Replace(schema, "Optional event filters such as ", string(categoryTeaching[1:len(categoryTeaching)-1]), 1)
	property, _ := json.Marshal(map[string]any{
		"type": "array", "items": map[string]string{"type": "string"}, "maxItems": tracequery.EventSearchNameLimit,
		"description": "event_search only: exact case-sensitive parser-retained Event.Name OR set, preserving name bytes. Does not search payloads or expand categories. AND with event_types, patterns, identity and query bounds. Omit event_types when only original names are wanted; unknown names match zero rows. " + traceQueryEventNameTeaching,
	})
	return strings.Replace(schema, `"event_types":`, `"event_names": `+string(property)+",\n    "+`"event_types":`, 1)
}

func writeTraceEventFilterAndIONavigation(b *strings.Builder, result tracequery.Result, p traceQueryParams, source string) {
	if result.View != "event_search" {
		return
	}
	if len(p.EventTypes.Strings()) > 0 || len(p.EventNames) > 0 {
		fmt.Fprintf(b, "event_filter_contract event_types=%s event_names=%s: %s\n",
			traceQueryNameJSON(p.EventTypes.Strings()), traceQueryNameJSON(p.EventNames), traceQueryEventNameTeaching)
	}
	if !traceMarkerNavigationCatalogHasMetric("window_stats", "io_activity") {
		return
	}
	for _, event := range result.Events {
		if !tracequery.HasIOActivityEndpoint(event.Event) {
			continue
		}
		// This is a copyable advisory query, never an executed call. Original
		// caller bounds are used, not lookup-tolerant or matched-row envelopes.
		next := map[string]any{"view": "window_stats"}
		if p.Path != "" {
			next["path"] = p.Path
		} else {
			next["source"] = source
		}
		if p.TimeStart.Set() {
			next["time_start"] = traceQuerySecondParamString(p.TimeStart)
		}
		if p.TimeEnd.Set() {
			next["time_end"] = traceQuerySecondParamString(p.TimeEnd)
		}
		if p.LineStart.Int() > 0 {
			next["line_start"] = p.LineStart.Int()
		}
		if p.LineEnd.Int() > 0 {
			next["line_end"] = p.LineEnd.Int()
		}
		if p.BucketMs.Float64() > 0 {
			next["bucket_ms"] = p.BucketMs.Float64()
		}
		encoded, _ := json.Marshal(next)
		fmt.Fprintf(b, "io_query_navigation role=navigation_only section=io_activity optional_next_trace_query=%s\n", encoded)
		b.WriteString("For observed IO endpoint counts, byte totals, rates, size and read/write distributions, inspect window_stats.io_activity. Event search rows are discovery, not those measurements or completed request pairs. Reuse the original source and time/line bounds above, not the matched-event envelope or lookup tolerance. Search name/type/pattern filters are intentionally not carried into this measurement: inspect its separate source/layer/family/phase/device groups; never sum RQ, BIO, MMC and filesystem layers or start/completion populations. IO activity covers all issuers in the selected range, not target-thread waiting or a causal chain. Line-only ranges have no continuous-window rates; absent sizes are unknown, not zero. No extra call is required and no causal authority is granted.\n")
		return
	}
}

func traceQueryNameJSON(names []string) string {
	if len(names) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(names)
	return string(b)
}
