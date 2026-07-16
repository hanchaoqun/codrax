package hitraceconv

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"unsafe"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestProfilerPairEndpointRosterClosedBijection(t *testing.T) {
	if len(profilerPairEndpointRoster) != int(profilerPairEndpointSlotCount)-1 {
		t.Fatalf("endpoint roster=%d slots=%d", len(profilerPairEndpointRoster), profilerPairEndpointSlotCount)
	}
	seenNames := map[string]bool{}
	seenFields := map[int]bool{}
	for want := profilerPairEndpointSlot(1); want < profilerPairEndpointSlotCount; want++ {
		descriptor, ok := want.descriptor()
		if !ok || descriptor.slot != want || descriptor.name == "" ||
			descriptor.kind < pairRenderMMC || descriptor.kind > pairRenderBlock {
			t.Fatalf("invalid endpoint slot %d: %+v ok=%t", want, descriptor, ok)
		}
		if seenNames[descriptor.name] {
			t.Fatalf("duplicate endpoint name %q", descriptor.name)
		}
		seenNames[descriptor.name] = true
		if got, found := profilerPairEndpointForName(descriptor.name); !found || got != want {
			t.Fatalf("name round trip %q = %d,%t want %d", descriptor.name, got, found, want)
		}
		if got, governed := profilerPairKindForExactName(descriptor.name); !governed || got != descriptor.kind {
			t.Fatalf("endpoint authority kind %q = %d,%t want %d", descriptor.name, got, governed, descriptor.kind)
		}
		if descriptor.structuredField == 0 {
			if want != profilerPairEndpointF2FSDirectIOEnter && want != profilerPairEndpointF2FSDirectIOExit {
				t.Fatalf("unexpected text-only endpoint: %+v", descriptor)
			}
			continue
		}
		if seenFields[descriptor.structuredField] {
			t.Fatalf("duplicate structured field %d", descriptor.structuredField)
		}
		seenFields[descriptor.structuredField] = true
		if got, found := profilerPairEndpointForStructuredField(descriptor.structuredField); !found || got != want {
			t.Fatalf("field round trip %d = %d,%t want %d", descriptor.structuredField, got, found, want)
		}
		if !profilerStructuredPairEventField(descriptor.kind, descriptor.structuredField) {
			t.Fatalf("structured authority rejected endpoint %+v", descriptor)
		}
	}
	const textOnlyEndpointCount = 2
	wantStructuredFields := len(profilerPairEndpointRoster) - textOnlyEndpointCount
	if len(seenFields) != wantStructuredFields {
		t.Fatalf("structured endpoint fields=%d want=%d", len(seenFields), wantStructuredFields)
	}
	for _, invalid := range []string{
		"", "f2fs_write", "F2FS_write_begin", "block_rq_issue_extra", "attacker-1",
		"workqueue_execute_start", "workqueue_execute_end", "dma_fence_wait_start", "dma_fence_wait_end",
	} {
		if slot, ok := profilerPairEndpointForName(invalid); ok || slot != profilerPairEndpointNone {
			t.Fatalf("near/unknown endpoint %q admitted as %d", invalid, slot)
		}
		if kind, governed := profilerPairKindForExactName(invalid); governed || kind != pairRenderUnknown {
			t.Fatalf("non-Profiler endpoint %q entered Profiler authority as %d", invalid, kind)
		}
	}
	for _, invalid := range []int{0, -1, 205, 210, 212, 4013, 4014, 4017} {
		if slot, ok := profilerPairEndpointForStructuredField(invalid); ok || slot != profilerPairEndpointNone {
			t.Fatalf("unknown/inventory field %d admitted as %d", invalid, slot)
		}
	}
}

func TestProfilerPairPublisherSlotsMapClosedRoutes(t *testing.T) {
	want := []profilerPairPublisherSlot{
		profilerPairPublisherExactFtrace,
		profilerPairPublisherBytrace,
		profilerPairPublisherNoncanonicalFtrace,
		profilerPairPublisherOtherText,
	}
	for route := profilerPluginRoute(0); route < profilerPluginRouteCount; route++ {
		got, ok := profilerPairPublisherForRoute(route)
		if !ok || got != want[int(route)] {
			t.Fatalf("route %d mapped to %d,%t want %d", route, got, ok, want[int(route)])
		}
	}
	if got, ok := profilerPairPublisherForRoute(profilerPluginRouteCount); ok || got != profilerPairPublisherNone {
		t.Fatalf("invalid route admitted as %d,%t", got, ok)
	}
	for slot := profilerPairPublisherSlot(0); slot < profilerPairPublisherSlotCount; slot++ {
		wantText := slot == profilerPairPublisherExactFtrace || slot == profilerPairPublisherBytrace ||
			slot == profilerPairPublisherOtherText
		if slot.textCapable() != wantText {
			t.Fatalf("publisher %d text-capable=%t want=%t", slot, slot.textCapable(), wantText)
		}
	}
}

