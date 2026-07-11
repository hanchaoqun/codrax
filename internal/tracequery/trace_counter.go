package tracequery

import (
	"fmt"
	"math"
	"math/big"
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
	// The kernel trace_marker path used by OpenHarmony and AOSP has a 1024-byte
	// record boundary. At that boundary completeness cannot be proven, so this
	// profile admits only payloads strictly below the cap. OpenHarmony's
	// separately captured app-file profile can write longer complete rows; it
	// is conservatively inventory-only until the input carries a typed profile
	// discriminator. Event.FieldText remains a bounded display/search copy.
	traceCounterPayloadMaxBytes = 1024
	// IEEE-754 binary64 represents every integer in this closed range exactly.
	// Larger native int64 counters stay visible as inventory but cannot enter
	// the legacy float64 summary without lying about endpoints or deltas.
	traceCounterMaxExactFloatInteger int64 = 1 << 53
)

type traceCounterSample struct {
	ownerPID     int
	ownerRaw     string
	ownerScope   string
	name         string
	valueRaw     string
	metadataRaw  string
	outputLevel  string
	tagBits      string
	numericValue float64
	numericValid bool
	identityOK   bool
	issueReason  string
}

// parseTraceCounterPayload is the single complete-payload authority for the
// C| wire schema consumed by Event JSON/search, inventory, ordering, namespace
// derivation and numeric aggregation.
//
// The exact action and owner stay left-delimited. The value and optional exact
// OpenHarmony output-level/tag-bits metadata are right-delimited; every field
// between owner and value is one opaque counter name, including empty or
// pipe-separated components. Neither OpenHarmony nor AOSP escapes `|` in the
// name:
//
//	C|pid|opaque[|opaque...]|decimal
//	C|pid|opaque[|opaque...]|decimal|<D|I|C|M><tag-bit-pairs>
//
// Address-carved variants first pass through normalizeTraceMarkPayload and
// therefore enter this same grammar. An unknown tail is the value, not a
// guessed metadata token; it must independently pass the finite plain-decimal
// and representability gates.
func parseTraceCounterPayload(payload string) traceCounterSample {
	var out traceCounterSample
	payload = normalizeTraceMarkPayload(payload)
	if !strings.HasPrefix(payload, "C|") {
		out.issueReason = "malformed_counter_fields"
		return out
	}
	if len(payload) >= traceCounterPayloadMaxBytes {
		out.issueReason = "counter_payload_too_long"
		return out
	}

	parts := strings.Split(payload, "|")
	if len(parts) >= 2 {
		out.ownerRaw = parts[1]
	}
	// Seed bounded compatibility inventory fields before applying the closed
	// typed grammar. Invalid rows remain visible and are explicitly counted by
	// CounterQuality; valid rows overwrite these with the authoritative
	// right-delimited interpretation.
	if len(parts) >= 3 {
		out.name = parts[2]
	}
	if len(parts) >= 4 {
		out.valueRaw = parts[3]
	}
	if len(parts) < 4 {
		out.issueReason = "malformed_counter_fields"
		return out
	}
	valueAt := len(parts) - 1
	if level, bits, ok := parseHarmonyTraceMetadata(parts[valueAt]); ok {
		out.metadataRaw = parts[valueAt]
		out.outputLevel = level
		out.tagBits = bits
		valueAt--
	}
	if valueAt < 3 {
		out.issueReason = "malformed_counter_fields"
		return out
	}
	name, nameOK := joinTraceCounterName(parts[2:valueAt])
	if nameOK {
		out.name = name
	} else {
		out.name = ""
	}
	out.valueRaw = parts[valueAt]
	if out.ownerRaw != strings.TrimSpace(out.ownerRaw) {
		out.issueReason = "invalid_owner_pid"
		return out
	}
	owner, ownerOK := parseATraceExtendedPID(out.ownerRaw)
	if !ownerOK {
		out.issueReason = "invalid_owner_pid"
		return out
	}
	out.ownerPID = owner
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
	out.numericValue, out.numericValid, out.issueReason = parseTraceCounterNumeric(out.valueRaw)
	return out
}

