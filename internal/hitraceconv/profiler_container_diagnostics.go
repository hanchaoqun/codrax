package hitraceconv

// P1-a2.2-A: fixed-cardinality diagnostics for outer ProfilerPluginData
// frames.  Raw plugin names and per-frame reason strings are deliberately not
// keys: precise routing happens first, then this ledger retains only typed
// counters plus bounded, order-independent disclosure samples.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	profilerDiagnosticSampleLimit = 8
	profilerDiagnosticPrefixBytes = 96
)

type profilerPluginRoute uint8

const (
	profilerPluginRouteExactFtrace profilerPluginRoute = iota
	profilerPluginRouteBytrace
	profilerPluginRouteNoncanonicalFtrace
	profilerPluginRouteOtherText
	profilerPluginRouteCount
)

func classifyProfilerPluginRoute(name string) profilerPluginRoute {
	switch {
	case name == "ftrace-plugin":
		return profilerPluginRouteExactFtrace
	case name == "bytrace_plugin":
		return profilerPluginRouteBytrace
	case strings.EqualFold(name, "ftrace-plugin"):
		return profilerPluginRouteNoncanonicalFtrace
	default:
		return profilerPluginRouteOtherText
	}
}

func (route profilerPluginRoute) pluginKey() string {
	switch route {
	case profilerPluginRouteExactFtrace:
		return "ftrace-plugin"
	case profilerPluginRouteBytrace:
		return "bytrace_plugin"
	case profilerPluginRouteNoncanonicalFtrace:
		return "__noncanonical_ftrace__"
	case profilerPluginRouteOtherText:
		return "__other_text__"
	default:
		return "__invalid_plugin_route__"
	}
}

func (route profilerPluginRoute) coverageTable() string {
	return "plugin:" + route.pluginKey()
}

type profilerPluginOutcome uint8

const (
	profilerPluginOutcomeStructured profilerPluginOutcome = iota
	profilerPluginOutcomeStructuredDegraded
	profilerPluginOutcomeMalformed
	profilerPluginOutcomeStrictText
	profilerPluginOutcomeUnsupportedFtrace
	profilerPluginOutcomeNoncanonicalFtrace
	profilerPluginOutcomeTextRows
	profilerPluginOutcomeEmptyPayload
	profilerPluginOutcomeNoTextRows
	profilerPluginOutcomeCount
)

func (outcome profilerPluginOutcome) label() string {
	switch outcome {
	case profilerPluginOutcomeStructured:
		return "structured"
	case profilerPluginOutcomeStructuredDegraded:
		return "structured_degraded"
	case profilerPluginOutcomeMalformed:
		return "malformed"
	case profilerPluginOutcomeStrictText:
		return "strict_legacy_text"
	case profilerPluginOutcomeUnsupportedFtrace:
		return "unsupported_ftrace"
	case profilerPluginOutcomeNoncanonicalFtrace:
		return "noncanonical_ftrace"
	case profilerPluginOutcomeTextRows:
		return "text_rows"
	case profilerPluginOutcomeEmptyPayload:
		return "empty_payload"
	case profilerPluginOutcomeNoTextRows:
		return "no_text_rows"
	default:
		return "invalid_outcome"
	}
}

type profilerPluginIssueKind uint8

const (
	profilerPluginIssueMalformedWire profilerPluginIssueKind = iota
	profilerPluginIssueNameMissing
	profilerPluginIssueNameInvalid
	profilerPluginIssueStatusOutOfRange
	profilerPluginIssueClockIDOutOfRange
	profilerPluginIssueTVNsecOutOfRange
	profilerPluginIssueVersionInvalid
	profilerPluginIssueSampleIntervalOutOfRange
	profilerPluginIssueField1WrongWire
	profilerPluginIssueField2WrongWire
	profilerPluginIssueField3WrongWire
	profilerPluginIssueField4WrongWire
	profilerPluginIssueField5WrongWire
	profilerPluginIssueField6WrongWire
	profilerPluginIssueField7WrongWire
	profilerPluginIssueField8WrongWire
	profilerPluginIssueField1Duplicate
	profilerPluginIssueField2Duplicate
	profilerPluginIssueField3Duplicate
	profilerPluginIssueField4Duplicate
	profilerPluginIssueField5Duplicate
	profilerPluginIssueField6Duplicate
	profilerPluginIssueField7Duplicate
	profilerPluginIssueField8Duplicate
	profilerPluginIssueKindCount
)

