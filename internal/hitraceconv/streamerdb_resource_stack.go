package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// This route is independent of NativeHook's legacy I/C scheduler admission.
// A resource event has a proven source owner but need not have a CPU witness.
func exportTraceDBResourceStacks(ctx context.Context, tdb *traceDB, sink *traceDBRowSink, authority traceDBSchedulerAuthority) (TraceDBCoverage, error) {
	c, err := tdb.inspectCoverage(ctx, "resource_stack", "native_hook", []string{"start_ts", "event_type", "itid", "ipid", "callchain_id"})
	if err != nil || !c.Found || len(c.ColumnsMissing) > 0 {
		return c, err
	}
	c.FieldSources = map[string]string{"owner": "same sealed capture native_hook.itid/ipid plus thread/process lifecycle point; CPU unavailable is permitted", "frames": "same capture exact source callchain; physical rows, raw depth and nullable values preserved; no execution or causal authority", "protocol": "OpenHarmony 5c5afb0c native_hook{,_frame}_table.cpp: callchain uint32 sentinel excluded; int64 address bits and SQL NULL retained"}
	frames, frameAvailable, err := loadTraceDBResourceFrames(ctx, tdb)
	if err != nil {
		return c, err
	}
	stable, _, err := traceDBHiddenRowIDExpr(ctx, tdb.db, "native_hook")
	if err != nil {
		c.Skipped = "stable_resource_row_unavailable=1"
		return c, nil
	}
	columns, err := traceDBColumnNames(ctx, tdb.db, "native_hook")
	if err != nil {
		return c, err
	}
	optional := []string{"id", "addr", "heap_size", "end_ts"}
	selects := resourceOptionalSelects(columns, optional)
	rows, err := tdb.db.QueryContext(ctx, `SELECT `+stable+`,start_ts,event_type,itid,ipid,callchain_id,`+strings.Join(selects, ",")+` FROM native_hook ORDER BY `+stable)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	skipped := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return c, err
		}
		var rowRaw, tsRaw, opRaw, itidRaw, ipidRaw, callRaw any
		var raw [4]any
		if err := rows.Scan(&rowRaw, &tsRaw, &opRaw, &itidRaw, &ipidRaw, &callRaw, &raw[0], &raw[1], &raw[2], &raw[3]); err != nil {
			return c, err
		}
		row, rowOK := traceDBStrictSQLiteInt(rowRaw)
		ts, tsOK := traceDBStrictSQLiteInt(tsRaw)
		itid, itidOK := traceDBStrictInternalID(itidRaw)
		ipid, ipidOK := traceDBStrictInternalID(ipidRaw)
		op, _, opOK := traceDBNativeHookEventType(opRaw)
		if !rowOK || !tsOK || ts < 0 || !itidOK || !ipidOK || itid <= 0 || ipid <= 0 || !opOK {
			skipped["invalid_resource_event"]++
			continue
		}
		thread, process, resolved := authority.resolveThreadSubject(itid)
		if resolved != traceDBSchedulerThreadResolved || thread.IPID != ipid || process.IPID != ipid || thread.TID <= 0 || thread.TID > math.MaxInt32 || process.PID <= 0 || process.PID > math.MaxInt32 || traceDBBeforeCaptureStart(authority.identities, ts) || !authority.threadPointAllows(itid, ts) {
			skipped["unproven_resource_owner"]++
			continue
		}
		e := tracewire.ResourceEvent{PID: int(process.PID), TID: int(thread.TID), IPID: ipid, ITID: itid, Thread: thread.Name, Operation: op, CallchainID: resourceSQLScalar(callRaw, true)}
		e.SourceID, e.Address, e.Size, e.EndNS = resourceSQLScalar(raw[0], selects[0] != "NULL"), resourceSQLScalar(raw[1], selects[1] != "NULL"), resourceSQLScalar(raw[2], selects[2] != "NULL"), resourceSQLScalar(raw[3], selects[3] != "NULL")
		e.StackStatus = "unresolved_callchain"
		var attached []tracewire.ResourceFrame
		if chain, ok := e.CallchainID.Integer(); ok && chain >= 0 && chain < math.MaxUint32 {
			e.StackStatus = "frame_table_unavailable"
			if frameAvailable {
				attached = frames[chain]
				e.StackStatus = "no_frames"
				if len(attached) > 0 {
					e.StackStatus = "observed"
				}
			}
		}
		e.FrameCount = len(attached)
		r := tracewire.ResourceStackRecord{TimestampNS: ts, EventRowID: row, Event: &e}
		line, ok := tracewire.FormatResourceStack(r)
		if !ok {
			skipped["unrepresentable_resource_event"]++
			continue
		}
		if err := addTraceDBTypedCommentRow(sink, ts, line); err != nil {
			return c, err
		}
		c.RowsEmitted++
		for i := range attached {
			r.Event, r.Frame = nil, &attached[i]
			line, ok = tracewire.FormatResourceStack(r)
			if !ok {
				return c, fmt.Errorf("invalid resource frame carrier for row %d", row)
			}
			if err := addTraceDBTypedCommentRow(sink, ts, line); err != nil {
				return c, err
			}
			c.RowsEmitted++
		}
	}
	c.Skipped = traceDBCountSummary(skipped)
	return c, rows.Err()
}

func resourceOptionalSelects(columns, names []string) []string {
	var out []string
	for _, name := range names {
		value := "NULL"
		if traceDBStringSliceContains(columns, name) {
			value = quoteSQLiteIdent(name)
		}
		out = append(out, value)
	}
	return out
}

func resourceSQLScalar(raw any, present bool) tracewire.ResourceScalar {
	if !present {
		return tracewire.ResourceScalar{Status: "unavailable"}
	}
	if raw == nil {
		return tracewire.ResourceScalar{Status: "null"}
	}
	if n, ok := traceDBStrictSQLiteInt(raw); ok {
		return tracewire.ResourceScalar{Status: "known", Value: strconv.FormatInt(n, 10)}
	}
	return tracewire.ResourceScalar{Status: "invalid"}
}