// traceCounterRawPayloadAtCap preserves the writer-envelope truncation signal
// before parseLineScan trims fields. A record whose last captured byte is
// whitespace can otherwise shrink below 1024 and let a truncated numeric name
// component masquerade as the final value.
func traceCounterRawPayloadAtCap(rawFields, normalized string) bool {
	candidate := strings.TrimLeft(rawFields, " \t")
	if strings.HasPrefix(candidate, "C|") {
		return len(candidate) >= traceCounterPayloadMaxBytes
	}
	if idx := strings.IndexByte(candidate, ':'); idx > 0 {
		prefix := strings.TrimSpace(candidate[:idx])
		if tracePrintPrefixLooksLikeAddress(prefix) {
			payload := strings.TrimLeft(candidate[idx+1:], " \t")
			if strings.HasPrefix(payload, "C|") {
				return len(payload) >= traceCounterPayloadMaxBytes
			}
			if strings.HasPrefix(normalized, "C|") {
				// The converter-carved form omitted the two-byte action prefix.
				return len(payload)+2 >= traceCounterPayloadMaxBytes
			}
		}
	}
	return false
}

// parseTraceCounterSample consumes the admission-time side table for parsed
// files. The compatibility fallback exists only for package callers/tests
// which hand-build Event values; production never reparses the 300-byte
// FieldText inventory copy.
func parseTraceCounterSample(ev Event) traceCounterSample {
	var out traceCounterSample
	if ev.Type != EventTraceMark || ev.SpanAction != "C" {
		out.issueReason = "not_counter_mark"
		return out
	}
	if plugin := ev.PluginFields; plugin != nil && plugin.Counter != nil && plugin.Counter.Parsed {
		fields := plugin.Counter
		return traceCounterSample{
			ownerPID: ev.SpanPID, ownerRaw: fields.OwnerRaw,
			ownerScope: fields.OwnerScope, name: ev.SpanName,
			valueRaw: ev.SpanValue, metadataRaw: fields.Metadata,
			outputLevel: fields.OutputLevel, tagBits: fields.TagBits,
			numericValue: fields.NumericValue, numericValid: fields.NumericValid,
			identityOK: fields.IdentityValid, issueReason: fields.IssueReason,
		}
	}

	payload := normalizeTraceMarkPayload(ev.FieldText)
	if strings.HasPrefix(payload, "C|") {
		return parseTraceCounterPayload(payload)
	}
	// A positive SpanPID proves presence for a hand-built typed event. Zero
	// cannot prove that the wire explicitly carried pid=0, so it must not mint
	// a global counter without FieldText provenance.
	if ev.FieldText == "" && ev.SpanPID > 0 {
		return parseTraceCounterPayload(fmt.Sprintf("C|%d|%s|%s", ev.SpanPID, ev.SpanName, ev.SpanValue))
	}
	out.ownerRaw = strconv.Itoa(ev.SpanPID)
	out.name = strings.TrimSpace(ev.SpanName)
	out.valueRaw = strings.TrimSpace(ev.SpanValue)
	out.issueReason = "malformed_counter_fields"
	return out
}