func TestProfilerPairProvenanceNumericABI(t *testing.T) {
	gotKinds := []pairRenderKind{
		pairRenderUnknown, pairRenderWorkqueue, pairRenderDMAFence,
		pairRenderMMC, pairRenderF2FS, pairRenderBlock, pairRenderKindCount,
	}
	for index, got := range gotKinds {
		if got != pairRenderKind(index) {
			t.Fatalf("pair kind ABI[%d]=%d want=%d", index, got, index)
		}
	}
	wantEndpoints := []profilerPairEndpointDescriptor{
		{1, pairRenderMMC, "mmc_request_start", 4016},
		{2, pairRenderMMC, "mmc_request_done", 4015},
		{3, pairRenderF2FS, "f2fs_sync_file_enter", 4009},
		{4, pairRenderF2FS, "f2fs_sync_file_exit", 4010},
		{5, pairRenderF2FS, "f2fs_direct_IO_enter", 0},
		{6, pairRenderF2FS, "f2fs_direct_IO_exit", 0},
		{7, pairRenderF2FS, "f2fs_write_begin", 4011},
		{8, pairRenderF2FS, "f2fs_write_end", 4012},
		{9, pairRenderBlock, "block_bio_queue", 204},
		{10, pairRenderBlock, "block_bio_complete", 202},
		{11, pairRenderBlock, "block_rq_issue", 211},
		{12, pairRenderBlock, "block_rq_complete", 209},
	}
	if len(profilerPairEndpointRoster) != len(wantEndpoints) {
		t.Fatalf("endpoint ABI count=%d want=%d", len(profilerPairEndpointRoster), len(wantEndpoints))
	}
	for index, want := range wantEndpoints {
		if got := profilerPairEndpointRoster[index]; got != want {
			t.Fatalf("endpoint ABI[%d]=%+v want=%+v", index, got, want)
		}
	}
	wantPublishers := []profilerPairPublisherSlot{0, 1, 2, 3, 4, 5}
	gotPublishers := []profilerPairPublisherSlot{
		profilerPairPublisherNone, profilerPairPublisherExactFtrace, profilerPairPublisherBytrace,
		profilerPairPublisherNoncanonicalFtrace, profilerPairPublisherOtherText, profilerPairPublisherSession,
	}
	for index := range wantPublishers {
		if gotPublishers[index] != wantPublishers[index] {
			t.Fatalf("publisher ABI[%d]=%d want=%d", index, gotPublishers[index], wantPublishers[index])
		}
	}
	if profilerPairPublisherSlotCount != 6 || profilerPairRowProvenanceText != 1 ||
		profilerPairRowProvenanceStructured != 2 || profilerPairRowProvenanceFlagMask != 3 ||
		profilerTraceClassNone != 0 || profilerTraceClassStructuredKnown != 1 ||
		profilerTraceClassTextKnown != 2 || profilerTraceClassTextIntentionalUnknown != 3 ||
		profilerTraceClassCount != 4 {
		t.Fatalf("publisher/flag ABI drift: slots=%d text=%d structured=%d mask=%d",
			profilerPairPublisherSlotCount, profilerPairRowProvenanceText,
			profilerPairRowProvenanceStructured, profilerPairRowProvenanceFlagMask)
	}
}

func TestProfilerTraceClassMintingIsSourceClosed(t *testing.T) {
	knownLine := traceDBFormatLine("worker", 7, 7, 1, 1_000_000_000, 0, 0,
		"sched_wakeup: comm=app pid=42 prio=120 target_cpu=1")
	unknownLine := traceDBFormatLine("worker", 7, 7, 1, 1_000_000_000, 0, 0,
		"vendor_private_event: opaque=1")
	if event, parsed, err := parseOwnedSystraceRow(1, unknownLine); err != nil || !parsed ||
		event.Type != tracequery.EventUnknown {
		t.Fatalf("unknown fixture drifted: parsed=%t event=%+v err=%v", parsed, event, err)
	}
	tests := []struct {
		name      string
		publisher profilerPairPublisherSlot
		text      bool
		line      string
		wantClass profilerTraceClass
		wantError string
	}{
		{name: "structured known", publisher: profilerPairPublisherExactFtrace,
			line: knownLine, wantClass: profilerTraceClassStructuredKnown},
		{name: "strict text known", publisher: profilerPairPublisherExactFtrace, text: true,
			line: knownLine, wantClass: profilerTraceClassTextKnown},
		{name: "strict text intentional unknown", publisher: profilerPairPublisherExactFtrace, text: true,
			line: unknownLine, wantClass: profilerTraceClassTextIntentionalUnknown},
		{name: "bytrace known", publisher: profilerPairPublisherBytrace, text: true,
			line: knownLine, wantClass: profilerTraceClassTextKnown},
		{name: "bytrace intentional unknown", publisher: profilerPairPublisherBytrace, text: true,
			line: unknownLine, wantClass: profilerTraceClassTextIntentionalUnknown},
		{name: "other text known", publisher: profilerPairPublisherOtherText, text: true,
			line: knownLine, wantClass: profilerTraceClassTextKnown},
		{name: "other text intentional unknown", publisher: profilerPairPublisherOtherText, text: true,
			line: unknownLine, wantClass: profilerTraceClassTextIntentionalUnknown},
		{name: "session known", publisher: profilerPairPublisherSession,
			line: knownLine, wantClass: profilerTraceClassTextKnown},
		{name: "session intentional unknown", publisher: profilerPairPublisherSession,
			line: unknownLine, wantClass: profilerTraceClassTextIntentionalUnknown},
		{name: "structured unknown rejected", publisher: profilerPairPublisherExactFtrace,
			line: unknownLine, wantError: "profiler_trace_class_structured_unknown"},
		{name: "unparsed text rejected", publisher: profilerPairPublisherOtherText, text: true,
			line: "not-a-trace-row", wantError: "profiler_trace_class_unparsed"},
		{name: "publisher missing", line: knownLine, wantError: "profiler_trace_class_publisher_missing"},
		{name: "noncanonical source rejected", publisher: profilerPairPublisherNoncanonicalFtrace,
			line: knownLine, wantError: "profiler_row_provenance_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if err := sink.enableProfilerTraceClassification(); err != nil {
				t.Fatal(err)
			}
			if !sink.beginPairRowCensusForPublisher(test.publisher) {
				t.Fatal("begin publisher census")
			}
			if test.text && !sink.beginProfilerTextMessage() {
				t.Fatal("begin text message")
			}
			err = sink.add(renderedRow{tsNS: 1_000_000_000, seq: 1, line: test.line})
			if test.wantError != "" {
				if reason := profilerSinkInvariantReason(t, err); reason != test.wantError {
					t.Fatalf("reason=%q want=%q err=%v", reason, test.wantError, err)
				}
				if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || sink.profilerSourceProof.count != 0 {
					t.Fatalf("classification failure mutated sink: stats=%+v rows=%+v proof=%+v",
						sink.stats, sink.rows, sink.profilerSourceProof)
				}
				return
			}
			if err != nil || len(sink.rows) != 1 {
				t.Fatalf("classified add failed: rows=%d err=%v", len(sink.rows), err)
			}
			got := sink.rows[0].profilerProvenance()
			if got.TraceClass != test.wantClass || !got.classifiedValid() {
				t.Fatalf("trace class=%+v want=%d", got, test.wantClass)
			}
		})
	}
}

