package hitraceconv

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceDBRawAsyncKey struct {
	PayloadPID int64
	Name       string
	Value      string
}

type traceDBRawAsyncExactKey struct {
	traceDBRawAsyncKey
	Start uint64
	End   uint64
}

// traceDBRawAsyncIntervalKey deliberately excludes PayloadPID. The official
// high-level callstack row exposes a logical owner process, while the raw
// S/F payload may carry a namespace PID. They are not interchangeable
// identities. A cross-PID claim therefore requires this complete interval key
// plus one unique matching physical begin emitter envelope.
type traceDBRawAsyncIntervalKey struct {
	Name  string
	Value string
	Start uint64
	End   uint64
}

type traceDBRawAsyncSemanticKey struct {
	Name  string
	Value string
}

type traceDBRawAsyncNameIntervalKey struct {
	Name  string
	Start uint64
	End   uint64
}

type traceDBRawAsyncValueIntervalKey struct {
	Value string
	Start uint64
	End   uint64
}

type traceDBRawAsyncTimeIntervalKey struct {
	Start uint64
	End   uint64
}

type traceDBRawAsyncPair struct {
	begin        traceDBRawMarkerRecord
	end          traceDBRawMarkerRecord
	beginThread  traceDBThread
	beginProcess traceDBProcess
	endThread    traceDBThread
	endProcess   traceDBProcess
	claimed      bool
}

// traceDBRawAsyncMatchLedger is intentionally match-only in its first
// production version. Exact physical S/F pairs may replace one already-proven
// high-level completed interval, but unmatched raw pairs are not independently
// published until the legacy DB async endpoint lane has a cross-source dedup
// authority.
type traceDBRawAsyncMatchLedger struct {
	state                string
	pairs                map[traceDBRawAsyncExactKey][]*traceDBRawAsyncPair
	byInterval           map[traceDBRawAsyncIntervalKey][]*traceDBRawAsyncPair
	bySemantic           map[traceDBRawAsyncSemanticKey][]*traceDBRawAsyncPair
	byNameInterval       map[traceDBRawAsyncNameIntervalKey][]*traceDBRawAsyncPair
	byValueInterval      map[traceDBRawAsyncValueIntervalKey][]*traceDBRawAsyncPair
	byTimeInterval       map[traceDBRawAsyncTimeIntervalKey][]*traceDBRawAsyncPair
	metrics              map[string]int64
	mismatchWitnesses    []string
	mismatchWitnessTotal int
}

const traceDBRawAsyncMismatchWitnessCap = 8