func profilerPluginWrongWireIssue(field int) (profilerPluginIssueKind, bool) {
	if field < 1 || field > 8 {
		return 0, false
	}
	return profilerPluginIssueKind(int(profilerPluginIssueField1WrongWire) + field - 1), true
}

func profilerPluginDuplicateIssue(field int) (profilerPluginIssueKind, bool) {
	if field < 1 || field > 8 {
		return 0, false
	}
	return profilerPluginIssueKind(int(profilerPluginIssueField1Duplicate) + field - 1), true
}

func (kind profilerPluginIssueKind) label() string {
	switch kind {
	case profilerPluginIssueMalformedWire:
		return "plugin_message_malformed_wire"
	case profilerPluginIssueNameMissing:
		return "plugin_name_missing"
	case profilerPluginIssueNameInvalid:
		return "plugin_name_invalid"
	case profilerPluginIssueStatusOutOfRange:
		return "plugin_status_out_of_range"
	case profilerPluginIssueClockIDOutOfRange:
		return "plugin_clock_id_out_of_range"
	case profilerPluginIssueTVNsecOutOfRange:
		return "plugin_tv_nsec_out_of_range"
	case profilerPluginIssueVersionInvalid:
		return "plugin_version_invalid"
	case profilerPluginIssueSampleIntervalOutOfRange:
		return "plugin_sample_interval_out_of_range"
	}
	if kind >= profilerPluginIssueField1WrongWire && kind <= profilerPluginIssueField8WrongWire {
		return fmt.Sprintf("plugin_field%d_wrong_wire", int(kind-profilerPluginIssueField1WrongWire)+1)
	}
	if kind >= profilerPluginIssueField1Duplicate && kind <= profilerPluginIssueField8Duplicate {
		return fmt.Sprintf("plugin_field%d_duplicate", int(kind-profilerPluginIssueField1Duplicate)+1)
	}
	return "plugin_issue_invalid"
}

type profilerPluginIssueCensus struct {
	Occurrences     [profilerPluginIssueKindCount]uint64
	AffectedFrames  [profilerPluginIssueKindCount]uint64
	DuplicateExcess [8]uint64
}

func (census *profilerPluginIssueCensus) observe(kind profilerPluginIssueKind, delta uint64) bool {
	if census == nil || kind >= profilerPluginIssueKindCount || delta == 0 {
		return census != nil && kind < profilerPluginIssueKindCount
	}
	index := int(kind)
	if census.Occurrences[index] == 0 {
		if !checkedProfilerUint64AddTo(&census.AffectedFrames[index], 1) {
			return false
		}
	}
	return checkedProfilerUint64AddTo(&census.Occurrences[index], delta)
}

func (census *profilerPluginIssueCensus) observeDuplicate(field int, excess uint64) bool {
	kind, ok := profilerPluginDuplicateIssue(field)
	if !ok || excess == 0 || !census.observe(kind, 1) {
		return ok && excess == 0
	}
	return checkedProfilerUint64AddTo(&census.DuplicateExcess[field-1], excess)
}

func (census *profilerPluginIssueCensus) merge(frame profilerPluginIssueCensus) bool {
	if census == nil {
		return false
	}
	for index := 0; index < int(profilerPluginIssueKindCount); index++ {
		if !checkedProfilerUint64AddTo(&census.Occurrences[index], frame.Occurrences[index]) ||
			!checkedProfilerUint64AddTo(&census.AffectedFrames[index], frame.AffectedFrames[index]) {
			return false
		}
	}
	for index := range census.DuplicateExcess {
		if !checkedProfilerUint64AddTo(&census.DuplicateExcess[index], frame.DuplicateExcess[index]) {
			return false
		}
	}
	return true
}

func (census profilerPluginIssueCensus) empty() bool {
	for _, count := range census.Occurrences {
		if count > 0 {
			return false
		}
	}
	return true
}