func TestProfilerTraceClassEnableIsOneShotBeforeFirstRow(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.enableProfilerTraceClassification(); err != nil {
		t.Fatal(err)
	}
	if reason := profilerSinkInvariantReason(t, sink.enableProfilerTraceClassification()); reason != "profiler_trace_class_enable_state_invalid" {
		t.Fatalf("repeat enable reason=%q", reason)
	}

	late, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer late.cleanup()
	if err := late.add(renderedRow{tsNS: 1, seq: 1, line: "source-only-row"}); err != nil {
		t.Fatal(err)
	}
	if reason := profilerSinkInvariantReason(t, late.enableProfilerTraceClassification()); reason != "profiler_trace_class_enable_state_invalid" {
		t.Fatalf("late enable reason=%q", reason)
	}
}

func TestProfilerProductionStorageRejectsEveryUnclassifiedPublisher(t *testing.T) {
	sink := &traceDBRowSink{profilerTraceClassification: true}
	for _, publisher := range []profilerPairPublisherSlot{
		profilerPairPublisherNone,
		profilerPairPublisherExactFtrace,
		profilerPairPublisherBytrace,
		profilerPairPublisherOtherText,
		profilerPairPublisherSession,
	} {
		provenance := profilerPairRowProvenance{PairKind: pairRenderUnknown, PublisherSlot: publisher}
		if publisher == profilerPairPublisherBytrace || publisher == profilerPairPublisherOtherText {
			provenance.TextMessageOrdinal = 1
			provenance.Flags = profilerPairRowProvenanceText
		}
		if sink.profilerStoredProvenanceValid(provenance) {
			t.Fatalf("production admitted unclassified publisher: %+v", provenance)
		}
	}
	classified := profilerPairRowProvenance{
		PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherExactFtrace,
		TraceClass: profilerTraceClassStructuredKnown,
	}
	if !sink.profilerStoredProvenanceValid(classified) {
		t.Fatalf("production rejected classified provenance: %+v", classified)
	}
}

func TestProfilerTraceClassFailureCannotSpillAcceptedPrefix(t *testing.T) {
	source := profilerSourceLifecycleFile(t)
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatal(err)
	}
	if err := sink.enableProfilerTraceClassification(); err != nil {
		t.Fatal(err)
	}
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherExactFtrace) {
		t.Fatal("begin structured publisher")
	}
	known := traceDBFormatLine("worker", 7, 7, 1, 1_000_000_000, 0, 0,
		"sched_wakeup: comm=app pid=42 prio=120 target_cpu=1")
	if err := sink.addProfilerEventContext(context.Background(), renderedRow{
		tsNS: 1_000_000_000, seq: 1, line: known,
	}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	wantRows := append([]traceDBStoredRow(nil), sink.rows...)
	wantOrdinals := append([]uint64(nil), sink.rowIngestOrdinals...)
	wantStats := sink.stats
	wantBuffered := sink.bufferedBytes
	wantProofCount := sink.profilerSourceProof.count
	unknown := traceDBFormatLine("worker", 7, 7, 1, 1_001_000_000, 0, 0,
		"vendor_private_event: opaque=1")
	err = sink.addProfilerEventContext(context.Background(), renderedRow{
		tsNS: 1_001_000_000, seq: 2, line: unknown,
	}, traceDBProfilerEventDelta{})
	if reason := profilerSinkInvariantReason(t, err); reason != "profiler_trace_class_structured_unknown" {
		t.Fatalf("classification reason=%q err=%v", reason, err)
	}
	if !reflect.DeepEqual(sink.rows, wantRows) || !reflect.DeepEqual(sink.rowIngestOrdinals, wantOrdinals) ||
		sink.stats != wantStats || sink.bufferedBytes != wantBuffered || len(sink.runs) != 0 ||
		sink.nextIngestOrdinal != 1 || sink.profilerSourceProof.count != wantProofCount ||
		sink.profilerSourceProof.prepared {
		t.Fatalf("classification failure mutated accepted prefix: rows=%+v/%+v ordinals=%v/%v stats=%+v/%+v buffered=%d/%d runs=%+v next=%d proof=%+v",
			sink.rows, wantRows, sink.rowIngestOrdinals, wantOrdinals, sink.stats, wantStats,
			sink.bufferedBytes, wantBuffered, sink.runs, sink.nextIngestOrdinal, sink.profilerSourceProof)
	}
	sink.abortPairRowCensus()
}

