package hitraceconv

import (
	"context"
	"os"
	"reflect"
	"testing"
)

func profilerTerminalTextProvenance(
	ordinal uint32,
	publisher profilerPairPublisherSlot,
	kind pairRenderKind,
	endpoint profilerPairEndpointSlot,
) profilerPairRowProvenance {
	provenance := profilerPairRowProvenance{
		TextMessageOrdinal: ordinal,
		PairKind:           kind,
		EndpointSlot:       endpoint,
		PublisherSlot:      publisher,
		Flags:              profilerPairRowProvenanceText,
	}
	if kind != pairRenderUnknown {
		provenance.LaneID = 1
	}
	return provenance
}

func profilerTerminalTypeHasDynamicState(current reflect.Type, seen map[reflect.Type]bool) bool {
	if current == nil || seen[current] {
		return false
	}
	seen[current] = true
	switch current.Kind() {
	case reflect.Map, reflect.Slice, reflect.String, reflect.Interface, reflect.Func, reflect.Chan:
		return true
	case reflect.Pointer, reflect.Array:
		return profilerTerminalTypeHasDynamicState(current.Elem(), seen)
	case reflect.Struct:
		for index := 0; index < current.NumField(); index++ {
			if profilerTerminalTypeHasDynamicState(current.Field(index).Type, seen) {
				return true
			}
		}
	}
	return false
}

func TestProfilerTerminalPublicationStateHasNoDynamicCollections(t *testing.T) {
	for _, value := range []any{
		profilerTerminalPublicationCounts{},
		profilerTerminalTextMessageLedger{},
		profilerTerminalPublicationLedger{},
		profilerTerminalPublicationBuilder{},
	} {
		typeOf := reflect.TypeOf(value)
		if profilerTerminalTypeHasDynamicState(typeOf, make(map[reflect.Type]bool)) {
			t.Fatalf("terminal publication state %v gained dynamic retained storage", typeOf)
		}
	}
}

func TestProfilerTerminalPublicationBuilderAccountsWholeMessagesAndRowClasses(t *testing.T) {
	builder := profilerTerminalPublicationBuilder{}
	rows := []struct {
		provenance  profilerPairRowProvenance
		disposition profilerSourceOrderDisposition
	}{
		{profilerTerminalTextProvenance(1, profilerPairPublisherOtherText,
			pairRenderUnknown, profilerPairEndpointNone), profilerSourceOrderDispositionPublish},
		{profilerTerminalTextProvenance(1, profilerPairPublisherOtherText,
			pairRenderF2FS, profilerPairEndpointF2FSWriteBegin), profilerSourceOrderDispositionWithhold},
		{profilerPairRowProvenance{LaneID: 1, PairKind: pairRenderMMC, EndpointSlot: profilerPairEndpointMMCRequestStart,
			PublisherSlot: profilerPairPublisherExactFtrace,
			Flags:         profilerPairRowProvenanceStructured}, profilerSourceOrderDispositionPublish},
		{profilerTerminalTextProvenance(2, profilerPairPublisherBytrace,
			pairRenderMMC, profilerPairEndpointMMCRequestDone), profilerSourceOrderDispositionWithhold},
		{profilerTerminalTextProvenance(2, profilerPairPublisherBytrace,
			pairRenderF2FS, profilerPairEndpointF2FSWriteEnd), profilerSourceOrderDispositionWithhold},
		{profilerPairRowProvenance{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherSession},
			profilerSourceOrderDispositionPublish},
	}
	for index, row := range rows {
		if !builder.observe(row.provenance, row.disposition) {
			t.Fatalf("row %d was rejected: provenance=%+v disposition=%d", index, row.provenance, row.disposition)
		}
	}
	ledger, ok := builder.finish()
	if !ok {
		t.Fatal("terminal builder did not finish")
	}
	if ledger.rows != (profilerTerminalPublicationCounts{staged: 6, published: 3, withheld: 3}) ||
		ledger.textRows != (profilerTerminalPublicationCounts{staged: 5, published: 2, withheld: 3}) ||
		ledger.structuredRows != (profilerTerminalPublicationCounts{staged: 1, published: 1}) ||
		ledger.textMessages != (profilerTerminalTextMessageLedger{
			staged: 2, published: 1, fullyWithheld: 1, pairBearing: 2,
		}) {
		t.Fatalf("terminal aggregate drifted: %+v", ledger)
	}
	if ledger.publisherFamilies[profilerPairPublisherOtherText][pairRenderF2FS].withheld != 1 ||
		ledger.publisherFamilies[profilerPairPublisherBytrace][pairRenderMMC].withheld != 1 ||
		ledger.publisherFamilies[profilerPairPublisherBytrace][pairRenderF2FS].withheld != 1 ||
		ledger.structuredEndpoints[profilerPairEndpointMMCRequestStart].published != 1 {
		t.Fatalf("terminal typed projections drifted: %+v", ledger)
	}
}

