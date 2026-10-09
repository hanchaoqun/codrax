package tracequery

import (
	"container/heap"
	"encoding/json"
	"math"
	"math/big"
	"sort"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// Only this source protocol permits finite numeric REAL values to mean Hz.
// It does not change the general SQL integer/type authority of the carrier.
func preferredFrameRateValue(s tracewire.ProcessMeasureScalar) (string, bool) {
	var value string
	if v, ok := s.Integer(); ok {
		value = strconv.FormatInt(v, 10)
	} else if s.Status == "invalid_storage" && s.StorageClass == "real" {
		v, err := strconv.ParseFloat(s.Value, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return "", false
		}
		value = strconv.FormatFloat(v, 'g', -1, 64)
	} else {
		return "", false
	}
	n, ok := new(big.Rat).SetString(value)
	if !ok || n.Sign() <= 0 {
		return "", false
	}
	if n.IsInt() {
		return n.Num().String(), true
	}
	return value, true
}

type preferredRateObservation struct {
	start, end int64
	rate       string
	line       int
}
type preferredRateLane struct {
	series    PreferredFrameRateSeries
	intervals []preferredRateObservation
}

func buildPreferredFrameRate(idx *Index, q Query) *PreferredFrameRateResult {
	p := &PreferredFrameRateResult{Status: "unavailable", SourcePath: idx.Path, Window: ProcessMeasurementsWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}, TargetPID: q.PID, TargetScope: q.TargetScope, Caveats: []string{PreferredFrameRateTeaching}}
	fail := func(reason string) *PreferredFrameRateResult {
		p.Series = nil
		p.Observations = nil
		p.TotalSeries = 0
		p.OmittedSeries = 0
		p.TotalObservations = 0
		p.OmittedObservations = 0
		p.UnpositionedRows = 0
		p.Caveats = append(p.Caveats, reason)
		return p
	}
	if !resourceStackSingleSource(idx) || idx.ProcessMeasureMalformed > 0 {
		return fail("requires_complete_valid_identity_mapped_single_source")
	}
	if q.timeStartBackfilled || q.timeEndBackfilled {
		return fail("requires_explicit_window_no_capture_extent_inference")
	}
	start, sok := processMeasurementNS(q.TimeStart)
	end, eok := processMeasurementNS(q.TimeEnd)
	if !sok || !eok || end <= start || start < 0 && end > math.MaxInt64+start {
		return fail("requires_ordered_representable_nanosecond_window")
	}
	if q.TargetScope != TargetScopeProcess || q.PID < 0 || q.Thread != "" || q.ThreadInput != "" || q.ThreadPIDInferred {
		return fail("requires_process_ownership_not_thread")
	}
	if q.Pattern != "" || len(q.Patterns) > 0 || q.SpanName != "" || len(q.EventTypes) > 0 || len(q.EventNames) > 0 || len(q.EventFieldFilters) > 0 || len(q.TraceMarkActions) > 0 {
		return fail("requires_time_line_and_process_selectors_only")
	}
	lanes := map[string]*preferredRateLane{}
	seen := map[int64]bool{}
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return nil
		}
		if ev.Type != EventProcessMeasureInterval {
			continue
		}
		if ev.PluginFields == nil || ev.PluginFields.ProcessMeasure == nil || !ev.PluginFields.ProcessMeasure.Valid() {
			return fail("invalid_process_measure_record")
		}
		r := *ev.PluginFields.ProcessMeasure
		if seen[r.RowID] {
			return fail("duplicate_process_measure_row_identity")
		}
		seen[r.RowID] = true
		if !r.NameKnown || r.Name != "H:PreferredFrameRate" || q.LineStart > 0 && ev.Line < q.LineStart || q.LineEnd > 0 && ev.Line > q.LineEnd || q.PID > 0 && (r.PID == nil || *r.PID != q.PID) {
			continue
		}
		selection, left, right := processMeasurementSelection(r, start, end, false)
		_, positioned := r.StartNS.Integer()
		if positioned && selection == "" {
			continue
		}
		source, local := idx.Path, ev.Line
		if len(idx.TraceArtifacts) > 0 {
			spans := idx.ResolveArtifactSpans(ev.Line, ev.Line)
			if len(spans) != 1 {
				return fail("source_row_mapping_unavailable")
			}
			source, local = spans[0].SourcePath, spans[0].LocalLineStart
		}
		id := PreferredFrameRateSeries{SourcePath: source, IPID: r.IPID.Value, FilterID: r.FilterID.Value, PID: r.PID, ProcessName: r.ProcessName, OwnerStatus: r.OwnerStatus}
		if r.OwnerStatus != "known" {
			id.UnknownOwnerRowID = strconv.FormatInt(r.RowID, 10)
		}
		keyBytes, _ := json.Marshal(id)
		key := string(keyBytes)
		lane := lanes[key]
		if lane == nil {
			lane = &preferredRateLane{series: id}
			lanes[key] = lane
		}
		if !positioned {
			p.UnpositionedRows++
			lane.series.UnpositionedRows++
			continue
		}
		p.TotalObservations++
		lane.series.Observations++
		if len(p.Observations) < ProcessMeasurementsLimit {
			p.Observations = append(p.Observations, ProcessMeasurementRow{source, ev.Line, local, r, left, right, selection, "Hz (PreferredFrameRate protocol; invalid values remain unknown)"})
		} else {
			p.OmittedObservations++
		}
		if left != nil && right != nil && *right > *left {
			rate, _ := preferredFrameRateValue(r.Value)
			lane.intervals = append(lane.intervals, preferredRateObservation{*left, *right, rate, local})
		}
	}
	keys := make([]string, 0, len(lanes))
	for key := range lanes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	p.TotalSeries = len(keys)
	for _, key := range keys {
		if q.runCancel.tick() {
			return nil
		}
		lane := lanes[key]
		if !preferredFrameRateSweep(lane, start, end, q.runCancel.tick) {
			return nil
		}
		if len(p.Series) < ViewCapacityFor(ViewPreferredFrameRate).ClampLimit(q.Limit) {
			p.Series = append(p.Series, lane.series)
		} else {
			p.OmittedSeries++
		}
	}
	p.Status = "available"
	if q.LineStart > 0 || q.LineEnd > 0 {
		p.Caveats = append(p.Caveats, "Line restrictions select only those source observations; uncovered time means no accepted interval from the selected lines, not absence elsewhere in the capture.")
	}
	if len(keys) == 0 {
		p.Caveats = append(p.Caveats, "No matching PreferredFrameRate owner/filter observations; no fallback, no rate or coverage is inferred.")
	}
	return p
}

