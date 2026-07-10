package tracequery

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	traceCounterOwnerPayloadProcess = "payload_process"
	traceCounterOwnerGlobal         = "global"

	traceCounterBaselineInWindowFirstSample = "in_window_first_sample"
	traceCounterUnitUnknown                 = "unknown"

	traceCounterIssueSampleCap = 3
	traceCounterSeriesBudget   = 8192
)

type traceCounterSample struct {
	ownerPID     int
	ownerRaw     string
	ownerScope   string
	name         string
	valueRaw     string
	trailingTag  string
	numericValue float64
	numericValid bool
	identityOK   bool
	issueReason  string
}

// parseTraceCounterSample turns the already-recognised EventTraceMark C row
// into the exact C| wire schema consumed by numeric aggregation. It reparses
// FieldText instead of trusting SpanPID's zero value because pid=0 is a valid
// explicit GLOBAL owner while a malformed/missing pid must fail closed.
//
// Accepted forms are deliberately closed:
//
//	C|pid|name|decimal
//	C|pid|name|decimal|<UPPER><digits>
//	C|pid|name|(unit)|decimal
//	C|pid|name|(unit)|decimal|<UPPER><digits>
//
// The second form is the Harmony track-tag shape (I38/M0538). Address-carved
// variants first pass through normalizeTraceMarkPayload and therefore enter
// this same grammar. Unknown extra fields are disclosed, never guessed into
// either the value or identity.
func parseTraceCounterSample(ev Event) traceCounterSample {
	var out traceCounterSample
	if ev.Type != EventTraceMark || ev.SpanAction != "C" {
		out.issueReason = "not_counter_mark"
		return out
	}

	payload := normalizeTraceMarkPayload(ev.FieldText)
	if !strings.HasPrefix(payload, "C|") {
		// Compatibility for package callers/tests which construct a typed Event
		// directly rather than going through ParseLine. A positive SpanPID has
		// explicit presence; zero cannot prove the wire carried pid=0 and is
		// therefore rejected instead of silently becoming a global counter.
		if ev.FieldText == "" && ev.SpanPID > 0 {
			payload = fmt.Sprintf("C|%d|%s|%s", ev.SpanPID, ev.SpanName, ev.SpanValue)
		} else {
			out.ownerRaw = strconv.Itoa(ev.SpanPID)
			out.name = strings.TrimSpace(ev.SpanName)
			out.valueRaw = strings.TrimSpace(ev.SpanValue)
			out.issueReason = "malformed_counter_fields"
			return out
		}
	}

	parts := strings.Split(payload, "|")
	if len(parts) >= 2 {
		out.ownerRaw = strings.TrimSpace(parts[1])
	}
	// Seed the compatibility inventory fields from the standard four-field
	// positions before applying the closed typed grammar below. Invalid rows
	// remain visible (and are explicitly counted by CounterQuality), while a
	// recognised pipe-unit/tag shape overwrites these with its authoritative
	// right-delimited name/value/tag interpretation.
	if len(parts) >= 3 {
		out.name = strings.TrimSpace(parts[2])
	}
	if len(parts) >= 4 {
		out.valueRaw = strings.TrimSpace(parts[3])
	}
	if len(parts) < 4 {
		out.issueReason = "malformed_counter_fields"
		return out
	}
	nameEnd, valueAt := 3, 3
	switch len(parts) {
	case 4:
		// Standard atrace counter.
	case 5:
		if isInstanceTag(strings.TrimSpace(parts[4])) {
			out.trailingTag = strings.TrimSpace(parts[4])
		} else if traceCounterEmbeddedUnitToken(parts[3]) {
			// Production Harmony witness: `Heap size |(KB)|205455`.
			// Only the exact parenthesized-unit grammar may extend the opaque
			// name; an arbitrary fifth field remains invalid rather than being
			// guessed into identity.
			nameEnd, valueAt = 4, 4
		} else {
			out.issueReason = "invalid_trailing_tag"
			return out
		}
	case 6:
		if !traceCounterEmbeddedUnitToken(parts[3]) || !isInstanceTag(strings.TrimSpace(parts[5])) {
			out.issueReason = "unsupported_extra_fields"
			return out
		}
		nameEnd, valueAt = 4, 4
		out.trailingTag = strings.TrimSpace(parts[5])
	default:
		out.issueReason = "unsupported_extra_fields"
		return out
	}
	out.name = strings.TrimSpace(strings.Join(parts[2:nameEnd], "|"))
	out.valueRaw = strings.TrimSpace(parts[valueAt])
	if !isAllDigits(out.ownerRaw) {
		out.issueReason = "invalid_owner_pid"
		return out
	}
	owner, err := strconv.ParseUint(out.ownerRaw, 10, strconv.IntSize)
	if err != nil || owner > uint64(^uint(0)>>1) {
		out.issueReason = "invalid_owner_pid"
		return out
	}
	out.ownerPID = int(owner)
	if out.ownerPID == 0 {
		out.ownerScope = traceCounterOwnerGlobal
	} else {
		out.ownerScope = traceCounterOwnerPayloadProcess
	}
	if out.name == "" {
		out.issueReason = "empty_counter_name"
		return out
	}
	out.identityOK = true
	if out.valueRaw == "" {
		out.issueReason = "empty_counter_value"
		return out
	}
	// The accepted numeric domain matches the precise carved-counter grammar:
	// finite plain decimal only. strconv.ParseFloat alone would admit NaN/Inf,
	// and those poison ordering plus JSON serialization.
	if !isAllNumeric(out.valueRaw) {
		out.issueReason = "non_decimal_or_non_finite_value"
		return out
	}
	value, err := strconv.ParseFloat(out.valueRaw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		out.issueReason = "non_decimal_or_non_finite_value"
		return out
	}
	out.numericValue = value
	out.numericValid = true
	return out
}