func newTraceDBRawAsyncMatchLedger(
	inventory *traceDBSourceNameInventory,
	authority traceDBSchedulerAuthority,
) *traceDBRawAsyncMatchLedger {
	ledger := &traceDBRawAsyncMatchLedger{
		state:             "unavailable",
		pairs:             map[traceDBRawAsyncExactKey][]*traceDBRawAsyncPair{},
		byInterval:        map[traceDBRawAsyncIntervalKey][]*traceDBRawAsyncPair{},
		bySemantic:        map[traceDBRawAsyncSemanticKey][]*traceDBRawAsyncPair{},
		byNameInterval:    map[traceDBRawAsyncNameIntervalKey][]*traceDBRawAsyncPair{},
		byValueInterval:   map[traceDBRawAsyncValueIntervalKey][]*traceDBRawAsyncPair{},
		byTimeInterval:    map[traceDBRawAsyncTimeIntervalKey][]*traceDBRawAsyncPair{},
		metrics:           map[string]int64{},
		mismatchWitnesses: make([]string, 0, traceDBRawAsyncMismatchWitnessCap),
	}
	if inventory == nil {
		return ledger
	}
	if inventory.RawDecode.Metadata["decode_state"] != "strict_target_ledger_complete" {
		ledger.state = "withheld_raw_decode_incomplete"
		return ledger
	}
	rows := make([]traceDBRawMarkerRecord, 0)
	for _, row := range inventory.RawMarkers {
		if row.Admitted && (row.Action == "S" || row.Action == "F") {
			rows = append(rows, row)
		}
	}
	ledger.metrics["endpoint_records"] = int64(len(rows))
	if inventory.RawDecode.Metrics["target_marker_async_records_retained"] != int64(len(rows)) {
		ledger.state = "withheld_endpoint_census_mismatch"
		return ledger
	}
	if !authority.initialized || !authority.complete {
		ledger.state = "withheld_lifecycle_authority_incomplete"
		return ledger
	}
	if len(rows) == 0 {
		ledger.state = "complete_no_endpoint"
		return ledger
	}

	byKey := map[traceDBRawAsyncKey][]traceDBRawMarkerRecord{}
	for _, row := range rows {
		key := traceDBRawAsyncKey{
			PayloadPID: row.PayloadPID,
			Name:       row.Name,
			Value:      row.Value,
		}
		byKey[key] = append(byKey[key], row)
	}
	keys := make([]traceDBRawAsyncKey, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].PayloadPID != keys[j].PayloadPID {
			return keys[i].PayloadPID < keys[j].PayloadPID
		}
		if keys[i].Name != keys[j].Name {
			return keys[i].Name < keys[j].Name
		}
		return keys[i].Value < keys[j].Value
	})
	ledger.metrics["keys"] = int64(len(keys))

	for _, key := range keys {
		lane := byKey[key]
		sort.SliceStable(lane, func(i, j int) bool {
			if lane[i].TimestampNS != lane[j].TimestampNS {
				return lane[i].TimestampNS < lane[j].TimestampNS
			}
			return lane[i].PhysicalOrdinal < lane[j].PhysicalOrdinal
		})
		var open *traceDBRawMarkerRecord
		provisional := make([]traceDBRawMarkerPair, 0, len(lane)/2)
		poisoned := false
		var lastTS uint64
		var lastOrdinal int64
		haveLast := false
		for index := range lane {
			row := lane[index]
			if reason := traceDBRawAsyncRecordReason(row, key); reason != "" {
				ledger.metrics["records_withheld_"+traceDBRawDecodeReasonMetric(reason)]++
				poisoned = true
				break
			}
			if row.PhysicalOrdinal <= 0 ||
				haveLast && row.TimestampNS == lastTS && row.PhysicalOrdinal <= lastOrdinal {
				ledger.metrics["records_withheld_invalid_physical_order"]++
				poisoned = true
				break
			}
			lastTS, lastOrdinal, haveLast = row.TimestampNS, row.PhysicalOrdinal, true
			switch row.Action {
			case "S":
				if open != nil {
					ledger.metrics["duplicate_open_starts"]++
					poisoned = true
					break
				}
				copy := row
				open = &copy
			case "F":
				if open == nil {
					ledger.metrics["orphan_finishes"]++
					continue
				}
				provisional = append(provisional, traceDBRawMarkerPair{
					begin: *open,
					end:   row,
				})
				open = nil
			}
			if poisoned {
				break
			}
		}
		if poisoned {
			ledger.metrics["keys_poisoned"]++
			continue
		}
		if open != nil {
			ledger.metrics["open_starts"]++
		}
		for _, rawPair := range provisional {
			pair, reason := traceDBRawAsyncPairFromRecords(rawPair, authority)
			if reason != "" {
				ledger.metrics["pairs_withheld_"+traceDBRawDecodeReasonMetric(reason)]++
				continue
			}
			exact := traceDBRawAsyncExactKey{
				traceDBRawAsyncKey: key,
				Start:              rawPair.begin.TimestampNS,
				End:                rawPair.end.TimestampNS,
			}
			stored := &pair
			ledger.pairs[exact] = append(ledger.pairs[exact], stored)
			intervalKey := traceDBRawAsyncIntervalKey{
				Name: key.Name, Value: key.Value,
				Start: rawPair.begin.TimestampNS, End: rawPair.end.TimestampNS,
			}
			ledger.byInterval[intervalKey] = append(ledger.byInterval[intervalKey], stored)
			semanticKey := traceDBRawAsyncSemanticKey{
				Name: key.Name, Value: key.Value,
			}
			ledger.bySemantic[semanticKey] =
				append(ledger.bySemantic[semanticKey], stored)
			nameIntervalKey := traceDBRawAsyncNameIntervalKey{
				Name: key.Name, Start: rawPair.begin.TimestampNS,
				End: rawPair.end.TimestampNS,
			}
			ledger.byNameInterval[nameIntervalKey] =
				append(ledger.byNameInterval[nameIntervalKey], stored)
			valueIntervalKey := traceDBRawAsyncValueIntervalKey{
				Value: key.Value, Start: rawPair.begin.TimestampNS,
				End: rawPair.end.TimestampNS,
			}
			ledger.byValueInterval[valueIntervalKey] =
				append(ledger.byValueInterval[valueIntervalKey], stored)
			timeIntervalKey := traceDBRawAsyncTimeIntervalKey{
				Start: rawPair.begin.TimestampNS, End: rawPair.end.TimestampNS,
			}
			ledger.byTimeInterval[timeIntervalKey] =
				append(ledger.byTimeInterval[timeIntervalKey], stored)
			ledger.metrics["pairs_matchable"]++
		}
	}
	ledger.state = "complete_match_only"
	return ledger
}