func (census profilerPluginIssueCensus) labels() []string {
	out := make([]string, 0, profilerPluginIssueKindCount)
	for kind := profilerPluginIssueKind(0); kind < profilerPluginIssueKindCount; kind++ {
		if census.Occurrences[int(kind)] > 0 {
			out = append(out, kind.label())
		}
	}
	return out
}

func (census profilerPluginIssueCensus) summary() string {
	parts := make([]string, 0, profilerPluginIssueKindCount)
	for kind := profilerPluginIssueKind(0); kind < profilerPluginIssueKindCount; kind++ {
		if count := census.Occurrences[int(kind)]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", kind.label(), count))
		}
	}
	return strings.Join(parts, ",")
}

func (census profilerPluginIssueCensus) appendFieldSources(fields map[string]string) {
	for kind := profilerPluginIssueKind(0); kind < profilerPluginIssueKindCount; kind++ {
		index := int(kind)
		if census.Occurrences[index] == 0 {
			continue
		}
		prefix := "issue_" + kind.label()
		fields[prefix+"_occurrences"] = strconv.FormatUint(census.Occurrences[index], 10)
		fields[prefix+"_affected_frames"] = strconv.FormatUint(census.AffectedFrames[index], 10)
	}
	for index, count := range census.DuplicateExcess {
		if count > 0 {
			fields[fmt.Sprintf("issue_plugin_field%d_duplicate_excess_occurrences", index+1)] = strconv.FormatUint(count, 10)
		}
	}
}

type profilerDiagnosticSample struct {
	Digest    [sha256.Size]byte
	Prefix    [profilerDiagnosticPrefixBytes]byte
	PrefixLen uint8
	InputLen  uint64
}

type profilerStableSampleSet struct {
	Items [profilerDiagnosticSampleLimit]profilerDiagnosticSample
	Used  uint8
}