func TestProfilerTerminalPublicationBuilderRejectsMessageIdentityDrift(t *testing.T) {
	ordinary := func(ordinal uint32, publisher profilerPairPublisherSlot) profilerPairRowProvenance {
		return profilerTerminalTextProvenance(
			ordinal, publisher, pairRenderUnknown, profilerPairEndpointNone,
		)
	}
	tests := []struct {
		name string
		rows []profilerPairRowProvenance
	}{
		{name: "first ordinal skips one", rows: []profilerPairRowProvenance{
			ordinary(2, profilerPairPublisherOtherText),
		}},
		{name: "closed ordinal reappears", rows: []profilerPairRowProvenance{
			ordinary(1, profilerPairPublisherOtherText),
			{PairKind: pairRenderUnknown, PublisherSlot: profilerPairPublisherSession},
			ordinary(1, profilerPairPublisherOtherText),
		}},
		{name: "same ordinal changes publisher", rows: []profilerPairRowProvenance{
			ordinary(1, profilerPairPublisherOtherText),
			ordinary(1, profilerPairPublisherBytrace),
		}},
		{name: "ordinal jumps", rows: []profilerPairRowProvenance{
			ordinary(1, profilerPairPublisherOtherText),
			ordinary(3, profilerPairPublisherOtherText),
		}},
		{name: "ordinal descends directly", rows: []profilerPairRowProvenance{
			ordinary(1, profilerPairPublisherOtherText),
			ordinary(2, profilerPairPublisherOtherText),
			ordinary(1, profilerPairPublisherOtherText),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := profilerTerminalPublicationBuilder{}
			rejected := false
			for _, provenance := range test.rows {
				if !builder.observe(provenance, profilerSourceOrderDispositionPublish) {
					rejected = true
					break
				}
			}
			if !rejected {
				t.Fatalf("message drift was accepted: %+v", test.rows)
			}
		})
	}
}