func TestProfilerPairRowProvenanceFixedShapeAndValidity(t *testing.T) {
	if got := unsafe.Sizeof(profilerPairRowProvenance{}); got != 16 {
		t.Fatalf("provenance size=%d want=16", got)
	}
	valid := []profilerPairRowProvenance{
		{},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherExactFtrace},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherSession},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherBytrace,
			TextMessageOrdinal: 1, Flags: profilerPairRowProvenanceText},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherOtherText,
			TextMessageOrdinal: 1, Flags: profilerPairRowProvenanceText},
		{PairKind: pairRenderF2FS, LaneID: 1, EndpointSlot: profilerPairEndpointF2FSWriteBegin,
			PublisherSlot: profilerPairPublisherOtherText, TextMessageOrdinal: 2, Flags: profilerPairRowProvenanceText},
		{PairKind: pairRenderBlock, LaneID: 2, EndpointSlot: profilerPairEndpointBlockRQIssue,
			PublisherSlot: profilerPairPublisherExactFtrace, Flags: profilerPairRowProvenanceStructured},
		{PairKind: pairRenderMMC, EndpointSlot: profilerPairEndpointMMCRequestStart,
			PublisherSlot: profilerPairPublisherSession},
		{PairKind: pairRenderWorkqueue},
	}
	for index, provenance := range valid {
		if !provenance.storageValid() {
			t.Fatalf("valid provenance[%d] rejected: %+v", index, provenance)
		}
	}
	invalid := []profilerPairRowProvenance{
		{PairKind: pairRenderKindCount},
		{PairKind: pairRenderUnknown, LaneID: 1},
		{PairKind: pairRenderUnknown, EndpointSlot: profilerPairEndpointMMCRequestStart},
		{PairKind: pairRenderF2FS, EndpointSlot: profilerPairEndpointMMCRequestStart},
		{PairKind: pairRenderF2FS, EndpointSlot: profilerPairEndpointF2FSDirectIOEnter,
			Flags: profilerPairRowProvenanceStructured},
		{PairKind: pairRenderBlock, EndpointSlot: profilerPairEndpointBlockRQIssue,
			TextMessageOrdinal: 1, Flags: profilerPairRowProvenanceText},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherSession,
			TextMessageOrdinal: 1, Flags: profilerPairRowProvenanceText},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherBytrace},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherOtherText},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherNoncanonicalFtrace},
		{PairKind: pairRenderWorkqueue, EndpointSlot: profilerPairEndpointBlockRQIssue},
		{PairKind: pairRenderWorkqueue, LaneID: 1},
		{PairKind: pairRenderMMC, EndpointSlot: profilerPairEndpointMMCRequestStart,
			Flags: profilerPairRowProvenanceFlagMask},
		{PairKind: pairRenderMMC, EndpointSlot: profilerPairEndpointSlotCount},
		{PairKind: pairRenderMMC, EndpointSlot: profilerPairEndpointMMCRequestStart,
			PublisherSlot: profilerPairPublisherSlotCount},
		{PairKind: pairRenderMMC, EndpointSlot: profilerPairEndpointMMCRequestStart,
			PublisherSlot: profilerPairPublisherSession, Flags: profilerPairRowProvenanceStructured},
		{PairKind: pairRenderMMC, EndpointSlot: profilerPairEndpointMMCRequestStart,
			PublisherSlot: profilerPairPublisherExactFtrace},
		{PairKind: pairRenderMMC, EndpointSlot: profilerPairEndpointMMCRequestStart,
			PublisherSlot: profilerPairPublisherOtherText},
		{PairKind: pairRenderMMC, EndpointSlot: profilerPairEndpointMMCRequestStart,
			PublisherSlot:      profilerPairPublisherNoncanonicalFtrace,
			TextMessageOrdinal: 1, Flags: profilerPairRowProvenanceText},
	}
	for index, provenance := range invalid {
		if provenance.storageValid() {
			t.Fatalf("invalid provenance[%d] admitted: %+v", index, provenance)
		}
	}
	classified := []profilerPairRowProvenance{
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherExactFtrace,
			TraceClass: profilerTraceClassStructuredKnown},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherExactFtrace,
			TextMessageOrdinal: 1, Flags: profilerPairRowProvenanceText,
			TraceClass: profilerTraceClassTextIntentionalUnknown},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherSession,
			TraceClass: profilerTraceClassTextKnown},
	}
	for index, provenance := range classified {
		if !provenance.classifiedValid() || !provenance.valid() {
			t.Fatalf("classified provenance[%d] rejected: %+v", index, provenance)
		}
	}
	misclassified := []profilerPairRowProvenance{
		{PairKind: pairRenderUnknown, TraceClass: profilerTraceClassStructuredKnown},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherExactFtrace,
			TraceClass: profilerTraceClassTextKnown},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherSession,
			TraceClass: profilerTraceClassStructuredKnown},
		{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherOtherText,
			TextMessageOrdinal: 1, Flags: profilerPairRowProvenanceText,
			TraceClass: profilerTraceClassCount},
	}
	for index, provenance := range misclassified {
		if provenance.classifiedValid() || provenance.valid() {
			t.Fatalf("misclassified provenance[%d] admitted: %+v", index, provenance)
		}
	}
}