func (samples *profilerStableSampleSet) observe(domain string, raw []byte) {
	if samples == nil {
		return
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(raw)
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	used := int(samples.Used)
	insert := used
	for index := 0; index < used; index++ {
		cmp := bytes.Compare(digest[:], samples.Items[index].Digest[:])
		if cmp == 0 {
			return
		}
		if cmp < 0 {
			insert = index
			break
		}
	}
	if used == profilerDiagnosticSampleLimit && insert == used {
		return
	}
	if used < profilerDiagnosticSampleLimit {
		used++
		samples.Used = uint8(used)
	}
	for index := used - 1; index > insert; index-- {
		samples.Items[index] = samples.Items[index-1]
	}
	item := profilerDiagnosticSample{Digest: digest, InputLen: uint64(len(raw))}
	prefixLen := min(len(raw), profilerDiagnosticPrefixBytes)
	copy(item.Prefix[:], raw[:prefixLen])
	item.PrefixLen = uint8(prefixLen)
	samples.Items[insert] = item
}

func (samples profilerStableSampleSet) render() string {
	parts := make([]string, 0, samples.Used)
	for index := 0; index < int(samples.Used); index++ {
		item := samples.Items[index]
		prefix := strings.ToValidUTF8(string(item.Prefix[:item.PrefixLen]), "�")
		parts = append(parts, fmt.Sprintf("prefix=%s,bytes=%d,sha256=%s",
			strconv.QuoteToASCII(prefix), item.InputLen, hex.EncodeToString(item.Digest[:])))
	}
	return strings.Join(parts, " | ")
}

type profilerPluginMetadataCensus struct {
	PayloadCount uint64
	PayloadTotal uint64
	PayloadMin   uint64
	PayloadMax   uint64

	StatusPresent uint64
	StatusMin     uint32
	StatusMax     uint32

	ClockPresent   uint64
	ClockIDs       [12]uint64
	ClockAmbiguous uint64

	TimePresent   uint64
	TimeAmbiguous uint64
	TimeMinSec    uint64
	TimeMinNsec   uint64
	TimeMaxSec    uint64
	TimeMaxNsec   uint64

	VersionPresent uint64
	VersionSamples profilerStableSampleSet

	SamplePresent uint64
	SampleMin     uint32
	SampleMax     uint32
}

func (census *profilerPluginMetadataCensus) observe(route profilerPluginRoute, plugin profilerPluginData) bool {
	if census == nil {
		return false
	}
	payloadBytes := uint64(len(plugin.Data))
	if census.PayloadCount == 0 {
		census.PayloadMin = payloadBytes
	}
	if !checkedProfilerUint64AddTo(&census.PayloadCount, 1) ||
		!checkedProfilerUint64AddTo(&census.PayloadTotal, payloadBytes) {
		return false
	}
	if payloadBytes < census.PayloadMin {
		census.PayloadMin = payloadBytes
	}
	if payloadBytes > census.PayloadMax {
		census.PayloadMax = payloadBytes
	}
	if plugin.StatusPresent {
		if census.StatusPresent == 0 {
			census.StatusMin, census.StatusMax = plugin.Status, plugin.Status
		}
		if !checkedProfilerUint64AddTo(&census.StatusPresent, 1) {
			return false
		}
		if plugin.Status < census.StatusMin {
			census.StatusMin = plugin.Status
		}
		if plugin.Status > census.StatusMax {
			census.StatusMax = plugin.Status
		}
	}
	if plugin.ClockIDAmbiguous {
		if !checkedProfilerUint64AddTo(&census.ClockAmbiguous, 1) {
			return false
		}
	} else if plugin.ClockIDPresent {
		if plugin.ClockID >= uint64(len(census.ClockIDs)) ||
			!checkedProfilerUint64AddTo(&census.ClockPresent, 1) ||
			!checkedProfilerUint64AddTo(&census.ClockIDs[plugin.ClockID], 1) {
			return false
		}
	}
	timePresent := plugin.TvSecPresent || plugin.TvNsecPresent
	if plugin.TimeTupleAmbiguous {
		if !checkedProfilerUint64AddTo(&census.TimeAmbiguous, 1) {
			return false
		}
	} else if timePresent {
		if census.TimePresent == 0 {
			census.TimeMinSec, census.TimeMaxSec = plugin.TvSec, plugin.TvSec
			census.TimeMinNsec, census.TimeMaxNsec = plugin.TvNsec, plugin.TvNsec
		}
		if !checkedProfilerUint64AddTo(&census.TimePresent, 1) {
			return false
		}
		if profilerTimeTupleLess(plugin.TvSec, plugin.TvNsec, census.TimeMinSec, census.TimeMinNsec) {
			census.TimeMinSec, census.TimeMinNsec = plugin.TvSec, plugin.TvNsec
		}
		if profilerTimeTupleLess(census.TimeMaxSec, census.TimeMaxNsec, plugin.TvSec, plugin.TvNsec) {
			census.TimeMaxSec, census.TimeMaxNsec = plugin.TvSec, plugin.TvNsec
		}
	}
	if plugin.VersionPresent {
		if !checkedProfilerUint64AddTo(&census.VersionPresent, 1) {
			return false
		}
		census.VersionSamples.observe("profiler-version:"+route.pluginKey(), []byte(plugin.Version))
	}
	if plugin.SampleIntervalPresent {
		if census.SamplePresent == 0 {
			census.SampleMin, census.SampleMax = plugin.SampleInterval, plugin.SampleInterval
		}
		if !checkedProfilerUint64AddTo(&census.SamplePresent, 1) {
			return false
		}
		if plugin.SampleInterval < census.SampleMin {
			census.SampleMin = plugin.SampleInterval
		}
		if plugin.SampleInterval > census.SampleMax {
			census.SampleMax = plugin.SampleInterval
		}
	}
	return true
}

func profilerTimeTupleLess(leftSec, leftNsec, rightSec, rightNsec uint64) bool {
	return leftSec < rightSec || leftSec == rightSec && leftNsec < rightNsec
}

type profilerPluginBucketCensus struct {
	Observed            bool
	CoverageIndex       int
	Messages            uint64
	FirstOffset         int64
	LastOffset          int64
	Outcomes            [profilerPluginOutcomeCount]uint64
	Issues              profilerPluginIssueCensus
	DegradedFrames      uint64
	FirstDegradedOffset int64
	LastDegradedOffset  int64
	Metadata            profilerPluginMetadataCensus
	NameSamples         profilerStableSampleSet
	StagedMMC           uint64
	StagedF2FS          uint64
}

type profilerRejectedFrameCensus struct {
	Observed      bool
	CoverageIndex int
	Frames        uint64
	FirstOffset   int64
	LastOffset    int64
	Issues        profilerPluginIssueCensus
}

type profilerContainerDiagnosticLedger struct {
	Buckets        [profilerPluginRouteCount]profilerPluginBucketCensus
	Rejected       profilerRejectedFrameCensus
	FtraceEnvelope profilerFtraceEnvelopeDiagnosticLedger
	FtraceSummary  profilerFtraceSummaryDiagnosticLedger
	FtraceEvents   profilerFtraceEventDiagnosticLedger
	Materialized   bool
}

func newProfilerContainerDiagnosticLedger() profilerContainerDiagnosticLedger {
	ledger := profilerContainerDiagnosticLedger{}
	for index := range ledger.Buckets {
		ledger.Buckets[index].CoverageIndex = -1
	}
	ledger.Rejected.CoverageIndex = -1
	return ledger
}

func (ledger *profilerContainerDiagnosticLedger) ensurePluginCoverage(out *profilerContainerExtraction, route profilerPluginRoute) (int, bool) {
	if ledger == nil || out == nil || route >= profilerPluginRouteCount {
		return -1, false
	}
	bucket := &ledger.Buckets[route]
	if bucket.CoverageIndex >= 0 {
		return bucket.CoverageIndex, true
	}
	role := "query_ready_export"
	if route == profilerPluginRouteNoncanonicalFtrace {
		role = "unsupported_input"
	}
	bucket.CoverageIndex = len(out.TraceCoverage)
	out.TraceCoverage = append(out.TraceCoverage, TraceDBCoverage{
		Family: "builtin_modern_profiler", Table: route.coverageTable(), Role: role, Found: true,
	})
	return bucket.CoverageIndex, true
}

func (ledger *profilerContainerDiagnosticLedger) observeAccepted(out *profilerContainerExtraction,
	route profilerPluginRoute, rawName string, plugin profilerPluginData, issues profilerPluginIssueCensus,
	offset int64, outcome profilerPluginOutcome, rowsEmitted int, mmcRows, f2fsRows profilerPairRowCensus,
) (int, bool) {
	index, ok := ledger.ensurePluginCoverage(out, route)
	if !ok || outcome >= profilerPluginOutcomeCount || rowsEmitted < 0 {
		return -1, false
	}
	bucket := &ledger.Buckets[route]
	if !bucket.Observed {
		bucket.Observed = true
		bucket.FirstOffset = offset
	}
	bucket.LastOffset = offset
	if !checkedProfilerUint64AddTo(&bucket.Messages, 1) ||
		!checkedProfilerUint64AddTo(&bucket.Outcomes[outcome], 1) ||
		!bucket.Issues.merge(issues) || !bucket.Metadata.observe(route, plugin) ||
		!checkedProfilerUint64AddTo(&bucket.StagedMMC, uint64(mmcRows.total)) ||
		!checkedProfilerUint64AddTo(&bucket.StagedF2FS, uint64(f2fsRows.total)) {
		return -1, false
	}
	if !issues.empty() {
		if bucket.DegradedFrames == 0 {
			bucket.FirstDegradedOffset = offset
		}
		if !checkedProfilerUint64AddTo(&bucket.DegradedFrames, 1) {
			return -1, false
		}
		bucket.LastDegradedOffset = offset
	}
	if route == profilerPluginRouteNoncanonicalFtrace || route == profilerPluginRouteOtherText {
		bucket.NameSamples.observe("profiler-name:"+route.pluginKey(), []byte(rawName))
	}
	if !checkedProfilerIntAddTo(&out.TraceCoverage[index].RowsRead, 1) ||
		!checkedProfilerIntAddTo(&out.TraceCoverage[index].RowsEmitted, rowsEmitted) {
		return -1, false
	}
	key := route.pluginKey()
	if out.PluginMessages == nil {
		out.PluginMessages = map[string]int{}
	}
	count := out.PluginMessages[key]
	if !checkedProfilerIntAddTo(&count, 1) {
		return -1, false
	}
	out.PluginMessages[key] = count
	return index, true
}

func (ledger *profilerContainerDiagnosticLedger) observeRejected(out *profilerContainerExtraction,
	issues profilerPluginIssueCensus, offset int64,
) bool {
	if ledger == nil || out == nil {
		return false
	}
	rejected := &ledger.Rejected
	if rejected.CoverageIndex < 0 {
		rejected.CoverageIndex = len(out.TraceCoverage)
		out.TraceCoverage = append(out.TraceCoverage, TraceDBCoverage{
			Family: "builtin_modern_profiler", Table: "plugin:__rejected__", Role: "unsupported_input", Found: true,
		})
	}
	if !rejected.Observed {
		rejected.Observed = true
		rejected.FirstOffset = offset
	}
	rejected.LastOffset = offset
	if !checkedProfilerUint64AddTo(&rejected.Frames, 1) || !rejected.Issues.merge(issues) {
		return false
	}
	return checkedProfilerIntAddTo(&out.TraceCoverage[rejected.CoverageIndex].RowsRead, 1)
}

func (ledger *profilerContainerDiagnosticLedger) materialize(out *profilerContainerExtraction) bool {
	if ledger == nil || out == nil || ledger.Materialized {
		return ledger != nil && out != nil
	}
	ledger.Materialized = true
	for route := profilerPluginRoute(0); route < profilerPluginRouteCount; route++ {
		bucket := &ledger.Buckets[route]
		if !bucket.Observed || bucket.CoverageIndex < 0 || bucket.CoverageIndex >= len(out.TraceCoverage) {
			continue
		}
		coverage := &out.TraceCoverage[bucket.CoverageIndex]
		fields := map[string]string{
			"schema_profile":     "ProfilerPluginData{name=1,status=2,data=3,clock_id=4,tv_sec=5,tv_nsec=6,version=7,sample_interval=8}",
			"aggregation_policy": "fixed_route_bucket_checked_counters",
			"observed_messages":  strconv.FormatUint(bucket.Messages, 10),
			"first_offset":       strconv.FormatInt(bucket.FirstOffset, 10),
			"last_offset":        strconv.FormatInt(bucket.LastOffset, 10),
		}
		if route == profilerPluginRouteNoncanonicalFtrace || route == profilerPluginRouteOtherText {
			fields["identity_compacted"] = "true"
			fields["identity_sample_policy"] = "sha256_min_k8_domain_separated_prefix96"
			fields["original_plugin_name_table_key"] = "false"
			fields["plugin_name_samples"] = bucket.NameSamples.render()
		}
		for outcome := profilerPluginOutcome(0); outcome < profilerPluginOutcomeCount; outcome++ {
			if count := bucket.Outcomes[outcome]; count > 0 {
				fields["outcome_"+outcome.label()+"_frames"] = strconv.FormatUint(count, 10)
			}
		}
		if bucket.StagedMMC > 0 {
			fields[profilerCoverageMMCStagedRows] = strconv.FormatUint(bucket.StagedMMC, 10)
		}
		if bucket.StagedF2FS > 0 {
			fields[profilerCoverageF2FSStagedRows] = strconv.FormatUint(bucket.StagedF2FS, 10)
		}
		bucket.Issues.appendFieldSources(fields)
		if bucket.DegradedFrames > 0 {
			fields["metadata_degraded_frames"] = strconv.FormatUint(bucket.DegradedFrames, 10)
			fields["metadata_degraded_first_offset"] = strconv.FormatInt(bucket.FirstDegradedOffset, 10)
			fields["metadata_degraded_last_offset"] = strconv.FormatInt(bucket.LastDegradedOffset, 10)
		}
		appendProfilerMetadataFieldSources(fields, bucket.Metadata)
		coverage.FieldSources = fields
		coverage.Skipped = profilerPluginBucketSkippedSummary(*bucket)
		out.Caveats = append(out.Caveats, profilerPluginBucketMetadataCaveat(route, *bucket))
		if summary := bucket.Issues.summary(); summary != "" {
			out.Caveats = append(out.Caveats, fmt.Sprintf(
				"profiler plugin bucket %s metadata degraded: frames=%d first_offset=%d last_offset=%d; reasons=%s",
				route.pluginKey(), bucket.DegradedFrames, bucket.FirstDegradedOffset, bucket.LastDegradedOffset, summary))
		}
	}
	if rejected := &ledger.Rejected; rejected.Observed {
		if rejected.CoverageIndex < 0 || rejected.CoverageIndex >= len(out.TraceCoverage) {
			return false
		}
		coverage := &out.TraceCoverage[rejected.CoverageIndex]
		fields := map[string]string{
			"schema_profile":     "ProfilerPluginData{name=1,status=2,data=3,clock_id=4,tv_sec=5,tv_nsec=6,version=7,sample_interval=8}",
			"aggregation_policy": "fixed_reason_census_with_first_last_offset",
			"observed_messages":  strconv.FormatUint(rejected.Frames, 10),
			"first_offset":       strconv.FormatInt(rejected.FirstOffset, 10),
			"last_offset":        strconv.FormatInt(rejected.LastOffset, 10),
		}
		rejected.Issues.appendFieldSources(fields)
		coverage.FieldSources = fields
		coverage.Skipped = rejected.Issues.summary()
		if coverage.Skipped == "" {
			coverage.Skipped = "plugin_message_rejected=1"
		}
		out.Caveats = append(out.Caveats, fmt.Sprintf(
			"rejected %d complete ProfilerPluginData message(s); first_offset=%d last_offset=%d; reasons=%s",
			rejected.Frames, rejected.FirstOffset, rejected.LastOffset, coverage.Skipped))
	}
	return ledger.FtraceEnvelope.materialize(out) && ledger.FtraceSummary.materialize(out) &&
		ledger.FtraceEvents.materialize(out)
}

func profilerPluginBucketSkippedSummary(bucket profilerPluginBucketCensus) string {
	parts := make([]string, 0, profilerPluginOutcomeCount)
	for _, outcome := range []profilerPluginOutcome{
		profilerPluginOutcomeStructuredDegraded,
		profilerPluginOutcomeMalformed,
		profilerPluginOutcomeUnsupportedFtrace,
		profilerPluginOutcomeNoncanonicalFtrace,
		profilerPluginOutcomeEmptyPayload,
		profilerPluginOutcomeNoTextRows,
	} {
		if count := bucket.Outcomes[outcome]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", outcome.label(), count))
		}
	}
	return strings.Join(parts, ",")
}