func TestProfilerTerminalPublicationKeepsPartiallyWithheldMultiLaneMessage(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 3, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin multi-lane message")
	}
	addProfilerSidecarOrdinaryRow(t, sink, renderedRow{tsNS: 1, seq: 1, line: "ordinary"})
	for index, lane := range []string{"poisoned-lane", "clean-lane"} {
		if err := sink.addProfilerEventContext(context.Background(), renderedRow{
			tsNS: uint64(index + 2), seq: index + 2, line: lane,
			pairKind: pairRenderF2FS, pairLane: lane, pairTable: "f2fs_write_begin",
			profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
		}, traceDBProfilerEventDelta{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.endProfilerTextMessage(3); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	sink.poisonPairLane(pairRenderF2FS, "poisoned-lane")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	terminal := sink.sourceOrderSidecar.terminal
	if terminal.textMessages != (profilerTerminalTextMessageLedger{
		staged: 1, published: 1, pairBearing: 1,
	}) || terminal.textRows != (profilerTerminalPublicationCounts{
		staged: 3, published: 2, withheld: 1,
	}) || terminal.publisherFamilies[profilerPairPublisherOtherText][pairRenderF2FS] !=
		(profilerTerminalPublicationCounts{staged: 2, published: 1, withheld: 1}) {
		t.Fatalf("partial multi-lane message was collapsed: %+v", terminal)
	}
}

func TestProfilerTerminalPublicationUsesIngestOrderWhenTimestampsInterleave(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin first message")
	}
	addProfilerSidecarOrdinaryRow(t, sink, renderedRow{tsNS: 1, seq: 1, line: "message-1-first"})
	if err := sink.addProfilerEventContext(context.Background(), renderedRow{
		tsNS: 3, seq: 2, line: "message-1-last", pairKind: pairRenderF2FS,
		pairLane: "interleaved-lane", pairTable: "f2fs_write_begin",
		profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
	}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	if err := sink.endProfilerTextMessage(2); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherBytrace) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin second message")
	}
	addProfilerSidecarOrdinaryRow(t, sink, renderedRow{tsNS: 2, seq: 3, line: "message-2"})
	if err := sink.endProfilerTextMessage(1); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	sink.poisonPairLane(pairRenderF2FS, "interleaved-lane")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	terminal := sink.sourceOrderSidecar.terminal
	if terminal.textMessages != (profilerTerminalTextMessageLedger{
		staged: 2, published: 2, pairBearing: 1,
	}) || terminal.textRows != (profilerTerminalPublicationCounts{
		staged: 3, published: 2, withheld: 1,
	}) || terminal.publisherFamilies[profilerPairPublisherOtherText][pairRenderF2FS].withheld != 1 {
		t.Fatalf("timestamp-sorted 1/2/1 rows corrupted ingest-order messages: %+v", terminal)
	}
}

func TestProfilerTerminalPublicationSeparatesSharedLaneAcrossPublishers(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 2, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	publishers := []profilerPairPublisherSlot{
		profilerPairPublisherExactFtrace,
		profilerPairPublisherBytrace,
		profilerPairPublisherOtherText,
	}
	for index, publisher := range publishers {
		if !sink.beginPairRowCensusForPublisher(publisher) || !sink.beginProfilerTextMessage() {
			t.Fatalf("begin publisher %d", publisher)
		}
		if err := sink.addProfilerEventContext(context.Background(), renderedRow{
			tsNS: uint64(index + 1), seq: index + 1, line: "shared-publisher-lane",
			pairKind: pairRenderF2FS, pairLane: "shared-lane", pairTable: "f2fs_write_begin",
			profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
		}, traceDBProfilerEventDelta{}); err != nil {
			t.Fatal(err)
		}
		if err := sink.endProfilerTextMessage(1); err != nil {
			t.Fatal(err)
		}
		_ = sink.endPairRowCensus()
	}
	sink.poisonPairLane(pairRenderF2FS, "shared-lane")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	terminal := sink.sourceOrderSidecar.terminal
	if terminal.textMessages != (profilerTerminalTextMessageLedger{
		staged: 3, fullyWithheld: 3, pairBearing: 3,
	}) {
		t.Fatalf("shared-lane messages merged across publishers: %+v", terminal.textMessages)
	}
	for _, publisher := range publishers {
		if terminal.publisherFamilies[publisher][pairRenderF2FS] !=
			(profilerTerminalPublicationCounts{staged: 1, withheld: 1}) {
			t.Fatalf("publisher %d lost exact shared-lane verdict: %+v",
				publisher, terminal.publisherFamilies[publisher][pairRenderF2FS])
		}
	}
}

