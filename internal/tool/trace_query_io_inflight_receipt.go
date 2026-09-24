package tool

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// One native population supplies summary, actual paired endpoints and the
// depth series. Presentation limits never become an arithmetic population.
func traceQueryIOInFlightReceipt(r types.ObservationRecord, stats *tracequery.IOInFlightStats, group tracequery.IOInFlightGroup) string {
	if group.AcceptedPairCount != len(group.Members)+group.OmittedMembers+group.MemberWitnessUnavailableCount {
		return "" // Do not publish contradictory member accounting.
	}
	// Twelve significant digits suppress binary subtraction noise while
	// preserving small non-zero quantities. Native JSON keeps full precision.
	f := func(v float64) string { return strconv.FormatFloat(v, 'g', 12, 64) }
	label := fmt.Sprintf("IO requests — %s / %s / %s / %s", group.Layer, group.EndpointFamily, group.Dev, group.Operation)
	window := "Continuous time window unavailable (not a measured zero)"
	if stats.Window != nil {
		window = fmt.Sprintf("Window [%s, %s) seconds", traceQueryDisplaySeconds(stats.Window.StartTs), traceQueryDisplaySeconds(stats.Window.EndTs))
	}
	notes := []string{window, "Source: " + group.SourcePath,
		"All issuing threads; successfully paired requests only. Request residence is not thread waiting or response delay. Independent dependency evidence is required for causal attribution.",
		"Request layers may describe the same IO operation: do not add them. Capture completeness is unknown.",
		fmt.Sprintf("Query groups: %d; groups not displayed: %d. Query target PID (%d) does not filter this all-thread account.", stats.GroupCount, stats.OmittedGroups, stats.QueryPID)}
	if stats.LineStart > 0 || stats.LineEnd > 0 {
		notes = append(notes, fmt.Sprintf("Query line range: %d–%d; line selection takes precedence over time parameters.", stats.LineStart, stats.LineEnd))
	}
	for _, coverage := range stats.Coverage {
		if (group.Layer == "block" && coverage.Family == "block") || (group.Layer != "block" && coverage.Family == "storage") {
			notes = append(notes, fmt.Sprintf("Whole pairing family %s (not this device alone): accepted pairs %d; unpaired starts %d; unpaired completions %d; ambiguous cohorts %d; pairing-suppressed rows %d; rejected rows %d; unresolved sources %d.",
				coverage.Family, coverage.AcceptedPairCount, coverage.UnpairedStartCount, coverage.UnpairedDoneCount,
				coverage.AmbiguousCohortCount, coverage.PairingSuppressedCount, coverage.RejectedEndpointRows, coverage.UnresolvedSources))
		}
	}
	table := func(view types.RuntimeMeasurementView, columns []string, rows [][]string, extra ...string) types.RuntimeMeasurementTable {
		return types.RuntimeMeasurementTable{ObservationID: r.ID, View: view, Label: label,
			Columns: columns, Rows: rows, Notes: append(append([]string(nil), notes...), extra...)}
	}
	values := []string{"unavailable", "unavailable", "unavailable", "unavailable"}
	if v := group.Values; v != nil {
		values = []string{strconv.Itoa(v.PeakRequests), f(v.MeanRequests), f(v.BusyMs), f(v.RequestMs)}
	}
	values = append(values, strconv.Itoa(group.AcceptedPairCount), strconv.Itoa(group.IssueCount))
	summary := table(types.RuntimeMeasurementSummary,
		[]string{"Peak concurrent requests", "Mean concurrent requests", "Busy time (ms)", "Request-time area (request·ms)", "Accepted complete pairs", "Starts inside query"},
		[][]string{values}, "Starts inside the query are arrivals, not paired-request count. Mean includes idle time; busy time is a union; request-time area is not wall-clock delay.")
	if group.Values == nil {
		if stats.Window == nil {
			summary.Notes = append(summary.Notes, "No continuous time denominator is established; concurrency and time measures are unavailable.")
		} else {
			summary.Notes = append(summary.Notes, "The time window is known, but no usable complete-pair measurements were established for this group; unavailable is not measured zero.")
		}
	}
	var memberRows [][]string
	for _, m := range group.Members {
		if m.SourcePath != group.SourcePath || m.IssueLocalLine <= 0 || m.CompleteLocalLine <= 0 || m.ID == "" {
			return ""
		}
		contribution := "unavailable"
		intersection := "unavailable"
		if m.WindowContributionMs != nil {
			contribution = f(*m.WindowContributionMs)
			intersection = "no overlap"
		}
		if w := m.WindowContribution; w != nil {
			intersection = "[" + traceQueryDisplaySeconds(w.StartTs) + ", " + traceQueryDisplaySeconds(w.EndTs) + ")"
		}
		memberRows = append(memberRows, []string{traceThreadLabel(m.IssueThread), traceThreadLabel(m.CompleteThread),
			strconv.Itoa(m.IssueLocalLine), strconv.Itoa(m.CompleteLocalLine),
			traceQueryDisplaySeconds(m.ActualStartTs), traceQueryDisplaySeconds(m.ActualEndTs), intersection, contribution})
	}
	members := table(types.RuntimeMeasurementMembers,
		[]string{"Issuing thread", "Completing thread", "Issue source line", "Completion source line", "Actual start (s)", "Actual end (s)", "Window intersection (s)", "In-window residence (ms)"}, memberRows,
		fmt.Sprintf("Accepted complete pairs: %d = displayed witnesses %d + display-limit omissions %d + unavailable endpoint witnesses %d. Statistics use all accepted pairs, not just these rows.", group.AcceptedPairCount, len(group.Members), group.OmittedMembers, group.MemberWitnessUnavailableCount),
		"Actual endpoints are not clipped; only in-window residence is clipped. A completing thread is not automatically the thread that woke the issuer.")
	var timelineRows [][]string
	for _, s := range group.Segments {
		timelineRows = append(timelineRows, []string{traceQueryDisplaySeconds(s.StartTs), traceQueryDisplaySeconds(s.EndTs), strconv.Itoa(s.Requests)})
	}
	timeline := table(types.RuntimeMeasurementTimeline, []string{"Start inclusive (s)", "End exclusive (s)", "Concurrent requests"}, timelineRows,
		fmt.Sprintf("Displayed segments: %d; omitted segments: %d. Summary uses the full series. Equal-depth segments may merge across a change of members; do not infer segment membership from depth alone.", len(group.Segments), group.OmittedSegments))
	publication := types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef,
		Tables: []types.RuntimeMeasurementTable{summary, members, timeline}}
	data, err := json.Marshal(publication)
	if err != nil {
		return ""
	}
	return types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)
}

// Finalize only receipts minted by this query after physical clock/source
// provenance and capture-read stamps have completed. This is not a consumer
// repair of mismatched evidence; downstream decoding remains exact.
func traceQueryFinalizeMeasurementSources(result *types.ToolResult) {
	if result == nil || !result.Success {
		return
	}
	for i := range result.Observations {
		r := &result.Observations[i]
		if !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Predicate != "io_inflight" {
			continue
		}
		for j, note := range r.RichNotes {
			raw, ok := strings.CutPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=")
			if !ok {
				continue
			}
			var p types.RuntimeMeasurementPublication
			if json.Unmarshal([]byte(raw), &p) != nil || p.ObservationID != r.ID ||
				p.Source.Path != r.SourceRef.Path || p.Source.PayloadRef != r.SourceRef.PayloadRef || p.Source.QueryScopeID != r.SourceRef.QueryScopeID {
				continue
			}
			p.Source = r.SourceRef
			if encoded, err := json.Marshal(p); err == nil {
				r.RichNotes[j] = types.TraceNoteKeyRuntimeMeasurement + "=" + string(encoded)
			}
		}
	}
}