// traceCounterEmbeddedUnitToken is the precise compatibility gate for
// counter names whose producer writes a unit as a pipe-separated name suffix,
// e.g. `Heap size |(KB)|205455`. Parentheses and a small ASCII unit alphabet
// are structural; arbitrary extra fields never enter the counter identity.
func traceCounterEmbeddedUnitToken(raw string) bool {
	raw = strings.TrimSpace(raw)
	if len(raw) < 3 || raw[0] != '(' || raw[len(raw)-1] != ')' {
		return false
	}
	for i := 1; i < len(raw)-1; i++ {
		ch := raw[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || strings.ContainsRune("%/_ .-", rune(ch)) {
			continue
		}
		return false
	}
	return true
}

func traceCounterTypedLaneKey(ev Event) (string, bool) {
	sample := parseTraceCounterSample(ev)
	if !sample.identityOK || !sample.numericValid {
		return "", false
	}
	return strings.Join([]string{
		sample.ownerScope,
		strconv.Itoa(sample.ownerPID),
		sample.name,
		sample.trailingTag,
	}, "\x00"), true
}

type traceCounterSeriesKey struct {
	sourcePath  string
	ownerPID    int
	ownerScope  string
	name        string
	trailingTag string
}

type traceCounterSeriesAccumulator struct {
	row          TraceCounterDeltaSummary
	invalidValue bool
	firstRaw     string
	lastRaw      string
	lastCoord    traceCounterSourceCoordinate
	lastLine     int
}

type traceCounterSourceCoordinate struct {
	path      string
	localLine int
}

// traceCounterSourceCoordinateForLine mirrors the tracePairingSourceIdentity
// discipline (block/trace-mark/storage lanes). ENG audit #4c (§29.25 处置委托
// 2026-07-10): the old fallback failed OPEN — when a populated composite
// ledger could not resolve the row to exactly one physical artifact, the
// shared idx.Path became the series identity and lookalike counters from
// different artifacts could join one series, contradicting the identity
// promise documented on computeCounterDeltas. A populated provenance ledger
// that cannot resolve the row now fails closed; the single-file/path-less
// compatibility lanes are unchanged.
func traceCounterSourceCoordinateForLine(idx *Index, line int) (traceCounterSourceCoordinate, bool) {
	if idx != nil {
		spans := idx.ResolveArtifactSpans(line, line)
		if len(spans) == 1 {
			return traceCounterSourceCoordinate{path: spans[0].SourcePath, localLine: spans[0].LocalLineStart}, true
		}
		if len(idx.TraceArtifacts) > 0 {
			return traceCounterSourceCoordinate{}, false
		}
		return traceCounterSourceCoordinate{path: idx.Path, localLine: line}, true
	}
	return traceCounterSourceCoordinate{localLine: line}, true
}

// computeCounterDeltas aggregates only complete, finite numeric series. The
// first sample is an explicitly in-window baseline; index padding is context,
// not proof of the counter's state at the left boundary. Source path is part
// of the identity so a composite index can never join lookalike counters from
// different artifacts after canonical timestamp sorting.
func computeCounterDeltas(idx *Index, q Query, max int) ([]TraceCounterDeltaSummary, *TraceCounterQualitySummary) {
	if idx == nil {
		return nil, nil
	}
	quality := &TraceCounterQualitySummary{
		BaselinePolicy: traceCounterBaselineInWindowFirstSample,
		UnitPolicy:     "wire_schema_has_no_unit",
		SeriesBudget:   traceCounterSeriesBudget,
	}
	series := map[traceCounterSeriesKey]*traceCounterSeriesAccumulator{}
	issues := map[string]*TraceCounterIssueSummary{}
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) || ev.Type != EventTraceMark || ev.SpanAction != "C" {
			continue
		}
		quality.Rows++
		sample := parseTraceCounterSample(ev)
		coord, coordOK := traceCounterSourceCoordinateForLine(idx, ev.Line)
		if !coordOK {
			// ENG audit #4c: a populated provenance ledger that cannot map
			// this row to exactly one physical artifact fails closed — the
			// row is excluded from series identity and disclosed, never
			// merged under the shared composite path.
			quality.InvalidRows++
			appendTraceCounterIssueReason(issues, "source_identity_unresolved", sample, coord, ev.Line)
			continue
		}
		if !sample.identityOK {
			quality.InvalidRows++
			appendTraceCounterIssue(issues, sample, coord, ev.Line)
			continue
		}
		quality.ValidIdentityRows++
		if sample.numericValid {
			quality.NumericRows++
		} else {
			quality.NonNumericRows++
			appendTraceCounterIssue(issues, sample, coord, ev.Line)
		}
		key := traceCounterSeriesKey{
			sourcePath: coord.path, ownerPID: sample.ownerPID, ownerScope: sample.ownerScope,
			name: sample.name, trailingTag: sample.trailingTag,
		}
		acc := series[key]
		if acc == nil {
			if len(series) >= traceCounterSeriesBudget {
				quality.SeriesBudgetExceeded = true
				quality.OverflowRows++
				appendTraceCounterIssueReason(issues, "series_budget_exceeded", sample, coord, ev.Line)
				continue
			}
			acc = &traceCounterSeriesAccumulator{row: TraceCounterDeltaSummary{
				Thread:   threadRefFromEvent(ev),
				OwnerPID: sample.ownerPID, OwnerScope: sample.ownerScope,
				Name: sample.name, TrailingTag: sample.trailingTag, SourcePath: coord.path,
				Baseline: traceCounterBaselineInWindowFirstSample, UnitStatus: traceCounterUnitUnknown,
			}}
			series[key] = acc
		}
		if !sample.numericValid {
			acc.invalidValue = true
			continue
		}
		row := &acc.row
		if row.Samples == 0 {
			row.First = sample.numericValue
			row.Last = sample.numericValue
			row.Min = sample.numericValue
			row.Max = sample.numericValue
			row.FirstLine = ev.Line
			row.LastLine = ev.Line
			row.FirstLocalLine = coord.localLine
			row.LastLocalLine = coord.localLine
			acc.firstRaw = sample.valueRaw
		} else {
			row.Last = sample.numericValue
			row.LastLine = ev.Line
			row.LastLocalLine = coord.localLine
			if sample.numericValue < row.Min {
				row.Min = sample.numericValue
			}
			if sample.numericValue > row.Max {
				row.Max = sample.numericValue
			}
		}
		acc.lastRaw = sample.valueRaw
		acc.lastCoord = coord
		acc.lastLine = ev.Line
		row.Samples++
	}
	if quality.Rows == 0 {
		return nil, nil
	}

	quality.TotalSeries = len(series)
	quality.TotalSeriesStatus = "exact"
	if quality.SeriesBudgetExceeded {
		quality.TotalSeriesStatus = "lower_bound"
	}
	out := make([]TraceCounterDeltaSummary, 0, len(series))
	for _, acc := range series {
		if acc.invalidValue || acc.row.Samples == 0 {
			quality.SuppressedSeries++
			continue
		}
		delta := acc.row.Last - acc.row.First
		if math.IsNaN(delta) || math.IsInf(delta, 0) {
			quality.DerivedInvalidSeries++
			quality.SuppressedSeries++
			appendTraceCounterIssueReason(issues, "derived_delta_non_finite", traceCounterSample{
				ownerPID: acc.row.OwnerPID, ownerRaw: strconv.Itoa(acc.row.OwnerPID), ownerScope: acc.row.OwnerScope,
				name: acc.row.Name, valueRaw: "first=" + acc.firstRaw + ",last=" + acc.lastRaw,
				trailingTag: acc.row.TrailingTag, identityOK: true,
			}, acc.lastCoord, acc.lastLine)
			continue
		}
		acc.row.Delta = delta
		out = append(out, acc.row)
	}
	if quality.SeriesBudgetExceeded {
		// The unseen identities may contain a larger |delta| than every admitted
		// row. Publishing any top-N would therefore be an ordering claim over an
		// incomplete candidate universe. Keep the compatibility TraceCounters
		// inventory, but fail this entire derived face closed.
		quality.PublishedSeries = 0
		quality.SuppressedSeries = len(series)
		quality.TruncatedSeries = 0
		quality.Issues = sortedTraceCounterIssues(issues)
		return nil, quality
	}
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := math.Abs(out[i].Delta), math.Abs(out[j].Delta)
		if di != dj {
			return di > dj
		}
		if out[i].Samples != out[j].Samples {
			return out[i].Samples > out[j].Samples
		}
		if out[i].SourcePath != out[j].SourcePath {
			return out[i].SourcePath < out[j].SourcePath
		}
		if out[i].OwnerScope != out[j].OwnerScope {
			return out[i].OwnerScope < out[j].OwnerScope
		}
		if out[i].OwnerPID != out[j].OwnerPID {
			return out[i].OwnerPID < out[j].OwnerPID
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].TrailingTag != out[j].TrailingTag {
			return out[i].TrailingTag < out[j].TrailingTag
		}
		return out[i].FirstLine < out[j].FirstLine
	})
	if max > 0 && len(out) > max {
		quality.TruncatedSeries = len(out) - max
		out = out[:max]
	}
	quality.PublishedSeries = len(out)
	quality.Issues = sortedTraceCounterIssues(issues)
	return out, quality
}