func TestProfilerTerminalPublicationCoversEveryStructuredEndpoint(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 3, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherExactFtrace) {
		t.Fatal("begin structured publisher")
	}
	rows := 0
	for _, descriptor := range profilerPairEndpointRoster {
		if descriptor.structuredField == 0 {
			continue
		}
		rows++
		if err := sink.addProfilerEventContext(context.Background(), renderedRow{
			tsNS: uint64(rows), seq: rows, line: descriptor.name,
			pairKind: descriptor.kind, pairLane: "structured-endpoint-lane",
			pairTable: descriptor.name, structuredPair: true,
			profilerEventField: descriptor.structuredField,
		}, traceDBProfilerEventDelta{}); err != nil {
			t.Fatal(err)
		}
	}
	_ = sink.endPairRowCensus()
	if rows != 10 {
		t.Fatalf("structured endpoint roster=%d want=10", rows)
	}
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	terminal := sink.sourceOrderSidecar.terminal
	if terminal.structuredRows != (profilerTerminalPublicationCounts{
		staged: uint64(rows), published: uint64(rows),
	}) || terminal.textMessages != (profilerTerminalTextMessageLedger{}) {
		t.Fatalf("structured endpoint aggregate drifted: %+v", terminal)
	}
	for _, descriptor := range profilerPairEndpointRoster {
		want := profilerTerminalPublicationCounts{}
		if descriptor.structuredField != 0 {
			want = profilerTerminalPublicationCounts{staged: 1, published: 1}
		}
		if terminal.structuredEndpoints[descriptor.slot] != want {
			t.Fatalf("structured endpoint %s=%+v want=%+v",
				descriptor.name, terminal.structuredEndpoints[descriptor.slot], want)
		}
	}
}