func parseTraceCounterNumeric(raw string) (float64, bool, string) {
	if raw != strings.TrimSpace(raw) {
		return 0, false, "non_decimal_or_non_finite_value"
	}
	if !isAllNumeric(raw) {
		return 0, false, "non_decimal_or_non_finite_value"
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false, "non_decimal_or_non_finite_value"
	}

	unsigned := raw
	if unsigned[0] == '+' || unsigned[0] == '-' {
		unsigned = unsigned[1:]
	}
	integerPart, fractionalPart, _ := strings.Cut(unsigned, ".")
	if n, integerValued, intOK := traceCounterExactInteger(raw); integerValued {
		if !intOK {
			return 0, false, "numeric_out_of_range"
		}
		if n < -traceCounterMaxExactFloatInteger || n > traceCounterMaxExactFloatInteger {
			return 0, false, "numeric_precision_unsafe"
		}
		return value, true, ""
	}

	// Decimal compatibility is intentionally narrower than the native int64
	// lane. At most 15 significant decimal digits plus a normal-range binary64
	// value keeps the float64 summary injective for admitted plain decimals.
	digits := strings.Trim(integerPart+fractionalPart, "0")
	if len(digits) > 15 {
		return 0, false, "numeric_precision_unsafe"
	}
	if value == 0 && strings.ContainsAny(unsigned, "123456789") {
		return 0, false, "numeric_precision_unsafe"
	}
	const smallestNormalFloat64 = 0x1p-1022
	if value != 0 && math.Abs(value) < smallestNormalFloat64 {
		return 0, false, "numeric_precision_unsafe"
	}
	return value, true, ""
}

// traceCounterExactInteger reports whether raw denotes an integer-valued
// plain decimal and, if so, parses it exactly as signed int64. The second
// result distinguishes a real decimal fraction from an out-of-range integer.
func traceCounterExactInteger(raw string) (value int64, integerValued, ok bool) {
	if raw == "" || raw != strings.TrimSpace(raw) || !isAllNumeric(raw) {
		return 0, false, false
	}
	unsigned := raw
	if unsigned[0] == '+' || unsigned[0] == '-' {
		unsigned = unsigned[1:]
	}
	integerPart, fractionalPart, hasDot := strings.Cut(unsigned, ".")
	if hasDot && strings.Trim(fractionalPart, "0") != "" {
		return 0, false, false
	}
	if integerPart == "" {
		integerPart = "0"
	}
	if raw[0] == '-' {
		integerPart = "-" + integerPart
	} else if raw[0] == '+' {
		integerPart = "+" + integerPart
	}
	n, err := strconv.ParseInt(integerPart, 10, 64)
	if err != nil {
		return 0, true, false
	}
	return n, true, true
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
	}, "\x00"), true
}

type traceCounterSeriesKey struct {
	sourcePath string
	ownerPID   int
	ownerScope string
	name       string
}

type traceCounterSeriesAccumulator struct {
	row          TraceCounterDeltaSummary
	invalidValue bool
	firstRaw     string
	lastRaw      string
	lastCoord    traceCounterSourceCoordinate
	lastLine     int
	metadataRaw  string
	metadataDiff bool
}

