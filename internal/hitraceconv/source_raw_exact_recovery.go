package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

var traceDBRawExactRecoveryFamilies = []string{
	traceDBRawRetentionWorkqueue,
	traceDBRawRetentionFilemap,
	traceDBRawRetentionF2FS,
}

func traceDBRawExactRecoveryExpectedPairFamily(family string) tracequery.PairingEndpointFamily {
	switch family {
	case traceDBRawRetentionWorkqueue:
		return tracequery.PairingEndpointWorkqueue
	case traceDBRawRetentionF2FS:
		return tracequery.PairingEndpointStorage
	default:
		return ""
	}
}

func traceDBRawExactRecoveryVerdictMatches(
	family, name, body string,
	headerPID int64,
	verdict tracequery.PairingEndpointVerdict,
) bool {
	expected := traceDBRawExactRecoveryExpectedPairFamily(family)
	return expected != "" && verdict.Recognized && verdict.Family == expected &&
		verdict.KeyKnown && verdict.PayloadAdmitted &&
		verdict.EmitterKnown && verdict.EmitterAdmitted &&
		traceDBRawPairingWireParity(name, name+": "+body, headerPID, verdict)
}

func traceDBRawExactFamilyAuthorityEligible(
	inventory *traceDBSourceNameInventory,
	family string,
) bool {
	if inventory == nil || !traceDBRawExactRecoveryFamilyKnown(family) ||
		!traceDBRawDecodeFamilyComplete(inventory.RawDecode, family) ||
		inventory.RawDecode.Metrics["target_"+family+"_record_capture_failed"] != 0 {
		return false
	}
	physical, admitted, retained := int64(0), int64(0), int64(0)
	for _, name := range traceDBRawExactRecoveryTargetNames() {
		if traceDBRawExactRecoveryFamily(name) != family {
			continue
		}
		metric := traceDBRawDecodeMetricName(name)
		physical += inventory.RawDecode.Metrics["target_"+metric+"_records"]
		admitted += inventory.RawDecode.Metrics["target_"+metric+"_body_admitted"]
	}
	for _, record := range inventory.RawExact {
		if record.Family == family {
			retained++
		}
	}
	return physical > 0 && physical == admitted && admitted == retained &&
		inventory.RawDecode.Metrics["target_"+family+"_records_retained"] == retained
}

func traceDBRawExactRecoveryFamilyKnown(family string) bool {
	for _, candidate := range traceDBRawExactRecoveryFamilies {
		if candidate == family {
			return true
		}
	}
	return false
}

func newTraceDBRawExactRecoveryCoverage(family string) TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_" + family,
		Table:  "__raw_" + family + "__",
		Role:   "query_ready_export",
		FieldSources: map[string]string{
			"authority":     "complete strict official raw decode ledger and the existing family-specific descriptor decoder; exact event-name registry only",
			"timestamp":     "exact source raw record timestamp on the standard ftrace wire, preserving nanosecond digits",
			"cpu":           "exact source raw page CPU",
			"envelope":      "exact source common_pid/common_flags/common_preempt_count",
			"namespace":     "physical common_pid is copied verbatim; no namespace PID, host PID, TGID, comm, or incarnation rewrite is attempted",
			"payload":       "canonical body from the sole existing strict workqueue/filemap/F2FS descriptor decoder; legacy/generic fallback is ineligible",
			"pairing":       "workqueue and F2FS endpoints must agree with tracequery's source-neutral typed endpoint authority; filemap rows are point observations and never endpoints",
			"deduplication": "a complete source family supersedes normalized SQLite raw rows of its exact governed event names before DB publication; partial families never override DB rows; rows of other names sharing a SQL raw class are never part of this overlap",
		},
		Metadata: map[string]string{"publication_state": "unavailable"},
	}
}

func publishTraceDBRawExactRecoveries(
	ctx context.Context,
	inventory *traceDBSourceNameInventory,
	sink *traceDBRowSink,
	dbRawCoverage []TraceDBCoverage,
) ([]TraceDBCoverage, error) {
	items := make([]TraceDBCoverage, 0, len(traceDBRawExactRecoveryFamilies))
	for _, family := range traceDBRawExactRecoveryFamilies {
		item, err := publishTraceDBRawExactRecoveryFamily(
			ctx, inventory, sink, dbRawCoverage, family)
		items = append(items, item)
		if err != nil {
			return items, err
		}
	}
	return items, nil
}

