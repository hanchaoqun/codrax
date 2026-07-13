package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
)

type profilerCancelAfterPollContext struct {
	context.Context
	polls    int
	cancelAt int
}

func (ctx *profilerCancelAfterPollContext) Err() error {
	ctx.polls++
	if ctx.polls >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestProfilerFtraceCPUDetailVisitorUsesFinalCPUAndSyntheticOrder(t *testing.T) {
	eventA := syntheticTracePluginFtraceEvent(10, 7, 7, "worker", 1109, protoBytes(2, []byte("A")))
	eventB := syntheticTracePluginFtraceEvent(20, 7, 7, "worker", 1109, protoBytes(2, []byte("B")))
	detail := protoPayload(
		eventA,
		protoVarint(2, 1),
		eventB,
		protoVarint(3, 1),
		protoVarint(3, 2),
		protoVarint(1, 3),
	)
	authority, err := auditProfilerFtraceCPUDetail(context.Background(), detail)
	if err != nil {
		t.Fatal(err)
	}
	var records []profilerFtraceEventRecord
	if err := visitProfilerFtraceCPUDetailEvents(context.Background(), authority, func(record profilerFtraceEventRecord) error {
		records = append(records, record)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 || records[0].TSNS != 10 || records[1].TSNS != 20 ||
		records[0].CPU != 3 || records[1].CPU != 3 ||
		records[2].EnvelopeIssueCount != 1 || records[2].EnvelopeIssues[0].Kind != profilerFtraceEventIssueEnvelopeEventContainerWrongWire ||
		records[3].EnvelopeIssueCount != 1 || records[3].EnvelopeIssues[0].Kind != profilerFtraceEventIssueEnvelopeOverwriteInvalid {
		t.Fatalf("CPUDetail physical/synthetic order drifted: %+v", records)
	}
}

func TestProfilerFtraceCPUDetailLateCPUDamageAppliesToEveryEvent(t *testing.T) {
	detail := protoPayload(
		syntheticTracePluginFtraceEvent(10, 7, 7, "worker", 1109, nil),
		syntheticTracePluginFtraceEvent(20, 7, 7, "worker", 1109, nil),
		protoVarint(1, 1),
		protoVarint(1, 2),
	)
	authority, err := auditProfilerFtraceCPUDetail(context.Background(), detail)
	if err != nil {
		t.Fatal(err)
	}
	visited := 0
	if err := visitProfilerFtraceCPUDetailEvents(context.Background(), authority, func(record profilerFtraceEventRecord) error {
		visited++
		if record.EnvelopeIssueCount == 0 || record.EnvelopeIssues[0].Kind != profilerFtraceEventIssueEnvelopeCPUDuplicate {
			t.Fatalf("late CPU duplicate did not taint event %d: %+v", visited, record)
		}
		return nil
	}); err != nil || visited != 2 {
		t.Fatalf("late CPU duplicate replay visited=%d err=%v", visited, err)
	}
}

func TestProfilerFtraceCPUDetailSummaryDropsBadCPUDetailButKeepsTopSiblings(t *testing.T) {
	event := func(ts uint64) []byte {
		return syntheticTracePluginFtraceEvent(ts, 7, 7, "worker", 1109, nil)
	}
	result := decodeProfilerTracePluginResult(protoPayload(
		protoBytes(2, protoPayload(protoVarint(1, 0), event(10))),
		protoBytes(2, protoPayload(event(20), protoVarint(1, 1), protoVarint(1, 2))),
		protoBytes(2, protoPayload(protoVarint(1, 3), event(30))),
	))
	summary, recognized, err := decodeProfilerFtraceSummaryResult(result)
	if err != nil || !recognized {
		t.Fatalf("summary recognized=%t err=%v", recognized, err)
	}
	if summary.DetailMessages != 2 || summary.DetailEventCount != 2 ||
		len(summary.DetailCPUs) != 2 || !summary.DetailCPUs[0] || !summary.DetailCPUs[3] || summary.DetailCPUs[2] ||
		summary.EventFieldCounts[1109] != 2 {
		t.Fatalf("bad CPU detail polluted summary or starved siblings: %+v", summary)
	}
}

func TestProfilerFtraceCPUDetailMalformedTailPublishesNoHealthyPrefix(t *testing.T) {
	event := syntheticTracePluginFtraceEvent(10, 7, 7, "worker", 4012, nil)
	detail := append(append([]byte(nil), event...), 0x80)
	authority, err := auditProfilerFtraceCPUDetail(context.Background(), detail)
	if err != nil {
		t.Fatal(err)
	}
	if !authority.Malformed || authority.PairFamilies&pairCriticalFormatFamilyF2FS == 0 {
		t.Fatalf("malformed detail lost typed pair provenance: %+v", authority)
	}
	var records []profilerFtraceEventRecord
	if err := visitProfilerFtraceCPUDetailEvents(context.Background(), authority, func(record profilerFtraceEventRecord) error {
		records = append(records, record)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].TSNS != 0 || !records[0].PairCaptureOpaque ||
		records[0].PairFamilies&pairCriticalFormatFamilyF2FS == 0 ||
		records[0].EnvelopeIssueCount != 1 || records[0].EnvelopeIssues[0].Kind != profilerFtraceEventIssueEnvelopeCPUDetailMalformedWire {
		t.Fatalf("malformed detail leaked a healthy prefix: %+v", records)
	}
}

func TestProfilerFtraceCPUDetailMalformedTailPublishesZeroRealRows(t *testing.T) {
	event := syntheticTracePluginFtraceEvent(10, 7, 7, "worker", 4012, nil)
	detail := append(append([]byte(nil), event...), 0x80)
	result := decodeProfilerTracePluginResult(protoBytes(2, detail))
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredResult(result, &seq, sink)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 || seq != 0 || sink.stats.RowsAccepted != 0 ||
		!coverageTableHasSkipped(coverage, "__cpu_detail_envelope__", "envelope_cpu_detail_malformed_wire") ||
		!sink.pairKindPoisoned(pairRenderF2FS) {
		t.Fatalf("malformed detail prefix escaped real renderer: rows=%d seq=%d sink=%+v coverage=%+v", rows, seq, sink.stats, coverage)
	}
}

func TestProfilerFtraceCPUDetailMillionEventsRetainFixedShape(t *testing.T) {
	const occurrences = 1_000_000
	detail := bytes.Repeat(protoBytes(2, nil), occurrences)
	authority, err := auditProfilerFtraceCPUDetail(context.Background(), detail)
	if err != nil {
		t.Fatal(err)
	}
	if authority.EventOccurrences != occurrences || authority.EventPayloadOccurrences != occurrences || len(authority.payload) != len(detail) {
		t.Fatalf("CPUDetail census drifted: %+v", authority)
	}
	visited := 0
	if err := visitProfilerFtraceCPUDetailEvents(context.Background(), authority, func(profilerFtraceEventRecord) error {
		visited++
		return nil
	}); err != nil || visited != occurrences {
		t.Fatalf("CPUDetail replay visited=%d err=%v", visited, err)
	}

	typ := reflect.TypeOf(authority)
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.Type.Kind() == reflect.Map ||
			field.Type.Kind() == reflect.Slice && (field.Name != "payload" || field.Type.Elem().Kind() != reflect.Uint8) {
			t.Fatalf("CPUDetail authority retains repeated state: %s %s", field.Name, field.Type)
		}
	}
}

func TestProfilerFtraceCPUDetailVisitorCancellationAndCallbackIdentity(t *testing.T) {
	authority, err := auditProfilerFtraceCPUDetail(context.Background(), bytes.Repeat(protoBytes(2, nil), 2_000))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	visited := 0
	err = visitProfilerFtraceCPUDetailEvents(ctx, authority, func(profilerFtraceEventRecord) error {
		visited++
		if visited == 300 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || visited < 300 || visited > 300+255 {
		t.Fatalf("CPUDetail cancellation drifted: visited=%d err=%v", visited, err)
	}

	sentinel := &traceDBOutputInvariantError{Reason: "test_cpu_detail_callback"}
	if err := visitProfilerFtraceCPUDetailEvents(context.Background(), authority, func(profilerFtraceEventRecord) error {
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("CPUDetail callback error identity lost: %v", err)
	}
}

func TestProfilerFtraceCPUDetailCancelsInsideOneLargeEventEnvelope(t *testing.T) {
	rawEvent := bytes.Repeat(protoVarint(1, 0), 2_000)
	authority, err := auditProfilerFtraceCPUDetail(context.Background(), protoBytes(2, rawEvent))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &profilerCancelAfterPollContext{Context: context.Background(), cancelAt: 6}
	visited := 0
	err = visitProfilerFtraceCPUDetailEvents(ctx, authority, func(profilerFtraceEventRecord) error {
		visited++
		return nil
	})
	if !errors.Is(err, context.Canceled) || visited != 0 || ctx.polls < ctx.cancelAt {
		t.Fatalf("large event envelope cancellation drifted: polls=%d visited=%d err=%v", ctx.polls, visited, err)
	}
}