func TestProfilerPairRowProvenanceUsesCompactStoredRowBudget(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("the zero-growth rendered-row layout contract is pinned on 64-bit targets")
	}
	if got := unsafe.Sizeof(renderedRow{}); got != 88 {
		t.Fatalf("rendered row size=%d want=88", got)
	}
	if got := unsafe.Sizeof(traceDBStoredRow{}); got != 48 {
		t.Fatalf("stored row size=%d want=48", got)
	}
	if got := unsafe.Sizeof(traceDBChunkRow{}); got != 56 {
		t.Fatalf("chunk row size=%d want=56", got)
	}
	if got := unsafe.Sizeof(traceDBChunkWireRow{}); got != 40 {
		t.Fatalf("chunk wire row size=%d want=40", got)
	}
	if traceDBBufferedRowMetadataProofBytes != 160 || traceDBBufferedRowMetadataBytes != 256 {
		t.Fatalf("metadata proof=%d charge=%d want=160/256",
			traceDBBufferedRowMetadataProofBytes, traceDBBufferedRowMetadataBytes)
	}
}

func TestProfilerPairRowProvenanceWireRequiresEveryScalar(t *testing.T) {
	compact, err := json.Marshal(profilerPairRowProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(compact), "[0,0,0,0,0,0,0]"; got != want {
		t.Fatalf("compact provenance wire=%q want=%q", got, want)
	}
	base, err := json.Marshal(traceDBChunkRow{
		TSNS: 1, Seq: 1, IngestOrdinal: 0, Line: "ordinary",
		ProfilerProvenance: profilerPairRowProvenance{PairKind: pairRenderUnknown},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(base) > 96 || !json.Valid(base) {
		t.Fatalf("ordinary compact run record grew beyond 96 bytes: bytes=%d wire=%s", len(base), base)
	}
	fields := []string{"lane_id", "text_message_ordinal", "pair_kind", "endpoint_slot", "publisher_slot", "flags", "trace_class"}
	for fieldIndex, field := range fields {
		for _, replacement := range []json.RawMessage{nil, json.RawMessage("null")} {
			name := "missing/" + field
			if replacement != nil {
				name = "null/" + field
			}
			t.Run(name, func(t *testing.T) {
				var outer map[string]json.RawMessage
				if err := json.Unmarshal(base, &outer); err != nil {
					t.Fatal(err)
				}
				var tuple []json.RawMessage
				if err := json.Unmarshal(outer["p"], &tuple); err != nil {
					t.Fatal(err)
				}
				if replacement == nil {
					tuple = append(tuple[:fieldIndex], tuple[fieldIndex+1:]...)
				} else {
					tuple[fieldIndex] = replacement
				}
				outer["p"], err = json.Marshal(tuple)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(outer)
				if err != nil {
					t.Fatal(err)
				}
				_, decodeErr := decodeTraceDBRunRecord(append(raw, '\n'), 1)
				if !traceDBInvariantChainContains(decodeErr, "profiler_row_provenance_wire_invalid") {
					t.Fatalf("malformed tuple did not retain typed wire reason: err=%v", decodeErr)
				}
			})
		}
	}
	for name, tuple := range map[string]json.RawMessage{
		"old v1 six-slot tuple": json.RawMessage(`[0,0,0,0,0,0]`),
		"trailing eighth slot":  json.RawMessage(`[0,0,0,0,0,0,0,0]`),
	} {
		t.Run(name, func(t *testing.T) {
			var outer map[string]json.RawMessage
			if err := json.Unmarshal(base, &outer); err != nil {
				t.Fatal(err)
			}
			outer["p"] = tuple
			raw, err := json.Marshal(outer)
			if err != nil {
				t.Fatal(err)
			}
			_, decodeErr := decodeTraceDBRunRecord(append(raw, '\n'), 1)
			if !traceDBInvariantChainContains(decodeErr, "profiler_row_provenance_wire_invalid") {
				t.Fatalf("tuple shape did not fail loud: tuple=%s err=%v", tuple, decodeErr)
			}
		})
	}

	for _, objectCase := range []string{"missing", "null"} {
		t.Run("object/"+objectCase, func(t *testing.T) {
			var outer map[string]json.RawMessage
			if err := json.Unmarshal(base, &outer); err != nil {
				t.Fatal(err)
			}
			if objectCase == "missing" {
				delete(outer, "p")
			} else {
				outer["p"] = json.RawMessage("null")
			}
			raw, err := json.Marshal(outer)
			if err != nil {
				t.Fatal(err)
			}
			_, decodeErr := decodeTraceDBRunRecord(append(raw, '\n'), 1)
			if reason := traceDBInvariantReason(decodeErr); reason != "trace_row_sort_record_required_field_missing" {
				t.Fatalf("decode reason=%q err=%v", reason, decodeErr)
			}
		})
	}

	for _, mutation := range []struct {
		name  string
		index int
		value json.RawMessage
	}{
		{name: "negative endpoint", index: 3, value: json.RawMessage("-1")},
		{name: "overflow pair kind", index: 2, value: json.RawMessage("256")},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			var outer map[string]json.RawMessage
			if err := json.Unmarshal(base, &outer); err != nil {
				t.Fatal(err)
			}
			var tuple []json.RawMessage
			if err := json.Unmarshal(outer["p"], &tuple); err != nil {
				t.Fatal(err)
			}
			tuple[mutation.index] = mutation.value
			outer["p"], err = json.Marshal(tuple)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(outer)
			if err != nil {
				t.Fatal(err)
			}
			if _, decodeErr := decodeTraceDBRunRecord(append(raw, '\n'), 1); decodeErr == nil {
				t.Fatalf("invalid nested provenance was admitted: %s", raw)
			}
		})
	}
	t.Run("object form rejected", func(t *testing.T) {
		var outer map[string]json.RawMessage
		if err := json.Unmarshal(base, &outer); err != nil {
			t.Fatal(err)
		}
		outer["p"] = json.RawMessage(`{"lane_id":0}`)
		raw, err := json.Marshal(outer)
		if err != nil {
			t.Fatal(err)
		}
		if _, decodeErr := decodeTraceDBRunRecord(append(raw, '\n'), 1); decodeErr == nil {
			t.Fatalf("object provenance wire was admitted: %s", raw)
		}
	})
	for _, legacyField := range []string{
		"pair_lane", "pair_table", "profiler_event_field", "pair_kind", "structured_pair",
	} {
		t.Run("legacy top-level field/"+legacyField, func(t *testing.T) {
			var outer map[string]json.RawMessage
			if err := json.Unmarshal(base, &outer); err != nil {
				t.Fatal(err)
			}
			outer[legacyField] = json.RawMessage(`0`)
			raw, err := json.Marshal(outer)
			if err != nil {
				t.Fatal(err)
			}
			_, decodeErr := decodeTraceDBRunRecord(append(raw, '\n'), 1)
			if !traceDBInvariantChainContains(decodeErr, "trace_row_sort_run_decode_failed") {
				t.Fatalf("retired run field %q was not rejected fail-loud: err=%v wire=%s",
					legacyField, decodeErr, raw)
			}
		})
	}
}

func traceDBInvariantChainContains(err error, reason string) bool {
	if err == nil {
		return false
	}
	if invariant, ok := err.(*traceDBOutputInvariantError); ok && invariant.Reason == reason {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if traceDBInvariantChainContains(child, reason) {
				return true
			}
		}
		return false
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return traceDBInvariantChainContains(wrapped.Unwrap(), reason)
	}
	return false
}