func publishTraceDBRawExactRecoveryFamily(
	ctx context.Context,
	inventory *traceDBSourceNameInventory,
	sink *traceDBRowSink,
	dbRawCoverage []TraceDBCoverage,
	family string,
) (TraceDBCoverage, error) {
	out := newTraceDBRawExactRecoveryCoverage(family)
	if inventory == nil {
		out.Skipped = "exact source recovery unavailable: immutable source inventory absent"
		return out, nil
	}
	out.Found = inventory.RawDecode.Found
	for _, record := range inventory.RawExact {
		if record.Family == family {
			out.RowsRead++
		}
	}
	if stop, err := traceDBApplySourceRawLaneGate(&out, inventory, "exact source recovery"); stop {
		return out, err
	}
	if !traceDBRawExactFamilyAuthorityEligible(inventory, family) {
		if traceDBRawDecodeFamilyComplete(inventory.RawDecode, family) && out.RowsRead == 0 {
			out.Metadata["publication_state"] = "complete_no_family_event"
			out.Skipped = "exact source recovery complete: no governed family event present"
		} else {
			out.Metadata["publication_state"] = "withheld_family_census_incomplete"
			out.Skipped = "exact source recovery withheld: physical/admitted/retained family census is not exact"
		}
		return out, nil
	}
	// Overlap is measured on the exact governed event names of this family,
	// never on a SQL raw class mapped from the family.
	supersession, registered := traceDBRawSourceSupersessionForFamily(family)
	if !registered {
		err := &traceDBOutputInvariantError{Reason: "source_supersession_family_unregistered"}
		out.Error = err.Error()
		return out, err
	}
	if publishable := traceDBRawSourceGovernedRowsPublishable(dbRawCoverage, supersession); publishable > 0 {
		traceDBAddCoverageMetric(&out, "db_raw_governed_family_rows_publishable", publishable)
		out.Metadata["publication_state"] = "withheld_db_raw_family_overlap"
		out.Skipped = "exact source recovery withheld: normalized DB raw-ftrace rows of governed family event names kept DB publication authority"
		return out, nil
	}
	if sink == nil {
		err := &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
		out.Error = err.Error()
		return out, err
	}
	if sink.stats.RowsAccepted < 0 || out.RowsRead > math.MaxInt-sink.stats.RowsAccepted {
		err := &traceDBOutputInvariantError{Reason: "invalid_raw_exact_recovery_sequence"}
		out.Error = err.Error()
		return out, err
	}

	type orderedExact struct {
		record traceDBRawExactRecord
		index  int
	}
	ordered := make([]orderedExact, 0, out.RowsRead)
	ordinals := make(map[int64]bool, out.RowsRead)
	for index, record := range inventory.RawExact {
		if record.Family != family {
			continue
		}
		if record.PhysicalOrdinal <= 0 || ordinals[record.PhysicalOrdinal] ||
			record.TimestampNS > math.MaxInt64 ||
			!validTraceDBCPUIndex(int64(record.CPU)) ||
			record.HeaderPID < 0 || record.HeaderPID > math.MaxInt32 ||
			record.Flags < 0 || record.Flags > math.MaxUint8 ||
			record.PreemptCount < 0 || record.PreemptCount > math.MaxUint8 ||
			traceDBRawExactRecoveryFamily(record.Name) != family ||
			record.Body == "" || !traceDBSinglePhysicalLine(record.Body, false) {
			out.Metadata["publication_state"] = "withheld_invalid_retained_envelope"
			out.Skipped = "exact source recovery withheld: retained event envelope or body is invalid"
			return out, nil
		}
		if traceDBRawExactRecoveryPairEndpoint(record.Name) {
			verdict := tracequery.DecodePairingEndpoint(
				record.Name, record.Body, record.HeaderPID)
			if !traceDBRawExactRecoveryVerdictMatches(
				family, record.Name, record.Body, record.HeaderPID, verdict) {
				out.Metadata["publication_state"] = "withheld_pairing_wire_mismatch"
				out.Skipped = "exact source recovery withheld: retained endpoint disagrees with source-neutral pairing authority"
				return out, nil
			}
			traceDBAddCoverageMetric(&out, "pairing_endpoint_rows_validated", 1)
		}
		ordinals[record.PhysicalOrdinal] = true
		ordered = append(ordered, orderedExact{record: record, index: index})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].record.TimestampNS != ordered[j].record.TimestampNS {
			return ordered[i].record.TimestampNS < ordered[j].record.TimestampNS
		}
		if ordered[i].record.PhysicalOrdinal != ordered[j].record.PhysicalOrdinal {
			return ordered[i].record.PhysicalOrdinal < ordered[j].record.PhysicalOrdinal
		}
		return ordered[i].index < ordered[j].index
	})
	rendered := make([]renderedRow, 0, len(ordered))
	for index, item := range ordered {
		record := item.record
		task := "unknown"
		if name := inventory.Names[record.HeaderPID]; name != "" {
			task = name
		}
		row, err := prepareTraceDBRenderedRowEnvelopeContext(
			ctx, int64(record.TimestampNS), sink.stats.RowsAccepted+index,
			task, record.HeaderPID, 0, int64(record.CPU), record.Flags,
			record.PreemptCount, record.HeaderPID != 0,
			record.Name+": "+record.Body)
		if err != nil {
			out.Error = err.Error()
			return out, err
		}
		rendered = append(rendered, row)
	}
	for _, row := range rendered {
		if err := sink.addContext(ctx, row, nil, false); err != nil {
			out.Error = err.Error()
			return out, err
		}
		out.RowsEmitted++
	}
	out.Metadata["publication_state"] = "published_complete_exact_source_family"
	out.Metadata["publication_contract"] = fmt.Sprintf(
		"family=%s; events=%d; exact raw timestamp/cpu/flags/common_pid and canonical strict payload",
		family, out.RowsEmitted)
	return out, nil
}