func traceDBRawAsyncRecordReason(row traceDBRawMarkerRecord, key traceDBRawAsyncKey) string {
	verdict := tracequery.DecodeTraceMarkEndpointPayload(row.Buffer)
	switch {
	case !row.Admitted || row.RejectReason != "":
		return "endpoint_not_admitted"
	case row.Action != "S" && row.Action != "F":
		return "invalid_action"
	case row.PayloadPID <= 0 || row.PayloadPID > math.MaxInt32:
		return "invalid_payload_pid"
	case row.HeaderPID <= 0 || row.HeaderPID > math.MaxInt32:
		return "invalid_header_pid"
	case row.TimestampNS > math.MaxInt64:
		return "timestamp_overflow"
	case !validTraceDBCPUIndex(int64(row.CPU)):
		return "invalid_cpu"
	case row.Flags < 0 || row.Flags > math.MaxUint8:
		return "invalid_flags"
	case row.PreemptCount < 0 || row.PreemptCount > math.MaxUint8:
		return "invalid_preempt_count"
	case !traceDBCallstackSpanName(row.Name):
		return "invalid_span_name"
	case !traceDBCallstackMarkerToken(row.Value):
		return "invalid_cookie"
	case key.PayloadPID != row.PayloadPID || key.Name != row.Name || key.Value != row.Value:
		return "key_mismatch"
	case !verdict.Admitted || verdict.Action != row.Action ||
		int64(verdict.SpanPID) != row.PayloadPID ||
		verdict.Name != row.Name || verdict.Value != row.Value:
		return "payload_verdict_mismatch"
	default:
		return ""
	}
}

func traceDBRawAsyncPairFromRecords(
	raw traceDBRawMarkerPair,
	authority traceDBSchedulerAuthority,
) (traceDBRawAsyncPair, string) {
	begin, end := raw.begin, raw.end
	switch {
	case begin.Action != "S" || end.Action != "F":
		return traceDBRawAsyncPair{}, "action_mismatch"
	case begin.PayloadPID != end.PayloadPID ||
		begin.Name != end.Name || begin.Value != end.Value:
		return traceDBRawAsyncPair{}, "key_drift"
	case end.TimestampNS < begin.TimestampNS:
		return traceDBRawAsyncPair{}, "reversed_interval"
	case begin.TimestampNS > math.MaxInt64 || end.TimestampNS > math.MaxInt64:
		return traceDBRawAsyncPair{}, "timestamp_overflow"
	case !traceDBWireIntervalRepresentable(
		int64(begin.TimestampNS), int64(end.TimestampNS)):
		return traceDBRawAsyncPair{}, "unrepresentable_interval"
	}
	beginThread, beginProcess, reason := traceDBResolveRawPublicTID(
		authority, begin.HeaderPID, int64(begin.TimestampNS))
	if reason != "" {
		return traceDBRawAsyncPair{}, "begin_" + reason
	}
	endThread, endProcess, reason := traceDBResolveRawPublicTID(
		authority, end.HeaderPID, int64(end.TimestampNS))
	if reason != "" {
		return traceDBRawAsyncPair{}, "end_" + reason
	}
	if beginProcess.PID <= 0 || endProcess.PID <= 0 {
		return traceDBRawAsyncPair{}, "invalid_emitter_process"
	}
	return traceDBRawAsyncPair{
		begin: begin, end: end,
		beginThread: beginThread, beginProcess: beginProcess,
		endThread: endThread, endProcess: endProcess,
	}, ""
}

