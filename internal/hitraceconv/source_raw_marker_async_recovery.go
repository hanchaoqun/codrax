package hitraceconv

import (
	"fmt"
	"math"
	"sort"

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
	state   string
	pairs   map[traceDBRawAsyncExactKey][]*traceDBRawAsyncPair
	metrics map[string]int64
}

func newTraceDBRawAsyncMatchLedger(
	inventory *traceDBSourceNameInventory,
	authority traceDBSchedulerAuthority,
) *traceDBRawAsyncMatchLedger {
	ledger := &traceDBRawAsyncMatchLedger{
		state:   "unavailable",
		pairs:   map[traceDBRawAsyncExactKey][]*traceDBRawAsyncPair{},
		metrics: map[string]int64{},
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
			ledger.pairs[exact] = append(ledger.pairs[exact], &pair)
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
	exact := traceDBRawAsyncExactKey{
		traceDBRawAsyncKey: traceDBRawAsyncKey{
			PayloadPID: row.TGID,
			Name:       row.Name,
			Value:      row.Cookie,
		},
		Start: uint64(row.TS),
		End:   uint64(row.End),
	}
	var matches []*traceDBRawAsyncPair
	for _, pair := range ledger.pairs[exact] {
		if pair.claimed ||
			pair.beginThread.TID != row.TID ||
			pair.beginProcess.PID != row.HeaderTGID {
			continue
		}
		if row.CPUPlacement == traceDBSyncSpanCPUPlacementKnown &&
			int64(pair.begin.CPU) != row.StartCPU {
			continue
		}
		matches = append(matches, pair)
	}
	switch len(matches) {
	case 0:
		ledger.metrics["official_intervals_without_exact_raw_pair"]++
		return nil, false
	case 1:
		matches[0].claimed = true
		ledger.metrics["pairs_claimed"]++
		return matches[0], true
	default:
		ledger.metrics["official_intervals_ambiguous_exact_raw_pair"]++
		return nil, false
	}
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