func preferredFrameRateSweep(lane *preferredRateLane, start, end int64, canceled func() bool) bool {
	type edge struct {
		at    int64
		index int
		add   bool
	}
	edges := make([]edge, 0, len(lane.intervals)*2+2)
	edges = append(edges, edge{at: start, index: -1}, edge{at: end, index: -1})
	for i, r := range lane.intervals {
		edges = append(edges, edge{r.start, i, true}, edge{r.end, i, false})
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].at < edges[j].at })
	active := map[int]bool{}
	rates := map[string]int{}
	unknown := 0
	distribution := map[string]*PreferredFrameRateDistribution{}
	lineHeap := &preferredRateLineHeap{}
	var prior *PreferredFrameRateInterval
	flush := func() {
		if prior == nil {
			return
		}
		lane.series.TotalIntervals++
		if len(lane.series.Timeline) < PreferredFrameRateDetailLimit {
			lane.series.Timeline = append(lane.series.Timeline, *prior)
		} else {
			lane.series.OmittedIntervals++
		}
		prior = nil
	}
	for i := 0; i < len(edges); {
		if canceled() {
			return false
		}
		at := edges[i].at
		for i < len(edges) && edges[i].at == at {
			e := edges[i]
			i++
			if e.index < 0 {
				continue
			}
			r := lane.intervals[e.index]
			if e.add {
				active[e.index] = true
				heap.Push(lineHeap, e.index)
				if r.rate == "" {
					unknown++
				} else {
					rates[r.rate]++
				}
			} else {
				delete(active, e.index)
				if r.rate == "" {
					unknown--
				} else {
					rates[r.rate]--
					if rates[r.rate] == 0 {
						delete(rates, r.rate)
					}
				}
			}
		}
		if i == len(edges) {
			break
		}
		next := edges[i].at
		if next <= at {
			continue
		}
		row := PreferredFrameRateInterval{StartNS: at, EndNS: next, State: "unobserved"}
		switch {
		case len(rates) > 1:
			row.State = "conflict"
			lane.series.ConflictDurationNS += next - at
		case unknown > 0:
			row.State = "unknown_value"
			lane.series.UnknownValueDurationNS += next - at
		case len(rates) == 1:
			row.State = "known"
			for rate := range rates {
				row.RateHz = rate
			}
			lane.series.KnownDurationNS += next - at
		default:
			lane.series.UnobservedDurationNS += next - at
		}
		row.TotalSourceLines = len(active)
		for lineHeap.Len() > 0 && !active[(*lineHeap)[0]] {
			heap.Pop(lineHeap)
		}
		if lineHeap.Len() > 0 {
			row.SourceLines = []int{lane.intervals[(*lineHeap)[0]].line}
			row.OmittedSourceLines = row.TotalSourceLines - 1
		}
		// Merge equal state/rate only when provenance is unchanged. Distribution
		// independently unions adjacent known equal-rate pieces across source rows.
		same := prior != nil && prior.EndNS == at && prior.State == row.State && prior.RateHz == row.RateHz
		if row.State == "known" {
			d := distribution[row.RateHz]
			if d == nil {
				d = &PreferredFrameRateDistribution{RateHz: row.RateHz}
				distribution[row.RateHz] = d
			}
			d.DurationNS += next - at
			if !same {
				d.Intervals++
			}
		}
		if same && equalPreferredFrameRateLines(prior, row) {
			prior.EndNS = next
		} else {
			flush()
			prior = &row
		}
	}
	flush()
	for _, d := range distribution {
		d.WindowPercent = float64(d.DurationNS) / float64(end-start) * 100
		lane.series.Distribution = append(lane.series.Distribution, *d)
	}
	sort.Slice(lane.series.Distribution, func(i, j int) bool {
		a, b := lane.series.Distribution[i], lane.series.Distribution[j]
		if a.DurationNS != b.DurationNS {
			return a.DurationNS > b.DurationNS
		}
		return a.RateHz < b.RateHz
	})
	lane.series.TotalDistribution = len(lane.series.Distribution)
	if len(lane.series.Distribution) > PreferredFrameRateDetailLimit {
		lane.series.OmittedDistribution = len(lane.series.Distribution) - PreferredFrameRateDetailLimit
		lane.series.Distribution = lane.series.Distribution[:PreferredFrameRateDetailLimit]
	}
	return true
}

type preferredRateLineHeap []int

func (h preferredRateLineHeap) Len() int           { return len(h) }
func (h preferredRateLineHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h preferredRateLineHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *preferredRateLineHeap) Push(v any)        { *h = append(*h, v.(int)) }
func (h *preferredRateLineHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

func equalPreferredFrameRateLines(a *PreferredFrameRateInterval, b PreferredFrameRateInterval) bool {
	// Equal abbreviated samples do not prove equal complete active sets.
	if a.OmittedSourceLines > 0 || b.OmittedSourceLines > 0 {
		return false
	}
	if a.TotalSourceLines != b.TotalSourceLines || a.OmittedSourceLines != b.OmittedSourceLines || len(a.SourceLines) != len(b.SourceLines) {
		return false
	}
	for i := range a.SourceLines {
		if a.SourceLines[i] != b.SourceLines[i] {
			return false
		}
	}
	return true
}