func (ledger *traceDBRawAsyncMatchLedger) claim(
	row traceDBCallstackRow,
) (*traceDBRawAsyncPair, bool) {
	if ledger == nil || ledger.state != "complete_match_only" {
		return nil, false
	}
	key := traceDBRawAsyncKey{
		PayloadPID: row.TGID,
		Name:       row.Name,
		Value:      row.Cookie,
	}
	exact := traceDBRawAsyncExactKey{
		traceDBRawAsyncKey: key,
		Start:              uint64(row.TS),
		End:                uint64(row.End),
	}
	exactPairs := ledger.pairs[exact]
	if len(exactPairs) == 0 {
		intervalPairs := ledger.byInterval[traceDBRawAsyncIntervalKey{
			Name: row.Name, Value: row.Cookie,
			Start: exact.Start, End: exact.End,
		}]
		if len(intervalPairs) > 0 {
			pair, mismatchClass := ledger.claimUniquePhysicalEnvelope(row, intervalPairs)
			if pair != nil {
				pair.claimed = true
				ledger.metrics["pairs_claimed"]++
				ledger.metrics["official_intervals_namespace_payload_pid_joined"]++
				return pair, true
			}
			if mismatchClass != "" {
				ledger.noteMismatchWitness(row, mismatchClass, intervalPairs)
				ledger.metrics["official_intervals_without_exact_raw_pair"]++
				return nil, false
			}
		}
		nameDriftPairs := ledger.byValueInterval[traceDBRawAsyncValueIntervalKey{
			Value: row.Cookie, Start: exact.Start, End: exact.End,
		}]
		if len(nameDriftPairs) > 0 {
			pair, mismatchClass :=
				ledger.claimUniquePhysicalEnvelope(row, nameDriftPairs)
			if pair != nil {
				pair.claimed = true
				ledger.metrics["pairs_claimed"]++
				ledger.metrics["official_intervals_exact_name_drift_joined"]++
				return pair, true
			}
			if mismatchClass != "" {
				ledger.noteMismatchWitness(row, mismatchClass, nameDriftPairs)
				ledger.metrics["official_intervals_without_exact_raw_pair"]++
				return nil, false
			}
		}
		ledger.noteMissingExactKey(row)
		ledger.metrics["official_intervals_without_exact_raw_pair"]++
		return nil, false
	}
	pair, mismatchClass := ledger.claimUniquePhysicalEnvelope(row, exactPairs)
	if pair != nil {
		pair.claimed = true
		ledger.metrics["pairs_claimed"]++
		return pair, true
	}
	ledger.noteMismatchWitness(row, mismatchClass, exactPairs)
	ledger.metrics["official_intervals_without_exact_raw_pair"]++
	return nil, false
}

func (ledger *traceDBRawAsyncMatchLedger) claimUniquePhysicalEnvelope(
	row traceDBCallstackRow,
	pairs []*traceDBRawAsyncPair,
) (*traceDBRawAsyncPair, string) {
	var matches []*traceDBRawAsyncPair
	unclaimed := 0
	beginTIDMatched := 0
	beginTGIDMatched := 0
	beginCPUMatched := 0
	for _, pair := range pairs {
		if pair.claimed {
			continue
		}
		unclaimed++
		if pair.beginThread.TID != row.TID {
			continue
		}
		beginTIDMatched++
		if pair.beginProcess.PID != row.HeaderTGID {
			continue
		}
		beginTGIDMatched++
		if row.CPUPlacement == traceDBSyncSpanCPUPlacementKnown &&
			int64(pair.begin.CPU) != row.StartCPU {
			continue
		}
		beginCPUMatched++
		matches = append(matches, pair)
	}
	switch len(matches) {
	case 0:
		class := ""
		switch {
		case unclaimed == 0:
			ledger.metrics["official_intervals_exact_pair_already_claimed"]++
			class = "exact_pair_already_claimed"
		case beginTIDMatched == 0:
			ledger.metrics["official_intervals_begin_tid_mismatch"]++
			class = "begin_tid_mismatch"
		case beginTGIDMatched == 0:
			ledger.metrics["official_intervals_begin_tgid_mismatch"]++
			class = "begin_tgid_mismatch"
		case beginCPUMatched == 0:
			ledger.metrics["official_intervals_begin_cpu_mismatch"]++
			class = "begin_cpu_mismatch"
		default:
			ledger.metrics["official_intervals_unclassified_exact_pair_mismatch"]++
			class = "unclassified_exact_pair_mismatch"
		}
		return nil, class
	case 1:
		return matches[0], ""
	default:
		ledger.metrics["official_intervals_ambiguous_exact_raw_pair"]++
		return nil, "ambiguous_exact_raw_pair"
	}
}