func TestProfilerPairRowProvenanceWireRoundTripsClosedEndpoints(t *testing.T) {
	for _, descriptor := range profilerPairEndpointRoster {
		modes := []struct {
			name       string
			publisher  profilerPairPublisherSlot
			ordinal    uint32
			flags      profilerPairRowProvenanceFlags
			structured bool
		}{
			{name: "text", publisher: profilerPairPublisherOtherText, ordinal: 1, flags: profilerPairRowProvenanceText},
			{name: "session", publisher: profilerPairPublisherSession},
		}
		if descriptor.structuredField != 0 {
			modes = append(modes, struct {
				name       string
				publisher  profilerPairPublisherSlot
				ordinal    uint32
				flags      profilerPairRowProvenanceFlags
				structured bool
			}{name: "structured", publisher: profilerPairPublisherExactFtrace,
				flags: profilerPairRowProvenanceStructured, structured: true})
		}
		for _, mode := range modes {
			t.Run(descriptor.name+"/"+mode.name, func(t *testing.T) {
				traceClass := profilerTraceClassTextKnown
				if mode.structured {
					traceClass = profilerTraceClassStructuredKnown
				}
				provenance := profilerPairRowProvenance{
					LaneID: 1, TextMessageOrdinal: mode.ordinal, PairKind: descriptor.kind,
					EndpointSlot: descriptor.slot, PublisherSlot: mode.publisher, Flags: mode.flags,
					TraceClass: traceClass,
				}
				raw, err := json.Marshal(traceDBChunkRow{
					TSNS: 1, Seq: 1, IngestOrdinal: 0, Line: "row", ProfilerProvenance: provenance,
				})
				if err != nil {
					t.Fatal(err)
				}
				record, err := decodeTraceDBRunRecord(append(raw, '\n'), 1)
				if err != nil || record.row.profilerProvenance() != provenance {
					t.Fatalf("roundtrip provenance=%+v want=%+v err=%v", record.row.profilerProvenance(), provenance, err)
				}
			})
		}
	}
}