func appendProfilerMetadataFieldSources(fields map[string]string, census profilerPluginMetadataCensus) {
	fields["payload_count"] = strconv.FormatUint(census.PayloadCount, 10)
	fields["payload_bytes_total"] = strconv.FormatUint(census.PayloadTotal, 10)
	fields["payload_bytes_min"] = strconv.FormatUint(census.PayloadMin, 10)
	fields["payload_bytes_max"] = strconv.FormatUint(census.PayloadMax, 10)
	if census.StatusPresent > 0 {
		fields["status_present"] = strconv.FormatUint(census.StatusPresent, 10)
		fields["status_min"] = strconv.FormatUint(uint64(census.StatusMin), 10)
		fields["status_max"] = strconv.FormatUint(uint64(census.StatusMax), 10)
	}
	if census.ClockPresent > 0 {
		fields["clock_id_present"] = strconv.FormatUint(census.ClockPresent, 10)
		for id, count := range census.ClockIDs {
			if count > 0 {
				fields[fmt.Sprintf("clock_id_%d_count", id)] = strconv.FormatUint(count, 10)
			}
		}
	}
	if census.ClockAmbiguous > 0 {
		fields["clock_id_ambiguous"] = strconv.FormatUint(census.ClockAmbiguous, 10)
	}
	if census.TimePresent > 0 {
		fields["time_tuple_present"] = strconv.FormatUint(census.TimePresent, 10)
		fields["time_tuple_min"] = fmt.Sprintf("%d.%09d", census.TimeMinSec, census.TimeMinNsec)
		fields["time_tuple_max"] = fmt.Sprintf("%d.%09d", census.TimeMaxSec, census.TimeMaxNsec)
	}
	if census.TimeAmbiguous > 0 {
		fields["time_tuple_ambiguous"] = strconv.FormatUint(census.TimeAmbiguous, 10)
	}
	if census.VersionPresent > 0 {
		fields["version_present"] = strconv.FormatUint(census.VersionPresent, 10)
		fields["version_sample_policy"] = "sha256_min_k8_domain_separated_prefix96"
		fields["version_samples"] = census.VersionSamples.render()
	}
	if census.SamplePresent > 0 {
		fields["sample_interval_present"] = strconv.FormatUint(census.SamplePresent, 10)
		fields["sample_interval_min_ms"] = strconv.FormatUint(uint64(census.SampleMin), 10)
		fields["sample_interval_max_ms"] = strconv.FormatUint(uint64(census.SampleMax), 10)
	}
}