func (ledger *traceDBRawAsyncMatchLedger) noteMissingExactKey(
	row traceDBCallstackRow,
) {
	if ledger == nil {
		return
	}
	start, end := uint64(row.TS), uint64(row.End)
	nameMatches := ledger.byNameInterval[traceDBRawAsyncNameIntervalKey{
		Name: row.Name, Start: start, End: end,
	}]
	valueMatches := ledger.byValueInterval[traceDBRawAsyncValueIntervalKey{
		Value: row.Cookie, Start: start, End: end,
	}]
	switch {
	case len(nameMatches) > 0 && len(valueMatches) > 0:
		ledger.metrics["official_intervals_key_dimension_ambiguous"]++
		ledger.noteMismatchWitness(row, "key_dimension_ambiguous",
			appendTraceDBRawAsyncWitnessCandidates(nameMatches, valueMatches))
		return
	case len(nameMatches) > 0:
		ledger.metrics["official_intervals_cookie_mismatch"]++
		ledger.noteMismatchWitness(row, "cookie_mismatch", nameMatches)
		return
	case len(valueMatches) > 0:
		ledger.metrics["official_intervals_name_mismatch"]++
		ledger.noteMismatchWitness(row, "name_mismatch", valueMatches)
		return
	}

	base := ledger.bySemantic[traceDBRawAsyncSemanticKey{
		Name: row.Name, Value: row.Cookie,
	}]
	if len(base) == 0 {
		timeMatches := ledger.byTimeInterval[traceDBRawAsyncTimeIntervalKey{
			Start: start, End: end,
		}]
		if len(timeMatches) > 0 {
			ledger.metrics["official_intervals_name_cookie_mismatch"]++
			ledger.noteMismatchWitness(row, "name_cookie_mismatch", timeMatches)
			return
		}
		ledger.metrics["official_intervals_no_semantic_or_interval_candidate"]++
		ledger.noteMismatchWitness(row, "no_semantic_or_interval_candidate", nil)
		return
	}
	startMatched := false
	endMatched := false
	for _, pair := range base {
		if pair == nil {
			continue
		}
		if pair.begin.TimestampNS == start {
			startMatched = true
		}
		if pair.end.TimestampNS == end {
			endMatched = true
		}
	}
	switch {
	case startMatched && endMatched:
		ledger.metrics["official_intervals_interval_endpoint_ambiguous"]++
		ledger.noteMismatchWitness(row, "interval_endpoint_ambiguous", base)
	case !startMatched && endMatched:
		ledger.metrics["official_intervals_start_mismatch"]++
		ledger.noteMismatchWitness(row, "start_mismatch", base)
	case startMatched && !endMatched:
		ledger.metrics["official_intervals_end_mismatch"]++
		ledger.noteMismatchWitness(row, "end_mismatch", base)
	default:
		ledger.metrics["official_intervals_interval_mismatch"]++
		ledger.noteMismatchWitness(row, "interval_mismatch", base)
	}
}

func appendTraceDBRawAsyncWitnessCandidates(
	left, right []*traceDBRawAsyncPair,
) []*traceDBRawAsyncPair {
	out := make([]*traceDBRawAsyncPair, 0, len(left)+len(right))
	seen := map[*traceDBRawAsyncPair]bool{}
	for _, group := range [][]*traceDBRawAsyncPair{left, right} {
		for _, pair := range group {
			if pair == nil || seen[pair] {
				continue
			}
			seen[pair] = true
			out = append(out, pair)
		}
	}
	return out
}

func (ledger *traceDBRawAsyncMatchLedger) noteMismatchWitness(
	row traceDBCallstackRow,
	class string,
	candidates []*traceDBRawAsyncPair,
) {
	if ledger == nil {
		return
	}
	ledger.mismatchWitnessTotal++
	if len(ledger.mismatchWitnesses) >= traceDBRawAsyncMismatchWitnessCap {
		return
	}
	raw := "none"
	for _, pair := range candidates {
		if pair == nil {
			continue
		}
		raw = fmt.Sprintf(
			"pid=%d/name=%s/cookie=%s/start_ns=%d/end_ns=%d/begin_tid=%d/begin_tgid=%d/begin_cpu=%d",
			pair.begin.PayloadPID, traceDBRawMarkerNameWitness(pair.begin.Name),
			traceDBRawMarkerNameWitness(pair.begin.Value),
			pair.begin.TimestampNS, pair.end.TimestampNS,
			pair.beginThread.TID, pair.beginProcess.PID, pair.begin.CPU)
		break
	}
	startCPU := "unavailable"
	if row.CPUPlacement == traceDBSyncSpanCPUPlacementKnown {
		startCPU = fmt.Sprintf("%d", row.StartCPU)
	}
	ledger.mismatchWitnesses = append(ledger.mismatchWitnesses, fmt.Sprintf(
		"row_id=%d/class=%s/db_pid=%d/db_name=%s/db_cookie=%s/start_ns=%d/end_ns=%d/begin_tid=%d/begin_tgid=%d/begin_cpu=%s/raw_candidates=%d/raw={%s}",
		row.ID, class, row.TGID, traceDBRawMarkerNameWitness(row.Name),
		traceDBRawMarkerNameWitness(row.Cookie), row.TS, row.End,
		row.TID, row.HeaderTGID, startCPU, len(candidates), raw))
}

