package tool

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceQueryIOActivityLabel(g tracequery.IOActivityGroup) string {
	family := map[string]string{"block_rq": "RQ", "block_bio": "BIO", "mmc_request": "MMC", "f2fs_sync_file": "F2FS file sync", "f2fs_direct_io": "F2FS direct IO", "f2fs_write": "F2FS write"}[g.EndpointFamily]
	if family == "" {
		family = "IO endpoints"
	}
	phase := map[string]string{"start": "starts", "done": "completions"}[g.Phase]
	caliber := map[string]string{"request_bytes": "requested bytes", "sector_bytes": "sectors × 512 bytes", "transferred_bytes": "reported transferred bytes", "copied_bytes": "reported copied bytes", "unspecified": "size not reported"}[g.ByteCaliber]
	return fmt.Sprintf("%s %s — device %s — %s", family, phase, g.Dev, caliber)
}

func traceQueryIOActivityValueRow(label string, v tracequery.IOActivityValues, rates *tracequery.IOActivityRates) []string {
	eventRate, byteRate := "unavailable", "unavailable"
	if rates != nil {
		eventRate, byteRate = traceQueryIOActivityNumber(&rates.EventsPerSecond), traceQueryIOActivityNumber(rates.KnownBytesPerSecond)
	}
	return []string{label, strconv.Itoa(v.EventCount), strconv.Itoa(v.KnownByteEventCount), traceQueryIOActivityBytes(v.KnownBytes), traceQueryIOActivityNumber(v.KnownSizeMeanBytes), eventRate, byteRate}
}

