package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Event discovery proves rows, not their pairing or measurements. Keep the
// next measurement surface near those rows, rather than asking the model to
// reconstruct a stack from a bounded event listing. This is advisory only: it
// neither executes a query nor changes observations, completion or evidence.
func writeTraceMarkerQueryNavigation(b *strings.Builder, result tracequery.Result) {
	if result.View != "event_search" {
		return
	}
	var syncEndpoints, asyncEndpoints bool
	for _, event := range result.Events {
		if event.Type != tracequery.EventTraceMark {
			continue
		}
		switch event.SpanAction {
		case "B", "E":
			syncEndpoints = true
		case "S", "F":
			asyncEndpoints = true
		}
	}
	if !syncEndpoints && !asyncEndpoints {
		return
	}
	var views []string
	if syncEndpoints && traceMarkerNavigationCatalogHasMetric("window_stats", "business_tree") {
		views = append(views, "window_stats")
	}
	if traceMarkerNavigationCatalogHasMetric("span_window", "trace_spans") {
		views = append(views, "span_window")
	}
	if len(views) == 0 {
		return
	}
	fmt.Fprintf(b, "marker_query_navigation role=navigation_only recommended_views=%s\n", strings.Join(views, ","))
	b.WriteString("Marker endpoints are event inventory, not proof of completed pairing, parent/child relationships, inclusive/self duration, or scheduler-state costs. ")
	for _, view := range views {
		switch view {
		case "window_stats":
			b.WriteString("For synchronous business nesting and per-layer costs, use view=\"window_stats\" and its business_tree section; it pairs same-source, same-thread B/E and measures inclusive/self intervals and available states. ")
		case "span_window":
			b.WriteString("For a named synchronous or asynchronous span, use view=\"span_window\" with span_name to inspect pairing and coverage. ")
		}
	}
	if asyncEndpoints {
		b.WriteString("S/F markers do not establish a synchronous child or the asynchronous work's CPU time; do not insert them into a B/E tree or subtract them from synchronous self time. ")
	}
	b.WriteString("Reuse the original query artifact(s), target and explicit time/line bounds. A matched-event envelope is not a requested time window; when bounds are absent, choose the needed scope explicitly rather than inventing endpoints. Do not pair across physical sources or join same-name threads. Inspect the follow-up result's closure, omissions and state coverage; missing state data is not zero. This navigation does not require another call or grant causal authority.\n")
}

// Resolve routes through the existing catalog so navigation cannot advertise
// an invented view or promote catalog capability into capture availability.
func traceMarkerNavigationCatalogHasMetric(view, metric string) bool {
	catalog, err := tracequery.TraceCapabilities(view, false)
	if err != nil {
		return false
	}
	for _, entry := range catalog.Views {
		if entry.View != view {
			continue
		}
		for _, ref := range entry.MetricRefs {
			if ref == metric {
				return true
			}
		}
	}
	return false
}