func (pair *traceDBRawAsyncPair) publish(sink *traceDBRowSink) error {
	if pair == nil {
		return &traceDBOutputInvariantError{Reason: "missing_raw_async_pair"}
	}
	begin, err := prepareTraceDBRenderedRowEnvelope(
		int64(pair.begin.TimestampNS), sink.stats.RowsAccepted,
		traceDBCommName(pair.beginThread.Name, "unknown"),
		pair.beginThread.TID, pair.beginProcess.PID, int64(pair.begin.CPU),
		pair.begin.Flags, pair.begin.PreemptCount, false,
		"tracing_mark_write: "+pair.begin.Buffer)
	if err != nil {
		return err
	}
	end, err := prepareTraceDBRenderedRowEnvelope(
		int64(pair.end.TimestampNS), sink.stats.RowsAccepted+1,
		traceDBCommName(pair.endThread.Name, "unknown"),
		pair.endThread.TID, pair.endProcess.PID, int64(pair.end.CPU),
		pair.end.Flags, pair.end.PreemptCount, false,
		"tracing_mark_write: "+pair.end.Buffer)
	if err != nil {
		return err
	}
	if err := sink.add(begin); err != nil {
		return err
	}
	return sink.add(end)
}

func (ledger *traceDBRawAsyncMatchLedger) applyCoverage(coverage *TraceDBCoverage) {
	if ledger == nil || coverage == nil {
		return
	}
	if coverage.Metadata == nil {
		coverage.Metadata = map[string]string{}
	}
	coverage.Metadata["raw_async_replacement_state"] = ledger.state
	for key, value := range ledger.metrics {
		traceDBAddCoverageMetric(coverage, "raw_async_"+key, value)
	}
	coverage.Metadata["raw_async_mismatch_census"] = "not_evaluated"
	if ledger.state == "complete_match_only" {
		if int64(ledger.mismatchWitnessTotal) ==
			ledger.metrics["official_intervals_without_exact_raw_pair"] {
			coverage.Metadata["raw_async_mismatch_census"] = "complete"
		} else {
			coverage.Metadata["raw_async_mismatch_census"] =
				"not_evaluated_count_mismatch"
		}
	}
	if len(ledger.mismatchWitnesses) > 0 {
		coverage.Metadata["raw_async_mismatch_witnesses"] =
			strings.Join(ledger.mismatchWitnesses, ";")
		traceDBAddCoverageMetric(coverage,
			"raw_async_mismatch_witnesses_emitted",
			int64(len(ledger.mismatchWitnesses)))
		if omitted := ledger.mismatchWitnessTotal -
			len(ledger.mismatchWitnesses); omitted > 0 {
			traceDBAddCoverageMetric(coverage,
				"raw_async_mismatch_witnesses_omitted", int64(omitted))
		}
		if coverage.FieldSources == nil {
			coverage.FieldSources = map[string]string{}
		}
		coverage.FieldSources["raw_async_mismatch_witnesses"] =
			"bounded exact DB interval and first raw comparison candidate after dimensioned matching; diagnostic only, never fuzzy join or endpoint authority"
	}
	unclaimed := ledger.metrics["pairs_matchable"] - ledger.metrics["pairs_claimed"]
	if unclaimed > 0 {
		traceDBAddCoverageMetric(coverage, "raw_async_pairs_unclaimed", unclaimed)
	}
}

func (ledger *traceDBRawAsyncMatchLedger) String() string {
	if ledger == nil {
		return "raw async ledger <nil>"
	}
	return fmt.Sprintf("state=%s matchable=%d claimed=%d",
		ledger.state, ledger.metrics["pairs_matchable"], ledger.metrics["pairs_claimed"])
}