func TestProfilerTerminalPublicationLegacyParityRejectsCrossPublisherSwap(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 2, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	addMessage := func(publisher profilerPairPublisherSlot, lane string, ts uint64) profilerPairCensusSet {
		t.Helper()
		if !sink.beginPairRowCensusForPublisher(publisher) || !sink.beginProfilerTextMessage() {
			t.Fatalf("begin publisher %d", publisher)
		}
		if err := sink.addProfilerEventContext(context.Background(), renderedRow{
			tsNS: ts, seq: int(ts), line: lane,
			pairKind: pairRenderF2FS, pairLane: lane, pairTable: "f2fs_write_begin",
			profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
		}, traceDBProfilerEventDelta{}); err != nil {
			t.Fatal(err)
		}
		if err := sink.endProfilerTextMessage(1); err != nil {
			t.Fatal(err)
		}
		return sink.endPairRowCensus()
	}
	otherStaged := addMessage(profilerPairPublisherOtherText, "withheld-lane", 1)
	bytraceStaged := addMessage(profilerPairPublisherBytrace, "published-lane", 2)
	sink.poisonPairLane(pairRenderF2FS, "withheld-lane")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extraction := profilerContainerExtraction{
		TextPluginMessages: 1,
		TextRows:           1,
		TraceCoverage: []TraceDBCoverage{
			{RowsEmitted: 1, FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"}},
			{RowsEmitted: 1, FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"}},
		},
		pairPublishers: []profilerPairPublisherCensus{
			{coverageIndex: 0, publisherSlot: profilerPairPublisherOtherText, staged: otherStaged},
			{coverageIndex: 1, publisherSlot: profilerPairPublisherBytrace, staged: bytraceStaged},
		},
		textMessages: []profilerTextMessageRows{
			{total: 1, staged: otherStaged},
			{total: 1, staged: bytraceStaged},
		},
	}
	if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherOtherText, 0) ||
		!extraction.profilerPublisherCoverage.observe(profilerPairPublisherBytrace, 1) {
		t.Fatal("record fixed publisher coverage")
	}
	if err := validateProfilerTerminalPublicationParity(extraction, sink); err != nil {
		t.Fatalf("valid terminal/legacy parity failed: %v", err)
	}
	extraction.TextPluginMessages++
	err := validateProfilerTerminalPublicationParity(extraction, sink)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_message_count_mismatch" {
		t.Fatalf("legacy message overcount escaped parity: reason=%q ok=%t err=%v", reason, ok, err)
	}
	extraction.TextPluginMessages--
	otherPublisher := &sink.sourceOrderSidecar.terminal.publishers[profilerPairPublisherOtherText]
	bytracePublisher := &sink.sourceOrderSidecar.terminal.publishers[profilerPairPublisherBytrace]
	*otherPublisher, *bytracePublisher = *bytracePublisher, *otherPublisher
	err = validateProfilerTerminalPublicationParity(extraction, sink)
	reason, ok = traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_publisher_verdict_mismatch" {
		t.Fatalf("publisher terminal verdict swap escaped parity: reason=%q ok=%t err=%v", reason, ok, err)
	}
	*otherPublisher, *bytracePublisher = *bytracePublisher, *otherPublisher
	other := &sink.sourceOrderSidecar.terminal.publisherFamilies[profilerPairPublisherOtherText][pairRenderF2FS]
	bytrace := &sink.sourceOrderSidecar.terminal.publisherFamilies[profilerPairPublisherBytrace][pairRenderF2FS]
	*other, *bytrace = *bytrace, *other
	err = validateProfilerTerminalPublicationParity(extraction, sink)
	reason, ok = traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_publisher_parity_mismatch" {
		t.Fatalf("cross-publisher terminal swap escaped parity: reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerTerminalPublicationStructuredEventCoverageIsExact(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherExactFtrace) {
		t.Fatal("begin structured publisher")
	}
	if err := sink.addProfilerEventContext(context.Background(), renderedRow{
		tsNS: 1, seq: 1, line: "structured-write-begin",
		pairKind: pairRenderF2FS, pairLane: "event-lane", pairTable: "f2fs_write_begin",
		structuredPair: true, profilerEventField: 4011,
	}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	staged := sink.endPairRowCensus()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extraction := profilerContainerExtraction{
		StructuredRows: 1,
		TraceCoverage:  []TraceDBCoverage{{RowsEmitted: 1}, {RowsEmitted: 1}},
		pairPublishers: []profilerPairPublisherCensus{{
			coverageIndex: 0, publisherSlot: profilerPairPublisherExactFtrace, staged: staged,
		}},
	}
	if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherExactFtrace, 0) {
		t.Fatal("record structured publisher coverage")
	}
	eventSlot := profilerFtraceEventSlot(4011)
	extraction.profilerEventCoverage.Present[eventSlot] = true
	extraction.profilerEventCoverage.Index[eventSlot] = 1
	if err := validateProfilerTerminalPublicationParity(extraction, sink); err != nil {
		t.Fatalf("valid structured event parity failed: %v", err)
	}

	for _, test := range []struct {
		name    string
		present bool
		rows    int
	}{
		{name: "missing", present: false, rows: 1},
		{name: "under", present: true, rows: 0},
		{name: "over", present: true, rows: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := extraction
			mutated.TraceCoverage = append([]TraceDBCoverage(nil), extraction.TraceCoverage...)
			mutated.profilerEventCoverage.Present[eventSlot] = test.present
			mutated.TraceCoverage[1].RowsEmitted = test.rows
			err := validateProfilerTerminalPublicationParity(mutated, sink)
			reason, ok := traceDBOutputInvariantReason(err)
			if !ok || reason != "profiler_terminal_publication_event_coverage_mismatch" {
				t.Fatalf("event coverage %s escaped: reason=%q ok=%t err=%v", test.name, reason, ok, err)
			}
		})
	}
	mutated := extraction
	mutated.profilerEventCoverage.Index[eventSlot] = 0
	err := validateProfilerTerminalPublicationParity(mutated, sink)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_coverage_index_collision" {
		t.Fatalf("event/publisher coverage collision escaped: reason=%q ok=%t err=%v", reason, ok, err)
	}
	mutated = extraction
	secondEventSlot := profilerFtraceEventSlot(2003)
	mutated.profilerEventCoverage.Present[secondEventSlot] = true
	mutated.profilerEventCoverage.Index[secondEventSlot] = 1
	err = validateProfilerTerminalPublicationParity(mutated, sink)
	reason, ok = traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_coverage_index_collision" {
		t.Fatalf("event/event coverage collision escaped: reason=%q ok=%t err=%v", reason, ok, err)
	}
	mutated = extraction
	mutated.profilerPublisherCoverage.Present[profilerPairPublisherExactFtrace] = false
	err = validateProfilerTerminalPublicationParity(mutated, sink)
	reason, ok = traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_publisher_index_mismatch" {
		t.Fatalf("publisher row lost its coverage mapping: reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerTerminalPublicationSessionHasNoPluginMessage(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherSession) {
		t.Fatal("begin Session publisher")
	}
	if err := sink.addProfilerEventContext(context.Background(), renderedRow{
		tsNS: 1, seq: 1, line: "session-row",
	}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	staged := sink.endPairRowCensus()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extraction := profilerContainerExtraction{
		Kind:     "openharmony_profiler_session_package",
		TextRows: 1,
		TraceCoverage: []TraceDBCoverage{{
			RowsEmitted: 1,
		}},
		pairPublishers: []profilerPairPublisherCensus{{
			coverageIndex: 0,
			publisherSlot: profilerPairPublisherSession,
			staged:        staged,
		}},
	}
	if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherSession, 0) {
		t.Fatal("record Session publisher coverage")
	}
	if err := validateProfilerTerminalPublicationParity(extraction, sink); err != nil {
		t.Fatalf("valid Session terminal parity failed: %v", err)
	}
	extraction.SourceFailClosed = true
	err := validateProfilerTerminalPublicationParity(extraction, sink)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_source_fail_close_state_mismatch" {
		t.Fatalf("extraction-only source fail-close escaped: reason=%q ok=%t err=%v", reason, ok, err)
	}
	extraction.SourceFailClosed = false
	sink.allRowsFailClosed = true
	err = validateProfilerTerminalPublicationParity(extraction, sink)
	reason, ok = traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_source_fail_close_state_mismatch" {
		t.Fatalf("sink-only source fail-close escaped: reason=%q ok=%t err=%v", reason, ok, err)
	}
	sink.allRowsFailClosed = false
	extraction.TextPluginMessages = 1
	err = validateProfilerTerminalPublicationParity(extraction, sink)
	reason, ok = traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_session_message_mismatch" {
		t.Fatalf("Session acquired a plugin-message seat: reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerTerminalPublicationRejectsProductionSourceNeutralRow(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	addProfilerSidecarOrdinaryRow(t, sink, renderedRow{tsNS: 1, seq: 1, line: "source-neutral"})
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	err := validateProfilerTerminalPublicationParity(
		profilerContainerExtraction{TextRows: 1}, sink,
	)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_source_neutral_row" {
		t.Fatalf("source-neutral production row escaped: reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerTerminalPublicationMissingSidecarRequiresEmptySourceFailure(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin storage-failure message")
	}
	addProfilerSidecarOrdinaryRow(t, sink, renderedRow{tsNS: 1, seq: 1, line: "before-drift"})
	if err := sink.endProfilerTextMessage(1); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	if err := sink.flushChunkContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	rewriteProfilerSpillChunkRow(t, sink.runs[0].path, func(row *traceDBChunkRow) {
		row.Line = "after-drift"
	})
	refreshProfilerRunProof(t, sink, 0)
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatalf("storage drift was not converted to source failure: %v", err)
	}
	extraction := profilerContainerExtraction{
		TextPluginMessages: 1,
		TextRows:           1,
		TraceCoverage:      []TraceDBCoverage{{RowsEmitted: 1}},
	}
	if err := applyProfilerCaptureSourceFailure(&extraction, sink); err != nil {
		t.Fatal(err)
	}
	if err := validateProfilerTerminalPublicationParity(extraction, sink); err != nil {
		t.Fatalf("valid missing-sidecar source failure failed parity: %v", err)
	}
	extraction.TextRows = 1
	err := validateProfilerTerminalPublicationParity(extraction, sink)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_source_fail_close_mismatch" {
		t.Fatalf("missing-sidecar nonempty row account escaped: reason=%q ok=%t err=%v", reason, ok, err)
	}
	extraction.TextRows = 0
	extraction.TraceCoverage[0].RowsEmitted = 1
	err = validateProfilerTerminalPublicationParity(extraction, sink)
	reason, ok = traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_source_fail_coverage_mismatch" {
		t.Fatalf("missing-sidecar nonempty coverage escaped: reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerTerminalPublicationZeroRowsRejectsMessageCount(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	err := validateProfilerTerminalPublicationParity(
		profilerContainerExtraction{TextPluginMessages: 1}, sink,
	)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_zero_row_mismatch" {
		t.Fatalf("zero-row message count escaped parity: reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerTerminalPublicationZeroRowsValidatesRowlessPublisherCoverage(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extraction := profilerContainerExtraction{TraceCoverage: []TraceDBCoverage{{RowsEmitted: 0}}}
	if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherOtherText, 0) {
		t.Fatal("record rowless publisher coverage")
	}
	if err := validateProfilerTerminalPublicationParity(extraction, sink); err != nil {
		t.Fatalf("valid rowless publisher coverage failed: %v", err)
	}
	extraction.TraceCoverage[0].RowsEmitted = 1
	err := validateProfilerTerminalPublicationParity(extraction, sink)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_publisher_coverage_mismatch" {
		t.Fatalf("rowless publisher acquired emitted rows: reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerTerminalPublicationZeroRowsRejectsUnattributedCoverageRows(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	err := validateProfilerTerminalPublicationParity(
		profilerContainerExtraction{TraceCoverage: []TraceDBCoverage{{RowsEmitted: 1}}}, sink,
	)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_zero_row_coverage_mismatch" {
		t.Fatalf("zero-row capture acquired unattributed coverage rows: reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerPublisherCoverageIndexesRejectIdentityCollisions(t *testing.T) {
	var indexes profilerPublisherCoverageIndexes
	if !indexes.observe(profilerPairPublisherExactFtrace, 0) ||
		!indexes.observe(profilerPairPublisherExactFtrace, 0) ||
		indexes.observe(profilerPairPublisherExactFtrace, 1) ||
		indexes.observe(profilerPairPublisherBytrace, 0) ||
		indexes.observe(profilerPairPublisherNone, 2) {
		t.Fatalf("publisher coverage identity collision escaped: %+v", indexes)
	}
}

