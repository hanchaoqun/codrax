package hitraceconv

import (
	"context"
	"fmt"
	"math"
)

func newTraceDBRawBlockedRecoveryCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_blocked_recovery",
		Table:  "__raw_only_blocked_reason__",
		Role:   "query_ready_export",
		FieldSources: map[string]string{
			"admission":     "RPD-2A exact content-multiset subset; only a raw content cohort with zero DB representation and exact target/header identity admission is eligible",
			"timestamp":     "exact source raw record timestamp",
			"cpu":           "exact source raw page CPU",
			"header_thread": "exact source common_pid resolved to one canonical host thread/process at the raw timestamp",
			"target_thread": "exact source payload pid resolved independently to one canonical host thread/process at the raw timestamp",
			"body":          "existing strict canonical sched_blocked_reason renderer over the admitted raw payload; optional delay and caller profile are preserved",
			"deduplication": "any content cohort represented in DB is wholly withheld; DB counts are never subtracted from raw counts",
			"namespace":     "no PID rewrite or namespace guess; absent, rejected, or ambiguous public identity remains unpublished",
		},
		Metadata: map[string]string{
			"publication_state": "unavailable",
		},
	}
}

func publishTraceDBRawBlockedRecovery(
	ctx context.Context,
	sink *traceDBRowSink,
	keyCoverage TraceDBCoverage,
	rows []traceDBRawBlockedRecoveryRow,
) (TraceDBCoverage, error) {
	out := newTraceDBRawBlockedRecoveryCoverage()
	out.Found = keyCoverage.Found
	if keyCoverage.Metadata["ledger_state"] != "exact_content_multiset_subset" {
		out.Metadata["publication_state"] = "withheld_key_ledger_not_exact"
		out.Skipped = "raw blocked recovery withheld: exact content-multiset subset ledger unavailable"
		return out, nil
	}
	out.RowsRead = len(rows)
	if int64(len(rows)) != keyCoverage.Metrics["raw_rows_absent_db_identity_admitted"] {
		out.Metadata["publication_state"] = "withheld_identity_census_mismatch"
		out.Skipped = "raw blocked recovery withheld: admitted identity census mismatch"
		return out, nil
	}
	if len(rows) > 0 && sink == nil {
		err := &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
		out.Error = err.Error()
		return out, err
	}
	if sink != nil && (sink.stats.RowsAccepted < 0 ||
		len(rows) > math.MaxInt-sink.stats.RowsAccepted) {
		err := &traceDBOutputInvariantError{Reason: "invalid_raw_blocked_recovery_sequence"}
		out.Error = err.Error()
		return out, err
	}
	prepared := make([]renderedRow, 0, len(rows))
	for index, item := range rows {
		raw := item.Raw
		if _, ok := traceDBRawBlockedComparableKey(raw); !ok ||
			item.TargetThread.TID != raw.TargetTID ||
			item.HeaderThread.TID != raw.HeaderPID ||
			item.TargetThread.IPID != item.TargetProcess.IPID ||
			item.HeaderThread.IPID != item.HeaderProcess.IPID {
			err := &traceDBOutputInvariantError{Reason: "invalid_raw_blocked_recovery_identity_tuple"}
			out.Error = err.Error()
			return out, err
		}
		payload := coreRenderPayload{
			Kind: coreRenderBlocked,
			Blocked: &coreBlockedPayload{
				PID: raw.TargetTID, IOWait: uint64(raw.IOWait),
				CallerRaw: raw.CallerRaw, Caller: raw.Caller,
				CallerSymbolized: raw.CallerSymbolized,
				CNodeIndex:       raw.CNodeIndex, CNodeKnown: raw.CNodeKnown,
				Delay: raw.Delay, DelayKnown: raw.DelayKnown,
			},
		}
		body, ok := renderCanonicalCorePayload(payload)
		if !ok {
			return out, &traceDBOutputInvariantError{Reason: "invalid_raw_blocked_recovery_body"}
		}
		body = "sched_blocked_reason: " + body +
			" source=official_rawtrace_rpd2a raw_db_content_cohort=absent"
		headerTGID := item.HeaderProcess.PID
		allowUnknownTGID := headerTGID == 0 && item.HeaderThread.TID != 0
		sequence := sink.stats.RowsAccepted + index
		row, err := prepareTraceDBRenderedRowEnvelopeContext(
			ctx, int64(raw.TimestampNS), sequence,
			traceDBCommName(item.HeaderThread.Name, "unknown"),
			item.HeaderThread.TID, headerTGID, int64(raw.CPU),
			raw.Flags, raw.PreemptCount, allowUnknownTGID, body)
		if err != nil {
			out.Error = err.Error()
			return out, err
		}
		prepared = append(prepared, row)
	}
	for _, row := range prepared {
		if err := sink.addContext(ctx, row, nil, false); err != nil {
			out.Error = err.Error()
			return out, err
		}
		out.RowsEmitted++
	}
	if out.RowsEmitted == 0 {
		out.Metadata["publication_state"] = "complete_no_eligible_raw_only_row"
		out.Skipped = "raw blocked recovery complete: no DB-disjoint identity-admitted row"
	} else {
		out.Metadata["publication_state"] = "published_exact_raw_only_content_cohorts"
		out.Metadata["publication_contract"] = fmt.Sprintf(
			"rows=%d; exact raw timestamp/cpu/flags/header and strict blocked body", out.RowsEmitted)
	}
	return out, nil
}