func TestProfilerPairRowProvenanceIsSinkOwnedBeforeMutation(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *traceDBRowSink)
		row     renderedRow
	}{
		{
			name: "publisher outside census",
			row: renderedRow{tsNS: 1, seq: 1, line: "row",
				profilerPublisherSlot: profilerPairPublisherOtherText},
		},
		{
			name: "matching publisher inside census",
			prepare: func(t *testing.T, sink *traceDBRowSink) {
				if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) {
					t.Fatal("begin census")
				}
			},
			row: renderedRow{tsNS: 1, seq: 1, line: "row",
				profilerPublisherSlot: profilerPairPublisherOtherText},
		},
		{
			name: "ordinal inside text message",
			prepare: func(t *testing.T, sink *traceDBRowSink) {
				if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) || !sink.beginProfilerTextMessage() {
					t.Fatal("begin text message")
				}
			},
			row: renderedRow{tsNS: 1, seq: 1, line: "row", profilerTextMessageOrdinal: 1},
		},
		{
			name: "flags inside text message",
			prepare: func(t *testing.T, sink *traceDBRowSink) {
				if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) || !sink.beginProfilerTextMessage() {
					t.Fatal("begin text message")
				}
			},
			row: renderedRow{tsNS: 1, seq: 1, line: "row", profilerProvenanceFlags: profilerPairRowProvenanceText},
		},
		{
			name: "trace class before deterministic mint",
			row: renderedRow{tsNS: 1, seq: 1, line: "row",
				profilerTraceClass: profilerTraceClassStructuredKnown},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if test.prepare != nil {
				test.prepare(t, sink)
			}
			if reason := profilerSinkInvariantReason(t, sink.add(test.row)); reason != "profiler_row_provenance_preassigned" {
				t.Fatalf("reason=%q", reason)
			}
			if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || sink.activeTextRows != 0 {
				t.Fatalf("rejected provenance mutated sink: stats=%+v rows=%d active_text_rows=%d",
					sink.stats, len(sink.rows), sink.activeTextRows)
			}
			sink.abortPairRowCensus()
		})
	}
}

func TestProfilerNonPublisherPairFamiliesKeepProfilerFieldsZero(t *testing.T) {
	for _, test := range []struct {
		name string
		kind pairRenderKind
	}{{name: "workqueue", kind: pairRenderWorkqueue}, {name: "dma_fence", kind: pairRenderDMAFence}} {
		t.Run(test.name+"/direct", func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "direct-pair", pairKind: test.kind}); err != nil {
				t.Fatal(err)
			}
			if got, want := sink.rows[0].profilerProvenance(), (profilerPairRowProvenance{PairKind: test.kind}); got != want {
				t.Fatalf("direct provenance=%+v want=%+v", got, want)
			}
		})
		t.Run(test.name+"/active profiler rejected", func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) || !sink.beginProfilerTextMessage() {
				t.Fatal("begin text message")
			}
			addErr := sink.add(renderedRow{tsNS: 1, seq: 1, line: "impossible-profiler-pair", pairKind: test.kind})
			if reason := profilerSinkInvariantReason(t, addErr); reason != "profiler_row_provenance_invalid" ||
				sink.stats.RowsAccepted != 0 || sink.activeTextRows != 0 {
				t.Fatalf("active Profiler admitted non-Profiler pair: reason=%q stats=%+v active=%d",
					reason, sink.stats, sink.activeTextRows)
			}
			sink.abortPairRowCensus()
		})
	}
}

func TestProfilerPairEndpointTupleRejectsMismatchBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		row    renderedRow
		reason string
	}{
		{
			name: "structured table",
			row: renderedRow{tsNS: 1, seq: 1, line: "row", pairKind: pairRenderF2FS,
				pairLane: "lane", pairTable: "f2fs_write_end", structuredPair: true, profilerEventField: 4011},
			reason: "profiler_pair_endpoint_table_mismatch",
		},
		{
			name: "structured slot",
			row: renderedRow{tsNS: 1, seq: 1, line: "row", pairKind: pairRenderF2FS,
				pairLane: "lane", pairTable: "f2fs_write_begin", structuredPair: true, profilerEventField: 4011,
				profilerEndpointSlot: profilerPairEndpointF2FSWriteEnd},
			reason: "profiler_pair_endpoint_slot_mismatch",
		},
		{
			name: "text unknown table",
			row: renderedRow{tsNS: 1, seq: 1, line: "row", pairKind: pairRenderF2FS,
				pairLane: "lane", pairTable: "f2fs_write"},
			reason: "profiler_pair_endpoint_table_unknown",
		},
		{
			name: "text cross-kind slot",
			row: renderedRow{tsNS: 1, seq: 1, line: "row", pairKind: pairRenderF2FS,
				pairLane: "lane", profilerEndpointSlot: profilerPairEndpointMMCRequestStart},
			reason: "profiler_pair_endpoint_kind_mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if reason := profilerSinkInvariantReason(t, sink.add(test.row)); reason != test.reason {
				t.Fatalf("reason=%q want=%q", reason, test.reason)
			}
			if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 ||
				sink.legacyPairProof.observations != 0 || len(sink.pairLaneRegistries[pairRenderF2FS].states) != 0 {
				t.Fatalf("rejected endpoint tuple mutated sink: stats=%+v rows=%d proof=%+v registry=%+v",
					sink.stats, len(sink.rows), sink.legacyPairProof, sink.pairLaneRegistries[pairRenderF2FS])
			}
		})
	}

	t.Run("active text requires typed slot", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) || !sink.beginProfilerTextMessage() {
			t.Fatal("begin text message")
		}
		row := renderedRow{tsNS: 1, seq: 1, line: "row", pairKind: pairRenderF2FS,
			pairLane: "lane", pairTable: "f2fs_write_begin"}
		if reason := profilerSinkInvariantReason(t, sink.add(row)); reason != "profiler_text_pair_endpoint_slot_missing" {
			t.Fatalf("reason=%q", reason)
		}
		if sink.stats.RowsAccepted != 0 || sink.activeTextRows != 0 ||
			len(sink.pairLaneRegistries[pairRenderF2FS].states) != 0 {
			t.Fatalf("missing typed slot mutated sink: stats=%+v active=%d registry=%+v",
				sink.stats, sink.activeTextRows, sink.pairLaneRegistries[pairRenderF2FS])
		}
		sink.abortPairRowCensus()
	})
}