func appendTraceCounterIssue(dst map[string]*TraceCounterIssueSummary, sample traceCounterSample, coord traceCounterSourceCoordinate, line int) {
	reason := sample.issueReason
	if reason == "" {
		reason = "invalid_counter_sample"
	}
	appendTraceCounterIssueReason(dst, reason, sample, coord, line)
}

func appendTraceCounterIssueReason(dst map[string]*TraceCounterIssueSummary, reason string, sample traceCounterSample, coord traceCounterSourceCoordinate, line int) {
	issue := dst[reason]
	if issue == nil {
		issue = &TraceCounterIssueSummary{Reason: reason}
		dst[reason] = issue
	}
	issue.Count++
	if len(issue.Samples) >= traceCounterIssueSampleCap {
		return
	}
	issue.Samples = append(issue.Samples, TraceCounterIssueSample{
		Line: line, LocalLine: coord.localLine, SourcePath: coord.path,
		OwnerRaw: clampString(sample.ownerRaw, 40), Name: clampString(sample.name, 120),
		Value: clampString(sample.valueRaw, 120), TrailingTag: clampString(sample.trailingTag, 40),
	})
}

func sortedTraceCounterIssues(src map[string]*TraceCounterIssueSummary) []TraceCounterIssueSummary {
	if len(src) == 0 {
		return nil
	}
	out := make([]TraceCounterIssueSummary, 0, len(src))
	for _, issue := range src {
		out = append(out, *issue)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reason < out[j].Reason })
	return out
}
