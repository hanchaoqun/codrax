package hitraceconv

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceDBEBPFCallchainAuthority struct {
	Available map[int64]bool
	Coverage  TraceDBCoverage
}

type traceDBEBPFCommonRow struct {
	StableID   int64
	Start      int64
	End        int64
	Duration   int64
	TypeID     uint64
	IPID       int64
	ITID       int64
	Callchain  int64
	Identity   string
	PublicPID  int64
	PublicTID  int64
	CallStatus string
}

func exportTraceDBEBPFIntervals(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	authority traceDBSchedulerAuthority,
) ([]TraceDBCoverage, error) {
	callchains, err := loadTraceDBEBPFCallchainAuthority(ctx, tdb)
	coverage := []TraceDBCoverage{callchains.Coverage}
	if err != nil {
		return coverage, err
	}
	exporters := []func(context.Context, *traceDB, *traceDBRowSink, traceDBSchedulerAuthority, traceDBEBPFCallchainAuthority) (TraceDBCoverage, error){
		exportTraceDBEBPFFilesystem,
		exportTraceDBEBPFPagedMemory,
		exportTraceDBEBPFBIOLatency,
	}
	for _, exporter := range exporters {
		item, exportErr := exporter(ctx, tdb, sink, authority, callchains)
		coverage = append(coverage, item)
		if exportErr != nil {
			return coverage, exportErr
		}
	}
	return coverage, nil
}