type traceCounterPublishedDelta struct {
	row      TraceCounterDeltaSummary
	exactAbs *big.Rat
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
			name: sample.name,
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
				Name: sample.name, TrailingTag: sample.metadataRaw,
				OutputLevel: sample.outputLevel, TagBits: sample.tagBits,
				MetadataStatus: traceCounterMetadataStatus(sample.metadataRaw), SourcePath: coord.path,
				Baseline: traceCounterBaselineInWindowFirstSample, UnitStatus: traceCounterUnitUnknown,
			}, metadataRaw: sample.metadataRaw}
			series[key] = acc
		}
		if sample.metadataRaw != acc.metadataRaw {
			acc.metadataDiff = true
			acc.row.MetadataStatus = "varied"
			// A single token cannot honestly summarize a varied series. The
			// full per-row inventory retains each observed metadata value.
			acc.row.TrailingTag = ""
			acc.row.OutputLevel = ""
			acc.row.TagBits = ""
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
	published := make([]traceCounterPublishedDelta, 0, len(series))
	for _, acc := range series {
		if acc.invalidValue || acc.row.Samples == 0 {
			quality.SuppressedSeries++
			continue
		}
		if acc.metadataDiff {
			quality.DerivedInvalidSeries++
			quality.SuppressedSeries++
			appendTraceCounterIssueReason(issues, "counter_metadata_changed", traceCounterSample{
				ownerPID: acc.row.OwnerPID, ownerRaw: strconv.Itoa(acc.row.OwnerPID), ownerScope: acc.row.OwnerScope,
				name: acc.row.Name, valueRaw: "first=" + acc.firstRaw + ",last=" + acc.lastRaw,
				metadataRaw: acc.metadataRaw, identityOK: true,
			}, acc.lastCoord, acc.lastLine)
			continue
		}
		delta, exactAbs, deltaIssue := traceCounterDerivedDelta(acc.firstRaw, acc.lastRaw)
		if deltaIssue != "" {
			quality.DerivedInvalidSeries++
			quality.SuppressedSeries++
			appendTraceCounterIssueReason(issues, deltaIssue, traceCounterSample{
				ownerPID: acc.row.OwnerPID, ownerRaw: strconv.Itoa(acc.row.OwnerPID), ownerScope: acc.row.OwnerScope,
				name: acc.row.Name, valueRaw: "first=" + acc.firstRaw + ",last=" + acc.lastRaw,
				metadataRaw: acc.row.TrailingTag, outputLevel: acc.row.OutputLevel, tagBits: acc.row.TagBits,
				identityOK: true,
			}, acc.lastCoord, acc.lastLine)
			continue
		}
		acc.row.Delta = delta
		published = append(published, traceCounterPublishedDelta{row: acc.row, exactAbs: exactAbs})
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
	sort.SliceStable(published, func(i, j int) bool {
		if cmp := published[i].exactAbs.Cmp(published[j].exactAbs); cmp != 0 {
			return cmp > 0
		}
		left, right := published[i].row, published[j].row
		if left.Samples != right.Samples {
			return left.Samples > right.Samples
		}
		if left.SourcePath != right.SourcePath {
			return left.SourcePath < right.SourcePath
		}
		if left.OwnerScope != right.OwnerScope {
			return left.OwnerScope < right.OwnerScope
		}
		if left.OwnerPID != right.OwnerPID {
			return left.OwnerPID < right.OwnerPID
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.FirstLine < right.FirstLine
	})
	if max > 0 && len(published) > max {
		quality.TruncatedSeries = len(published) - max
		published = published[:max]
	}
	out := make([]TraceCounterDeltaSummary, len(published))
	for i := range published {
		out[i] = published[i].row
	}
	quality.PublishedSeries = len(out)
	quality.Issues = sortedTraceCounterIssues(issues)
	return out, quality
}

// traceCounterDerivedDelta subtracts the original decimal tokens exactly and
// rounds only the final result to the public float64 wire. Subtracting two
// already-rounded endpoints can manufacture a different decimal delta (for
// example 90071992547409.2-90071992547409.1 => 0.109375). Integer deltas must
// remain exactly representable; decimal compatibility deltas may round once,
// but never to zero/subnormal/non-finite.
func traceCounterDerivedDelta(firstRaw, lastRaw string) (float64, *big.Rat, string) {
	first, ok := new(big.Rat).SetString(firstRaw)
	if !ok {
		return 0, nil, "derived_delta_non_finite"
	}
	last, ok := new(big.Rat).SetString(lastRaw)
	if !ok {
		return 0, nil, "derived_delta_non_finite"
	}
	exactDelta := new(big.Rat).Sub(last, first)
	delta, exact := exactDelta.Float64()
	if math.IsNaN(delta) || math.IsInf(delta, 0) {
		return 0, nil, "derived_delta_non_finite"
	}
	const smallestNormalFloat64 = 0x1p-1022
	if exactDelta.Sign() != 0 && (delta == 0 || math.Abs(delta) < smallestNormalFloat64) {
		return 0, nil, "derived_delta_precision_unsafe"
	}
	if exactDelta.IsInt() && !exact {
		return 0, nil, "derived_delta_precision_unsafe"
	}
	return delta, new(big.Rat).Abs(exactDelta), ""
}

func traceCounterMetadataStatus(raw string) string {
	if raw == "" {
		return "absent"
	}
	return "stable"
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
		Value: clampString(sample.valueRaw, 120), TrailingTag: clampString(sample.metadataRaw, 40),
		OutputLevel: sample.outputLevel, TagBits: sample.tagBits,
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