func TestProfilerPairRowProvenancePublisherModes(t *testing.T) {
	tests := []struct {
		name       string
		publisher  profilerPairPublisherSlot
		text       bool
		structured bool
		want       profilerPairRowProvenance
	}{
		{name: "strict text", publisher: profilerPairPublisherExactFtrace, text: true,
			want: profilerPairRowProvenance{LaneID: 1, TextMessageOrdinal: 1, PairKind: pairRenderF2FS,
				EndpointSlot: profilerPairEndpointF2FSWriteBegin, PublisherSlot: profilerPairPublisherExactFtrace,
				Flags: profilerPairRowProvenanceText}},
		{name: "bytrace text", publisher: profilerPairPublisherBytrace, text: true,
			want: profilerPairRowProvenance{LaneID: 1, TextMessageOrdinal: 1, PairKind: pairRenderF2FS,
				EndpointSlot: profilerPairEndpointF2FSWriteBegin, PublisherSlot: profilerPairPublisherBytrace,
				Flags: profilerPairRowProvenanceText}},
		{name: "structured exact", publisher: profilerPairPublisherExactFtrace, structured: true,
			want: profilerPairRowProvenance{LaneID: 1, PairKind: pairRenderF2FS,
				EndpointSlot: profilerPairEndpointF2FSWriteBegin, PublisherSlot: profilerPairPublisherExactFtrace,
				Flags: profilerPairRowProvenanceStructured}},
		{name: "session", publisher: profilerPairPublisherSession,
			want: profilerPairRowProvenance{LaneID: 1, PairKind: pairRenderF2FS,
				EndpointSlot: profilerPairEndpointF2FSWriteBegin, PublisherSlot: profilerPairPublisherSession}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if !sink.beginPairRowCensusForPublisher(test.publisher) {
				t.Fatal("begin publisher census")
			}
			if test.text && !sink.beginProfilerTextMessage() {
				t.Fatal("begin text message")
			}
			row := renderedRow{
				tsNS: 1, seq: 1, line: "f2fs-row", pairKind: pairRenderF2FS,
				pairLane: "lane", pairTable: "f2fs_write_begin",
				profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
			}
			if test.structured {
				row.structuredPair = true
				row.profilerEventField = 4011
			}
			if err := sink.add(row); err != nil {
				t.Fatal(err)
			}
			if got := sink.rows[0].profilerProvenance(); got != test.want {
				t.Fatalf("provenance=%+v want=%+v", got, test.want)
			}
			if test.text {
				if err := sink.endProfilerTextMessage(1); err != nil {
					t.Fatal(err)
				}
			}
			_ = sink.endPairRowCensus()
			if sink.pairCensusActive || sink.activePairPublisher != profilerPairPublisherNone ||
				sink.textMessageActive || sink.activeTextMessage != 0 || sink.activeTextRows != 0 {
				t.Fatalf("publisher context leaked: %+v", sink)
			}
		})
	}

	t.Run("noncanonical cannot begin text", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if !sink.beginPairRowCensusForPublisher(profilerPairPublisherNoncanonicalFtrace) {
			t.Fatal("begin noncanonical census")
		}
		if sink.beginProfilerTextMessage() || sink.textMessageActive || sink.activeTextMessage != 0 ||
			sink.activeTextRows != 0 || sink.nextTextMessage != 0 {
			t.Fatalf("noncanonical publisher entered text state: %+v", sink)
		}
		sink.abortPairRowCensus()
	})
}

func TestProfilerTextMessageEndFailureCleansContexts(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) || !sink.beginProfilerTextMessage() {
		t.Fatal("begin text message")
	}
	if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "ordinary"}); err != nil {
		t.Fatal(err)
	}
	if reason := profilerSinkInvariantReason(t, sink.endProfilerTextMessage(0)); reason != "profiler_text_message_end_state_invalid" {
		t.Fatalf("reason=%q", reason)
	}
	_ = sink.endPairRowCensus()
	if sink.captureBreach != "profiler_text_message_end_state_invalid" || sink.pairCensusActive ||
		sink.activePairPublisher != profilerPairPublisherNone || sink.textMessageActive ||
		sink.activeTextMessage != 0 || sink.activeTextRows != 0 || sink.nextTextMessage != 1 {
		t.Fatalf("failed text-message close leaked context: breach=%q census=%t publisher=%d text=%t/%d/%d next=%d",
			sink.captureBreach, sink.pairCensusActive, sink.activePairPublisher, sink.textMessageActive,
			sink.activeTextMessage, sink.activeTextRows, sink.nextTextMessage)
	}
}