func TestProfilerTerminalPublicationRejectsValidDispositionFlipDuringConstruction(t *testing.T) {
	flipped := false
	options := traceDBRowSinkOptions{ops: traceDBRowSinkOps{writeAt: func(
		file *os.File, data []byte, offset int64,
	) (int, error) {
		if !flipped && offset >= int64(profilerSourceOrderSidecarHeaderBytes) &&
			len(data) >= int(profilerSourceOrderSidecarRecordBytes) {
			flipped = true
			mutated := append([]byte(nil), data...)
			mutated[52] = byte(profilerSourceOrderDispositionWithhold)
			return file.WriteAt(mutated, offset)
		}
		return file.WriteAt(data, offset)
	}}}
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 8, options,
	)
	defer sink.cleanup()
	addProfilerSidecarOrdinaryRow(t, sink, renderedRow{tsNS: 1, seq: 1, line: "published"})
	err := sink.sealProfilerCapture()
	if !flipped {
		t.Fatal("sidecar record disposition was not flipped")
	}
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_source_order_sidecar_disposition_mismatch" ||
		sink.sourceOrderSidecar.present() {
		t.Fatalf("valid construction-time disposition flip escaped: reason=%q ok=%t err=%v sidecar=%+v",
			reason, ok, err, sink.sourceOrderSidecar)
	}
}