func traceQueryIOActivityReceipt(r types.ObservationRecord, s *tracequery.IOActivityStats, g tracequery.IOActivityGroup) string {
	window := "Continuous time window unavailable; rates are not measured zero."
	if s.Window != nil {
		window = fmt.Sprintf("Window [%s, %s) seconds", traceQueryDisplaySeconds(s.Window.StartTs), traceQueryDisplaySeconds(s.Window.EndTs))
		if s.Window.EndInclusive {
			window = fmt.Sprintf("Observed capture extent [%s, %s] seconds; includes the final endpoint. This is not an explicit half-open query with the same bounds.", traceQueryDisplaySeconds(s.Window.StartTs), traceQueryDisplaySeconds(s.Window.EndTs))
		}
	}
	notes := []string{window, "Source: " + g.SourcePath,
		"All issuing threads; counts are independently observed endpoint events, not unique logical requests. Missing completions or ambiguous pairing do not remove independently admitted starts; endpoints outside the selected range do not count.",
		"Do not add sources, layers, phases or byte calibers. Activity does not prove target waiting, response impact, hardware bandwidth or root cause. Capture completeness is unknown.",
		fmt.Sprintf("Query target PID %d is not an issuer filter. Groups %d; undisplayed groups %d. Supported endpoints %d; rejected endpoints %d; unresolved sources %d (query-wide coverage, not this group alone).", s.QueryPID, s.GroupCount, s.OmittedGroups, s.Coverage.SupportedEndpointCount, s.Coverage.RejectedEndpointCount, s.Coverage.UnresolvedSourceCount)}
	if s.LineStart > 0 || s.LineEnd > 0 {
		notes = append(notes, fmt.Sprintf("Lines %d–%d take precedence over time parameters; no wall-clock denominator is inferred.", s.LineStart, s.LineEnd))
	}
	table := func(view types.RuntimeMeasurementView, columns []string, rows [][]string, extra ...string) types.RuntimeMeasurementTable {
		return types.RuntimeMeasurementTable{ObservationID: r.ID, View: view, Label: traceQueryIOActivityLabel(g), Columns: columns, Rows: rows, Notes: append(append([]string(nil), notes...), extra...)}
	}
	rows := [][]string{traceQueryIOActivityValueRow("all operations", g.Values, g.Rates)}
	for _, d := range g.Directions {
		rows = append(rows, traceQueryIOActivityValueRow(d.Direction, d.Values, d.Rates))
	}
	summary := table(types.RuntimeMeasurementSummary,
		[]string{"Operation", "Endpoint events", "Known-size events", "Known bytes", "Mean known size (B)", "Events/s", "Known bytes/s"}, rows,
		"Rates use the entire wall-clock window including idle time, not summed request durations. Known bytes include known-size events only; missing/invalid/overflow sizes are not zero or estimated. Partial-size byte rates are not full throughput.")
	addStatus := func(label string, v tracequery.IOActivityValues) {
		summary.Notes = append(summary.Notes, fmt.Sprintf("%s size coverage: known %d, unknown %d, invalid %d, overflow %d; sum overflow %t.", label, v.KnownByteEventCount, v.UnknownByteEventCount, v.InvalidByteEventCount, v.OverflowByteEventCount, v.BytesOverflow))
	}
	addStatus("All operations", g.Values)
	for _, d := range g.Directions {
		addStatus(d.Direction, d.Values)
	}
	if rw := g.ReadWrite; rw != nil {
		summary.Notes = append(summary.Notes, fmt.Sprintf("Read/write event denominator %d; read fraction %s; write fraction %s. Known read/write byte denominator %s B; read fraction %s; write fraction %s. Fractions are 0–1 and exclude other operations; byte fractions describe known bytes only, not all traffic.", rw.EventDenominator, traceQueryIOActivityNumber(rw.ReadEventShare), traceQueryIOActivityNumber(rw.WriteEventShare), traceQueryIOActivityBytes(rw.KnownByteDenominator), traceQueryIOActivityNumber(rw.ReadKnownByteShare), traceQueryIOActivityNumber(rw.WriteKnownByteShare)))
	}
	var sizes [][]string
	for _, d := range g.Directions {
		for _, band := range d.Values.SizeBuckets {
			end := "unbounded"
			if band.MaxBytes != nil {
				end = strconv.FormatUint(*band.MaxBytes, 10)
			}
			sizes = append(sizes, []string{d.Direction, strconv.FormatUint(band.MinBytes, 10), end, strconv.Itoa(band.Count)})
		}
	}
	distribution := table(types.RuntimeMeasurementDistribution, []string{"Operation", "Size lower bound inclusive (B)", "Size upper bound exclusive (B)", "Known-size events"}, sizes,
		"Bands cover known-size events only, with inclusive lower and exclusive upper bounds. Zero size is known; unknown/invalid/overflow sizes are excluded, not assigned to the smallest band. Size alone does not prove random or sequential IO.")
	var buckets [][]string
	for _, b := range g.Buckets {
		prefix := []string{traceQueryDisplaySeconds(b.Window.StartTs), traceQueryDisplaySeconds(b.Window.EndTs)}
		buckets = append(buckets, append(append([]string(nil), prefix...), traceQueryIOActivityValueRow("all operations", b.Values, b.Rates)...))
		for _, d := range b.Directions {
			buckets = append(buckets, append(append([]string(nil), prefix...), traceQueryIOActivityValueRow(d.Direction, d.Values, d.Rates)...))
		}
	}
	endColumn := "End exclusive (s)"
	if s.Window != nil && s.Window.EndInclusive {
		endColumn = "End (s; capture end inclusive)"
	}
	timeline := table(types.RuntimeMeasurementTimeline,
		[]string{"Start inclusive (s)", endColumn, "Operation", "Endpoint events", "Known-size events", "Known bytes", "Mean known size (B)", "Events/s", "Known bytes/s"}, buckets,
		fmt.Sprintf("Bucket width %g ms; total buckets %d; displayed %d; omitted %d. Summary uses the full window, not only displayed buckets. No peak across the full window may be inferred from an omitted tail.", s.BucketMs, g.BucketCount, len(g.Buckets), g.OmittedBuckets),
		"Idle buckets remain present for observed groups. Each bucket rate uses its actual wall-clock width, including a shorter final bucket; do not average only occupied buckets or treat bucket counts as per-second rates.")
	if g.BucketsUnavailableReason != "" {
		timeline.Notes = append(timeline.Notes, "Time buckets unavailable; no zero-rate timeline or full-window peak is established.")
	}
	p := types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []types.RuntimeMeasurementTable{summary, distribution, timeline}}
	data, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)
}