func profilerPluginBucketMetadataCaveat(route profilerPluginRoute, bucket profilerPluginBucketCensus) string {
	census := bucket.Metadata
	parts := make([]string, 0, 12)
	if census.ClockPresent > 0 {
		oneID := -1
		for id, count := range census.ClockIDs {
			if count == 0 {
				continue
			}
			if oneID >= 0 {
				oneID = -2
				break
			}
			oneID = id
		}
		if oneID >= 0 && census.ClockIDs[oneID] == census.ClockPresent {
			parts = append(parts, "clock_id="+profilerPluginClockName(uint64(oneID)))
		} else {
			var counts []string
			for id, count := range census.ClockIDs {
				if count > 0 {
					counts = append(counts, fmt.Sprintf("%s:%d", profilerPluginClockName(uint64(id)), count))
				}
			}
			parts = append(parts, "clock_id_counts="+strings.Join(counts, ","))
		}
	}
	if census.TimePresent > 0 {
		if census.TimeMinSec == census.TimeMaxSec && census.TimeMinNsec == census.TimeMaxNsec {
			parts = append(parts, fmt.Sprintf("tv=%d.%09d", census.TimeMinSec, census.TimeMinNsec))
		} else {
			parts = append(parts, fmt.Sprintf("tv=%d.%09d~%d.%09d", census.TimeMinSec, census.TimeMinNsec, census.TimeMaxSec, census.TimeMaxNsec))
		}
	}
	if census.SamplePresent > 0 {
		if census.SampleMin == census.SampleMax {
			parts = append(parts, fmt.Sprintf("sample_interval_ms=%d", census.SampleMin))
		} else {
			parts = append(parts, fmt.Sprintf("sample_interval_ms=%d~%d", census.SampleMin, census.SampleMax))
		}
	}
	parts = append(parts,
		fmt.Sprintf("messages=%d", bucket.Messages),
		fmt.Sprintf("payload_bytes=count:%d/sum:%d/min:%d/max:%d", census.PayloadCount, census.PayloadTotal, census.PayloadMin, census.PayloadMax))
	if census.StatusPresent > 0 {
		parts = append(parts, fmt.Sprintf("status=%d~%d(present:%d)", census.StatusMin, census.StatusMax, census.StatusPresent))
	}
	if census.ClockAmbiguous > 0 || census.TimeAmbiguous > 0 {
		parts = append(parts, fmt.Sprintf("ambiguous_clock:%d/time:%d", census.ClockAmbiguous, census.TimeAmbiguous))
	}
	if census.VersionPresent > 0 {
		parts = append(parts, fmt.Sprintf("version_samples=%s", census.VersionSamples.render()))
	}
	if bucket.NameSamples.Used > 0 {
		parts = append(parts, fmt.Sprintf("name_samples=%s", bucket.NameSamples.render()))
	}
	return fmt.Sprintf("profiler plugin %s metadata: %s", route.pluginKey(), strings.Join(parts, "; "))
}

func checkedProfilerUint64AddTo(dst *uint64, delta uint64) bool {
	if dst == nil || delta > math.MaxUint64-*dst {
		return false
	}
	*dst += delta
	return true
}

func checkedProfilerIntAddTo(dst *int, delta int) bool {
	if dst == nil || delta < 0 || *dst < 0 || delta > math.MaxInt-*dst {
		return false
	}
	*dst += delta
	return true
}