func loadTraceDBEBPFCallchainAuthority(ctx context.Context, tdb *traceDB) (traceDBEBPFCallchainAuthority, error) {
	coverage, err := tdb.inspectCoverage(ctx, "resolver.ebpf", "ebpf_callstack",
		[]string{"id", "callchain_id", "depth"})
	coverage.FieldSources = map[string]string{
		"identity":     "official ebpf_callstack.id signed-int32 projection",
		"callchain":    "strict uint32 callchain_id",
		"completeness": "one unique row identity and one unique contiguous zero-based depth per callchain",
		"semantics":    "resolver only; frame IP/symbol/path bytes remain losslessly carried by sql_text_fidelity",
	}
	result := traceDBEBPFCallchainAuthority{Available: map[int64]bool{}, Coverage: coverage}
	fail := func(cause error) (traceDBEBPFCallchainAuthority, error) {
		if cause != nil {
			result.Coverage.Error = cause.Error()
		}
		return result, cause
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return fail(err)
	}
	rows, err := tdb.db.QueryContext(ctx, `SELECT id, callchain_id, depth FROM ebpf_callstack ORDER BY callchain_id, depth, id`)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	type frame struct {
		rowID, chainID, depth int64
		valid                 bool
	}
	var frames []frame
	rowCounts := map[int64]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		var idRaw, chainRaw, depthRaw any
		if err := rows.Scan(&idRaw, &chainRaw, &depthRaw); err != nil {
			return fail(err)
		}
		item := frame{}
		var idOK, chainOK, depthOK bool
		item.rowID, idOK = traceDBActivityITIDSignedInt32.decodeStableRowID(idRaw)
		item.chainID, chainOK = traceDBStrictSQLiteInt(chainRaw)
		item.depth, depthOK = traceDBStrictSQLiteInt(depthRaw)
		item.valid = idOK && chainOK && depthOK &&
			item.chainID >= 0 && item.chainID <= math.MaxUint32 &&
			item.depth >= 0 && item.depth <= math.MaxUint32
		if idOK {
			rowCounts[item.rowID]++
		}
		frames = append(frames, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	depths := map[int64]map[int64]bool{}
	poisoned := map[int64]bool{}
	for _, item := range frames {
		if !item.valid {
			if item.chainID >= 0 {
				poisoned[item.chainID] = true
			}
			continue
		}
		if rowCounts[item.rowID] != 1 {
			poisoned[item.chainID] = true
			continue
		}
		if depths[item.chainID] == nil {
			depths[item.chainID] = map[int64]bool{}
		}
		if depths[item.chainID][item.depth] {
			poisoned[item.chainID] = true
		}
		depths[item.chainID][item.depth] = true
	}
	for chainID, chainDepths := range depths {
		if poisoned[chainID] {
			continue
		}
		for depth := int64(0); depth < int64(len(chainDepths)); depth++ {
			if !chainDepths[depth] {
				poisoned[chainID] = true
				break
			}
		}
		if !poisoned[chainID] {
			result.Available[chainID] = true
		}
	}
	result.Coverage.RowsEmitted = len(result.Available)
	result.Coverage.Metrics = map[string]int64{
		"callchains_seen":      int64(len(depths)),
		"callchains_available": int64(len(result.Available)),
		"callchains_poisoned":  int64(len(poisoned)),
	}
	if len(poisoned) > 0 {
		result.Coverage.Skipped = fmt.Sprintf("incomplete_or_ambiguous_callchains=%d", len(poisoned))
	}
	return result, nil
}

func exportTraceDBEBPFFilesystem(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	authority traceDBSchedulerAuthority, callchains traceDBEBPFCallchainAuthority,
) (TraceDBCoverage, error) {
	required := []string{"id", "callchain_id", "type", "ipid", "itid", "start_ts", "end_ts", "dur",
		"return_value", "error_code", "fd", "file_id", "size",
		"first_argument", "second_argument", "third_argument", "fourth_argument"}
	coverage, err := inspectTraceDBEBPFCoverage(ctx, tdb, "file_system_sample", required)
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT id, callchain_id, type, ipid, itid, start_ts, end_ts, dur,
		       return_value, error_code, fd, file_id, size,
		       first_argument, second_argument, third_argument, fourth_argument
		FROM file_system_sample ORDER BY start_ts, id`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	rowCounts := map[int64]int{}
	type candidate struct {
		common  traceDBEBPFCommonRow
		details tracequery.OfficialEBPFFilesystemDetails
	}
	candidates := map[int64]candidate{}
	for rows.Next() {
		var raw [17]any
		if err := rows.Scan(&raw[0], &raw[1], &raw[2], &raw[3], &raw[4], &raw[5], &raw[6], &raw[7],
			&raw[8], &raw[9], &raw[10], &raw[11], &raw[12], &raw[13], &raw[14], &raw[15], &raw[16]); err != nil {
			return traceDBCoverageFailure(coverage, err)
		}
		common, reason := prepareTraceDBEBPFCommon(authority, callchains, raw[:8])
		if common.StableID >= 0 {
			rowCounts[common.StableID]++
		}
		if reason != "" || common.TypeID > 3 {
			if reason == "" {
				reason = "invalid_type"
			}
			skipped[reason]++
			continue
		}
		details := tracequery.OfficialEBPFFilesystemDetails{}
		if details.ReturnValue, details.ReturnValueKnown, reason = traceDBOptionalEBPFText(raw[8]); reason != "" {
			skipped[reason]++
			continue
		}
		if details.ErrorCode, details.ErrorCodeKnown, reason = traceDBOptionalEBPFText(raw[9]); reason != "" {
			skipped[reason]++
			continue
		}
		if details.FD, details.FDKnown, reason = traceDBOptionalEBPFSigned(raw[10]); reason != "" {
			skipped[reason]++
			continue
		}
		if details.FileID, details.FileIDKnown, reason = traceDBOptionalEBPFUnsigned(raw[11]); reason != "" {
			skipped[reason]++
			continue
		}
		if details.SizeBytes, details.SizeKnown, reason = traceDBOptionalEBPFUnsigned(raw[12]); reason != "" {
			skipped[reason]++
			continue
		}
		validDetails := true
		for i := 0; i < 4; i++ {
			value, known, detailReason := traceDBOptionalEBPFText(raw[13+i])
			if detailReason != "" {
				skipped[detailReason]++
				validDetails = false
				break
			}
			details.Arguments[i] = value
			if known {
				details.ArgumentKnownMask |= 1 << i
			}
		}
		if !validDetails {
			continue
		}
		candidates[common.StableID] = candidate{common: common, details: details}
	}
	if err := rows.Err(); err != nil {
		return traceDBCoverageFailure(coverage, err)
	}
	return emitTraceDBEBPFCandidates(sink, coverage, tracequery.EBPFFamilyFilesystem,
		rowCounts, skipped, candidates, func(item candidate) (traceDBEBPFCommonRow, any) {
			return item.common, item.details
		})
}

func exportTraceDBEBPFPagedMemory(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	authority traceDBSchedulerAuthority, callchains traceDBEBPFCallchainAuthority,
) (TraceDBCoverage, error) {
	required := []string{"id", "callchain_id", "type", "ipid", "itid", "start_ts", "end_ts", "dur", "size", "addr"}
	coverage, err := inspectTraceDBEBPFCoverage(ctx, tdb, "paged_memory_sample", required)
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT id, callchain_id, type, ipid, itid, start_ts, end_ts, dur, size, addr
		FROM paged_memory_sample ORDER BY start_ts, id`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	rowCounts := map[int64]int{}
	type candidate struct {
		common  traceDBEBPFCommonRow
		details tracequery.OfficialEBPFPagedMemoryDetails
	}
	candidates := map[int64]candidate{}
	for rows.Next() {
		var raw [10]any
		if err := rows.Scan(&raw[0], &raw[1], &raw[2], &raw[3], &raw[4],
			&raw[5], &raw[6], &raw[7], &raw[8], &raw[9]); err != nil {
			return traceDBCoverageFailure(coverage, err)
		}
		common, reason := prepareTraceDBEBPFCommon(authority, callchains, raw[:8])
		if common.StableID >= 0 {
			rowCounts[common.StableID]++
		}
		if reason != "" || common.TypeID >= math.MaxUint16 {
			if reason == "" {
				reason = "invalid_type"
			}
			skipped[reason]++
			continue
		}
		details := tracequery.OfficialEBPFPagedMemoryDetails{}
		if details.SizeBytes, details.SizeKnown, reason = traceDBOptionalEBPFUnsigned(raw[8]); reason != "" {
			skipped[reason]++
			continue
		}
		if details.Address, details.AddressKnown, reason = traceDBOptionalEBPFText(raw[9]); reason != "" {
			skipped[reason]++
			continue
		}
		candidates[common.StableID] = candidate{common: common, details: details}
	}
	if err := rows.Err(); err != nil {
		return traceDBCoverageFailure(coverage, err)
	}
	return emitTraceDBEBPFCandidates(sink, coverage, tracequery.EBPFFamilyPagedMemory,
		rowCounts, skipped, candidates, func(item candidate) (traceDBEBPFCommonRow, any) {
			return item.common, item.details
		})
}

func exportTraceDBEBPFBIOLatency(ctx context.Context, tdb *traceDB, sink *traceDBRowSink,
	authority traceDBSchedulerAuthority, callchains traceDBEBPFCallchainAuthority,
) (TraceDBCoverage, error) {
	required := []string{"id", "callchain_id", "type", "ipid", "itid", "start_ts", "end_ts",
		"latency_dur", "tier", "size", "block_number", "path_id", "dur_per_4k"}
	coverage, err := inspectTraceDBEBPFCoverage(ctx, tdb, "bio_latency_sample", required)
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT id, callchain_id, type, ipid, itid, start_ts, end_ts, latency_dur,
		       tier, size, block_number, path_id, dur_per_4k
		FROM bio_latency_sample ORDER BY start_ts, id`)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	rowCounts := map[int64]int{}
	type candidate struct {
		common  traceDBEBPFCommonRow
		details tracequery.OfficialEBPFBIOLatencyDetails
	}
	candidates := map[int64]candidate{}
	for rows.Next() {
		var raw [13]any
		if err := rows.Scan(&raw[0], &raw[1], &raw[2], &raw[3], &raw[4], &raw[5], &raw[6],
			&raw[7], &raw[8], &raw[9], &raw[10], &raw[11], &raw[12]); err != nil {
			return traceDBCoverageFailure(coverage, err)
		}
		common, reason := prepareTraceDBEBPFCommon(authority, callchains, raw[:8])
		if common.StableID >= 0 {
			rowCounts[common.StableID]++
		}
		if reason != "" {
			skipped[reason]++
			continue
		}
		details := tracequery.OfficialEBPFBIOLatencyDetails{}
		tier, known, detailReason := traceDBOptionalEBPFUnsigned(raw[8])
		if detailReason != "" || tier > math.MaxUint32 {
			skipped["invalid_optional_integer"]++
			continue
		}
		if known && tier == math.MaxUint32 {
			tier, known = 0, false
		}
		details.Tier, details.TierKnown = uint32(tier), known
		if details.SizeBytes, details.SizeKnown, reason = traceDBOptionalEBPFUnsigned(raw[9]); reason != "" {
			skipped[reason]++
			continue
		}
		if details.BlockNumber, details.BlockKnown, reason = traceDBOptionalEBPFText(raw[10]); reason != "" {
			skipped[reason]++
			continue
		}
		if details.PathID, details.PathIDKnown, reason = traceDBOptionalEBPFUnsignedTextCompat(raw[11]); reason != "" {
			skipped[reason]++
			continue
		}
		duration4K, durationKnown, detailReason := traceDBOptionalEBPFUnsigned(raw[12])
		if detailReason != "" {
			skipped[detailReason]++
			continue
		}
		if durationKnown && duration4K == math.MaxUint32 {
			duration4K, durationKnown = 0, false
		}
		details.DurationPer4K, details.Duration4KKnown = duration4K, durationKnown
		candidates[common.StableID] = candidate{common: common, details: details}
	}
	if err := rows.Err(); err != nil {
		return traceDBCoverageFailure(coverage, err)
	}
	return emitTraceDBEBPFCandidates(sink, coverage, tracequery.EBPFFamilyBIOLatency,
		rowCounts, skipped, candidates, func(item candidate) (traceDBEBPFCommonRow, any) {
			return item.common, item.details
		})
}

func inspectTraceDBEBPFCoverage(ctx context.Context, tdb *traceDB, table string,
	required []string,
) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "ebpf.interval", table, required)
	coverage.SourceTables = []string{table, "process", "thread", "ebpf_callstack"}
	coverage.FieldSources = map[string]string{
		"interval":     "official start/end/duration with exact end-start equality",
		"identity":     "official internal ipid/itid; public PID/TID only after shared identity and lifecycle resolution",
		"callchain":    "official callchain_id qualified by contiguous unique ebpf_callstack depths",
		"cpu":          "unavailable by schema; no physical CPU header is fabricated",
		"details":      "family-specific official SQL columns in canonical schema-checked JSON",
		"preservation": "rows rejected from typed semantics remain exact in sql_text_fidelity",
	}
	return coverage, err
}

func prepareTraceDBEBPFCommon(authority traceDBSchedulerAuthority,
	callchains traceDBEBPFCallchainAuthority, raw []any,
) (traceDBEBPFCommonRow, string) {
	row := traceDBEBPFCommonRow{StableID: -1, Callchain: -1}
	var ok bool
	if len(raw) != 8 {
		return row, "invalid_common_shape"
	}
	row.StableID, ok = traceDBActivityITIDSignedInt32.decodeStableRowID(raw[0])
	if !ok {
		return row, "invalid_row_identity"
	}
	row.Callchain, ok = traceDBStrictSQLiteInt(raw[1])
	if !ok || row.Callchain < -1 || row.Callchain > math.MaxUint32 {
		return row, "invalid_callchain_id"
	}
	typeSigned, typeOK := traceDBStrictSQLiteInt(raw[2])
	if !typeOK {
		return row, "invalid_type"
	}
	row.TypeID = uint64(typeSigned)
	row.IPID, ok = traceDBStrictInternalID(raw[3])
	if !ok {
		return row, "invalid_ipid"
	}
	row.ITID, ok = traceDBStrictInternalID(raw[4])
	if !ok {
		return row, "invalid_itid"
	}
	row.Start, ok = traceDBStrictSQLiteInt(raw[5])
	if !ok || row.Start < 0 {
		return row, "invalid_start"
	}
	row.End, ok = traceDBStrictSQLiteInt(raw[6])
	if !ok || row.End < row.Start {
		return row, "invalid_end"
	}
	row.Duration, ok = traceDBStrictSQLiteInt(raw[7])
	if !ok || row.Duration < 0 || row.End-row.Start != row.Duration {
		return row, "interval_mismatch"
	}
	if row.Callchain == -1 {
		row.CallStatus = "absent"
	} else if callchains.Available[row.Callchain] {
		row.CallStatus = "available"
	} else {
		row.CallStatus = "unavailable"
	}
	row.Identity = "unavailable"
	thread, process, resolution := authority.resolveThreadSubject(row.ITID)
	if resolution == traceDBSchedulerThreadResolved {
		if thread.IPID != row.IPID || process.IPID != row.IPID {
			row.Identity = "mismatch"
		} else if (row.Start == row.End && !authority.threadPointAllows(row.ITID, row.Start)) ||
			(row.Start != row.End && !authority.threadSourceIntervalAllows(row.ITID, row.Start, row.End)) {
			row.Identity = "lifecycle_rejected"
		} else if thread.TID > math.MaxInt32 || process.PID > math.MaxInt32 {
			row.Identity = "unavailable"
		} else {
			row.Identity = "resolved"
			row.PublicTID = thread.TID
			row.PublicPID = process.PID
		}
	}
	return row, ""
}

func emitTraceDBEBPFCandidates[T any](sink *traceDBRowSink, coverage TraceDBCoverage,
	family string, rowCounts map[int64]int, skipped map[string]int,
	candidates map[int64]T, unpack func(T) (traceDBEBPFCommonRow, any),
) (TraceDBCoverage, error) {
	ids := make([]int64, 0, len(candidates))
	for id, candidate := range candidates {
		if rowCounts[id] != 1 {
			skipped["duplicate_row_identity"]++
			continue
		}
		common, _ := unpack(candidate)
		if common.CallStatus == "unavailable" {
			coverage.Metrics = ensureTraceDBCoverageMetrics(coverage.Metrics)
			coverage.Metrics["unavailable_callchain_endpoints"]++
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, _ := unpack(candidates[ids[i]])
		right, _ := unpack(candidates[ids[j]])
		if left.Start != right.Start {
			return left.Start < right.Start
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		common, details := unpack(candidates[id])
		detailsBytes, err := json.Marshal(details)
		if err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		line, err := tracequery.FormatOfficialEBPFInterval(tracequery.OfficialEBPFInterval{
			Family: family, TimestampNS: uint64(common.Start), EndTimestampNS: uint64(common.End),
			DurationNS: uint64(common.Duration), SourceRow: uint32(common.StableID),
			TypeID: common.TypeID, InternalProcessID: uint32(common.IPID),
			InternalThreadID: uint32(common.ITID), PID: int(common.PublicPID), TID: int(common.PublicTID),
			IdentityStatus: common.Identity, CallchainID: common.Callchain,
			CallchainStatus: common.CallStatus, DetailsJSON: string(detailsBytes),
		})
		if err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		if len(line) > maxTraceDBSystraceLineBytes {
			skipped["typed_wire_line_too_long"]++
			continue
		}
		if err := addTraceDBTypedCommentRow(sink, common.Start, line); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		coverage.RowsEmitted++
	}
	coverage.Skipped = traceDBCountSummary(skipped)
	return coverage, nil
}

func traceDBOptionalEBPFText(raw any) (string, bool, string) {
	if raw == nil {
		return "", false, ""
	}
	value, ok := raw.(string)
	if !ok || !utf8.ValidString(value) {
		return "", false, "invalid_optional_text"
	}
	return value, true, ""
}

func traceDBOptionalEBPFSigned(raw any) (int64, bool, string) {
	if raw == nil {
		return 0, false, ""
	}
	value, ok := traceDBStrictSQLiteInt(raw)
	if !ok {
		return 0, false, "invalid_optional_integer"
	}
	return value, true, ""
}

func traceDBOptionalEBPFUnsigned(raw any) (uint64, bool, string) {
	value, known, reason := traceDBOptionalEBPFSigned(raw)
	if reason != "" || !known {
		return 0, known, reason
	}
	return uint64(value), true, ""
}

// path_id is declared TEXT by the official table but returned through an
// integer setter; exported SQLite builds therefore legitimately expose either
// INTEGER storage or canonical decimal TEXT. No other numeric field receives
// this compatibility.
func traceDBOptionalEBPFUnsignedTextCompat(raw any) (uint64, bool, string) {
	if raw == nil {
		return 0, false, ""
	}
	if text, ok := raw.(string); ok {
		if text == "" || (len(text) > 1 && text[0] == '0') {
			return 0, false, "invalid_optional_integer"
		}
		value, err := strconv.ParseUint(text, 10, 64)
		if err != nil || strconv.FormatUint(value, 10) != text {
			return 0, false, "invalid_optional_integer"
		}
		return value, true, ""
	}
	return traceDBOptionalEBPFUnsigned(raw)
}

func traceDBCoverageFailure(coverage TraceDBCoverage, err error) (TraceDBCoverage, error) {
	coverage.Error = err.Error()
	return coverage, err
}

func ensureTraceDBCoverageMetrics(metrics map[string]int64) map[string]int64 {
	if metrics == nil {
		return map[string]int64{}
	}
	return metrics
}
