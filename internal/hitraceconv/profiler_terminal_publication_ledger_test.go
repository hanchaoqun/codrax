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

func cloneProfilerContainerExtractionForTerminalTest(
	extraction profilerContainerExtraction,
) profilerContainerExtraction {
	cloned := extraction
	if extraction.PluginMessages != nil {
		cloned.PluginMessages = make(map[string]int, len(extraction.PluginMessages))
		for key, value := range extraction.PluginMessages {
			cloned.PluginMessages[key] = value
		}
	}
	cloned.TraceCoverage = append([]TraceDBCoverage(nil), extraction.TraceCoverage...)
	for index := range cloned.TraceCoverage {
		coverage := &cloned.TraceCoverage[index]
		coverage.ColumnsPresent = append([]string(nil), coverage.ColumnsPresent...)
		coverage.ColumnsMissing = append([]string(nil), coverage.ColumnsMissing...)
		if coverage.FieldSources != nil {
			fields := make(map[string]string, len(coverage.FieldSources))
			for key, value := range coverage.FieldSources {
				fields[key] = value
			}
			coverage.FieldSources = fields
		}
	}
	cloned.Caveats = append([]string(nil), extraction.Caveats...)
	return cloned
}

func requireProfilerTerminalApplyErrorUnchanged(
	t *testing.T,
	extraction profilerContainerExtraction,
	sink *traceDBRowSink,
	wantReason string,
) {
	t.Helper()
	candidate := cloneProfilerContainerExtractionForTerminalTest(extraction)
	before := cloneProfilerContainerExtractionForTerminalTest(candidate)
	_, err := applyProfilerTerminalPublication(&candidate, sink)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != wantReason {
		t.Fatalf("terminal publication error reason=%q ok=%t want=%q err=%v",
			reason, ok, wantReason, err)
	}
	if !reflect.DeepEqual(candidate, before) {
		t.Fatalf("failed terminal projection mutated extraction:\n before=%+v\n after=%+v",
			before, candidate)
	}
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

func TestProfilerContainerExtractionHasNoLegacyPublicationArrays(t *testing.T) {
	typeOf := reflect.TypeOf(profilerContainerExtraction{})
	for _, field := range []string{"pairPublishers", "textMessages"} {
		if _, present := typeOf.FieldByName(field); present {
			t.Fatalf("profiler extraction retained legacy publication array %q", field)
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

func TestProfilerTerminalPublicationProjectsStagedTextCountsFromFixedCoverage(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 2, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	addMessage := func(publisher profilerPairPublisherSlot, lane string, ts uint64) {
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
		_ = sink.endPairRowCensus()
	}
	addMessage(profilerPairPublisherOtherText, "withheld-lane", 1)
	addMessage(profilerPairPublisherBytrace, "published-lane", 2)
	sink.poisonPairLane(pairRenderF2FS, "withheld-lane")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	staged := profilerContainerExtraction{
		Messages:                 2,
		TextPluginMessages:       2,
		TextRows:                 2,
		Caveats:                  []string{"before publication", "after publication"},
		publicationCaveatPending: true,
		publicationCaveatIndex:   1,
		TraceCoverage: []TraceDBCoverage{
			{RowsEmitted: 1, FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"}},
			{RowsEmitted: 1, FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"}},
		},
	}
	if !staged.profilerPublisherCoverage.observe(profilerPairPublisherOtherText, 0) ||
		!staged.profilerPublisherCoverage.observe(profilerPairPublisherBytrace, 1) {
		t.Fatal("record fixed publisher coverage")
	}
	projected := cloneProfilerContainerExtractionForTerminalTest(staged)
	terminal, err := applyProfilerTerminalPublication(&projected, sink)
	if err != nil {
		t.Fatalf("valid staged terminal projection failed: %v", err)
	}
	if projected.TextPluginMessages != 1 || projected.TextRows != 1 || projected.StructuredRows != 0 {
		t.Fatalf("staged counts were not projected from the terminal ledger: %+v", projected)
	}
	if terminal.textMessages != (profilerTerminalTextMessageLedger{
		staged: 2, published: 1, fullyWithheld: 1, pairBearing: 2,
	}) || terminal.textRows != (profilerTerminalPublicationCounts{
		staged: 2, published: 1, withheld: 1,
	}) || terminal.publisherFamilies[profilerPairPublisherOtherText][pairRenderF2FS] !=
		(profilerTerminalPublicationCounts{staged: 1, withheld: 1}) ||
		terminal.publisherFamilies[profilerPairPublisherBytrace][pairRenderF2FS] !=
			(profilerTerminalPublicationCounts{staged: 1, published: 1}) {
		t.Fatalf("terminal authority drifted: %+v", terminal)
	}
	if projected.TraceCoverage[0].RowsEmitted != 0 ||
		projected.TraceCoverage[0].FieldSources["complete_capture_withheld_rows"] != "1" ||
		projected.TraceCoverage[1].RowsEmitted != 1 ||
		projected.TraceCoverage[1].FieldSources["complete_capture_withheld_rows"] != "" ||
		projected.profilerPublisherCoverage != staged.profilerPublisherCoverage {
		t.Fatalf("terminal verdict was not projected onto fixed publisher coverage: %+v", projected)
	}
	wantCaveats := []string{
		"before publication",
		"extracted 1 systrace text row(s) from 1 profiler plugin message(s)",
		"after publication",
	}
	if projected.publicationCaveatPending || !projected.terminalPublicationApplied ||
		!reflect.DeepEqual(projected.Caveats, wantCaveats) {
		t.Fatalf("deferred publication caveat drifted: got=%q pending=%t want=%q",
			projected.Caveats, projected.publicationCaveatPending, wantCaveats)
	}
	if staged.TraceCoverage[0].RowsEmitted != 1 ||
		staged.TraceCoverage[0].FieldSources["complete_capture_withheld_rows"] != "" ||
		!staged.publicationCaveatPending || !reflect.DeepEqual(staged.Caveats,
		[]string{"before publication", "after publication"}) {
		t.Fatalf("projection mutated the staged input authority: %+v", staged)
	}

	overcount := cloneProfilerContainerExtractionForTerminalTest(staged)
	overcount.TextPluginMessages++
	requireProfilerTerminalApplyErrorUnchanged(t, overcount, sink,
		"profiler_terminal_publication_message_count_mismatch")

	missingFamily := cloneProfilerContainerExtractionForTerminalTest(staged)
	delete(missingFamily.TraceCoverage[0].FieldSources, profilerCoverageF2FSStagedRows)
	requireProfilerTerminalApplyErrorUnchanged(t, missingFamily, sink,
		"profiler_terminal_publication_publisher_family_coverage_mismatch")

	wrongFamily := cloneProfilerContainerExtractionForTerminalTest(staged)
	wrongFamily.TraceCoverage[1].FieldSources[profilerCoverageF2FSStagedRows] = "2"
	requireProfilerTerminalApplyErrorUnchanged(t, wrongFamily, sink,
		"profiler_terminal_publication_publisher_family_coverage_mismatch")

	explicitZeroFamily := cloneProfilerContainerExtractionForTerminalTest(staged)
	explicitZeroFamily.TraceCoverage[0].FieldSources[profilerCoverageMMCStagedRows] = "0"
	requireProfilerTerminalApplyErrorUnchanged(t, explicitZeroFamily, sink,
		"profiler_terminal_publication_publisher_family_coverage_mismatch")

	negativeCaveatIndex := cloneProfilerContainerExtractionForTerminalTest(staged)
	negativeCaveatIndex.publicationCaveatIndex = -1
	requireProfilerTerminalApplyErrorUnchanged(t, negativeCaveatIndex, sink,
		"profiler_terminal_publication_caveat_index_invalid")

	pastCaveatIndex := cloneProfilerContainerExtractionForTerminalTest(staged)
	pastCaveatIndex.publicationCaveatIndex = len(pastCaveatIndex.Caveats) + 1
	requireProfilerTerminalApplyErrorUnchanged(t, pastCaveatIndex, sink,
		"profiler_terminal_publication_caveat_index_invalid")

	requireProfilerTerminalApplyErrorUnchanged(t, projected, sink,
		"profiler_terminal_publication_already_applied")

	// Terminal publisher verdicts are authoritative, but they must remain
	// internally consistent with the authenticated publisher-family projection.
	otherPublisher := &sink.sourceOrderSidecar.terminal.publishers[profilerPairPublisherOtherText]
	bytracePublisher := &sink.sourceOrderSidecar.terminal.publishers[profilerPairPublisherBytrace]
	*otherPublisher, *bytracePublisher = *bytracePublisher, *otherPublisher
	requireProfilerTerminalApplyErrorUnchanged(t, staged, sink,
		"profiler_terminal_publication_publisher_verdict_mismatch")
	*otherPublisher, *bytracePublisher = *bytracePublisher, *otherPublisher
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
	_ = sink.endPairRowCensus()
	sink.poisonPairLane(pairRenderF2FS, "event-lane")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extraction := profilerContainerExtraction{
		Messages:                 1,
		StructuredFtrace:         1,
		StructuredRows:           1,
		Caveats:                  []string{"before publication", "after publication"},
		publicationCaveatPending: true,
		publicationCaveatIndex:   1,
		TraceCoverage: []TraceDBCoverage{
			{RowsEmitted: 1, FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"}},
			{RowsEmitted: 1},
		},
	}
	if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherExactFtrace, 0) {
		t.Fatal("record structured publisher coverage")
	}
	eventSlot := profilerFtraceEventSlot(4011)
	extraction.profilerEventCoverage.Present[eventSlot] = true
	extraction.profilerEventCoverage.Index[eventSlot] = 1
	projected := cloneProfilerContainerExtractionForTerminalTest(extraction)
	terminal, err := applyProfilerTerminalPublication(&projected, sink)
	if err != nil {
		t.Fatalf("valid structured event projection failed: %v", err)
	}
	if projected.StructuredRows != 0 || projected.TextRows != 0 ||
		terminal.structuredEndpoints[profilerPairEndpointF2FSWriteBegin] !=
			(profilerTerminalPublicationCounts{staged: 1, withheld: 1}) ||
		projected.TraceCoverage[0].RowsEmitted != 0 ||
		projected.TraceCoverage[0].FieldSources["complete_capture_withheld_rows"] != "1" ||
		projected.TraceCoverage[1].RowsEmitted != 0 ||
		projected.TraceCoverage[1].FieldSources["complete_capture_withheld_rows"] != "1" {
		t.Fatalf("structured terminal projection drifted: projected=%+v terminal=%+v",
			projected, terminal)
	}
	wantCaveat := "decoded 1 authoritative ftrace-plugin TracePluginResult message(s) and rendered 0 structured trace row(s); unsupported or degraded members remain explicit in typed coverage"
	if projected.publicationCaveatPending || !projected.terminalPublicationApplied ||
		len(projected.Caveats) != 3 ||
		projected.Caveats[1] != wantCaveat {
		t.Fatalf("structured deferred caveat drifted: %+v", projected)
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
			mutated := cloneProfilerContainerExtractionForTerminalTest(extraction)
			mutated.profilerEventCoverage.Present[eventSlot] = test.present
			mutated.TraceCoverage[1].RowsEmitted = test.rows
			requireProfilerTerminalApplyErrorUnchanged(t, mutated, sink,
				"profiler_terminal_publication_event_coverage_mismatch")
		})
	}
	mutated := cloneProfilerContainerExtractionForTerminalTest(extraction)
	mutated.profilerEventCoverage.Index[eventSlot] = 0
	requireProfilerTerminalApplyErrorUnchanged(t, mutated, sink,
		"profiler_terminal_publication_coverage_index_collision")

	mutated = cloneProfilerContainerExtractionForTerminalTest(extraction)
	mutated.profilerPublisherCoverage.Present[profilerPairPublisherExactFtrace] = false
	requireProfilerTerminalApplyErrorUnchanged(t, mutated, sink,
		"profiler_terminal_publication_publisher_index_mismatch")

	mutated = cloneProfilerContainerExtractionForTerminalTest(extraction)
	delete(mutated.TraceCoverage[0].FieldSources, profilerCoverageF2FSStagedRows)
	requireProfilerTerminalApplyErrorUnchanged(t, mutated, sink,
		"profiler_terminal_publication_publisher_family_coverage_mismatch")
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
		pairKind: pairRenderF2FS, pairLane: "session-withheld-lane",
		pairTable: "f2fs_write_begin", profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
	}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	sink.poisonPairLane(pairRenderF2FS, "session-withheld-lane")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extraction := profilerContainerExtraction{
		Kind:                     "openharmony_profiler_session_package",
		TextRows:                 1,
		Caveats:                  []string{"before publication", "after publication"},
		publicationCaveatPending: true,
		publicationCaveatIndex:   1,
		TraceCoverage: []TraceDBCoverage{{
			RowsEmitted:  1,
			FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"},
		}},
	}
	if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherSession, 0) {
		t.Fatal("record Session publisher coverage")
	}
	projected := cloneProfilerContainerExtractionForTerminalTest(extraction)
	terminal, err := applyProfilerTerminalPublication(&projected, sink)
	if err != nil {
		t.Fatalf("valid Session terminal projection failed: %v", err)
	}
	if projected.TextRows != 0 || projected.TextPluginMessages != 0 ||
		terminal.textRows != (profilerTerminalPublicationCounts{staged: 1, withheld: 1}) ||
		terminal.textMessages != (profilerTerminalTextMessageLedger{}) ||
		terminal.publishers[profilerPairPublisherSession] !=
			(profilerTerminalPublicationCounts{staged: 1, withheld: 1}) ||
		projected.TraceCoverage[0].RowsEmitted != 0 ||
		projected.TraceCoverage[0].FieldSources["complete_capture_withheld_rows"] != "1" {
		t.Fatalf("Session terminal projection drifted: projected=%+v terminal=%+v",
			projected, terminal)
	}
	wantSkipped := "session pair-critical endpoint rows were staged but withheld by exact-lane or source-family full-capture barriers"
	wantCaveat := "profiler session package staged exact pair-critical rows, but exact-lane or source-family full-capture barriers withheld them before publication: mmc=0 f2fs=1 block=0"
	if projected.TraceCoverage[0].Skipped != wantSkipped ||
		projected.publicationCaveatPending || !projected.terminalPublicationApplied ||
		len(projected.Caveats) != 3 ||
		projected.Caveats[1] != wantCaveat {
		t.Fatalf("Session deferred disclosure drifted: %+v", projected)
	}

	mutated := cloneProfilerContainerExtractionForTerminalTest(extraction)
	mutated.SourceFailClosed = true
	requireProfilerTerminalApplyErrorUnchanged(t, mutated, sink,
		"profiler_terminal_publication_source_fail_close_state_mismatch")

	sink.allRowsFailClosed = true
	requireProfilerTerminalApplyErrorUnchanged(t, extraction, sink,
		"profiler_terminal_publication_source_fail_close_state_mismatch")
	sink.allRowsFailClosed = false

	mutated = cloneProfilerContainerExtractionForTerminalTest(extraction)
	mutated.TextPluginMessages = 1
	requireProfilerTerminalApplyErrorUnchanged(t, mutated, sink,
		"profiler_terminal_publication_session_message_mismatch")
}

func TestProfilerTerminalPublicationSessionDisclosurePublishedAndRowless(t *testing.T) {
	t.Run("published", func(t *testing.T) {
		sink := newProfilerSourceLifecycleCapture(
			t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
		)
		defer sink.cleanup()
		if !sink.beginPairRowCensusForPublisher(profilerPairPublisherSession) {
			t.Fatal("begin Session publisher")
		}
		if err := sink.addProfilerEventContext(context.Background(), renderedRow{
			tsNS: 1, seq: 1, line: "published-session-row",
		}, traceDBProfilerEventDelta{}); err != nil {
			t.Fatal(err)
		}
		_ = sink.endPairRowCensus()
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		extraction := profilerContainerExtraction{
			Kind:                     "openharmony_profiler_session_package",
			TextRows:                 1,
			Caveats:                  []string{"after publication"},
			publicationCaveatPending: true,
			TraceCoverage:            []TraceDBCoverage{{RowsEmitted: 1}},
		}
		if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherSession, 0) {
			t.Fatal("record Session publisher coverage")
		}
		terminal, err := applyProfilerTerminalPublication(&extraction, sink)
		if err != nil {
			t.Fatal(err)
		}
		wantCaveats := []string{
			"extracted 1 systrace text row(s) from profiler session package payload",
			"after publication",
		}
		if extraction.TextRows != 1 || extraction.TraceCoverage[0].RowsEmitted != 1 ||
			extraction.TraceCoverage[0].Skipped != "" || extraction.publicationCaveatPending ||
			!extraction.terminalPublicationApplied ||
			!reflect.DeepEqual(extraction.Caveats, wantCaveats) ||
			terminal.textRows != (profilerTerminalPublicationCounts{staged: 1, published: 1}) {
			t.Fatalf("published Session disclosure drifted: extraction=%+v terminal=%+v",
				extraction, terminal)
		}
	})

	t.Run("rowless", func(t *testing.T) {
		sink := newProfilerSourceLifecycleCapture(
			t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
		)
		defer sink.cleanup()
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		extraction := profilerContainerExtraction{
			Kind:                     "openharmony_profiler_session_package",
			Caveats:                  []string{"after publication"},
			publicationCaveatPending: true,
			TraceCoverage:            []TraceDBCoverage{{RowsEmitted: 0}},
		}
		if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherSession, 0) {
			t.Fatal("record rowless Session publisher coverage")
		}
		terminal, err := applyProfilerTerminalPublication(&extraction, sink)
		if err != nil {
			t.Fatal(err)
		}
		wantSkipped := "session package did not contain directly renderable systrace text rows"
		wantCaveats := []string{
			"session package did not contain directly renderable systrace text rows; attach extracted sidecars or export ftrace/bytrace text with the official profiler tooling",
			"after publication",
		}
		if terminal != (profilerTerminalPublicationLedger{}) ||
			extraction.TraceCoverage[0].Skipped != wantSkipped ||
			extraction.publicationCaveatPending || !extraction.terminalPublicationApplied ||
			!reflect.DeepEqual(extraction.Caveats, wantCaveats) {
			t.Fatalf("rowless Session disclosure drifted: extraction=%+v terminal=%+v",
				extraction, terminal)
		}

		missingMapping := profilerContainerExtraction{
			Kind:                     "openharmony_profiler_session_package",
			publicationCaveatPending: true,
			TraceCoverage:            []TraceDBCoverage{{RowsEmitted: 0}},
		}
		requireProfilerTerminalApplyErrorUnchanged(t, missingMapping, sink,
			"profiler_terminal_publication_publisher_index_mismatch")
	})
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
	requireProfilerTerminalApplyErrorUnchanged(t,
		profilerContainerExtraction{TextRows: 1, publicationCaveatPending: true}, sink,
		"profiler_terminal_publication_source_neutral_row")
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
		Messages:                 1,
		TextPluginMessages:       1,
		TextRows:                 1,
		publicationCaveatPending: true,
		TraceCoverage:            []TraceDBCoverage{{RowsEmitted: 1}},
	}
	if err := applyProfilerCaptureSourceFailure(&extraction, sink); err != nil {
		t.Fatal(err)
	}
	sourceFailure := cloneProfilerContainerExtractionForTerminalTest(extraction)
	terminal, err := applyProfilerTerminalPublication(&sourceFailure, sink)
	if err != nil {
		t.Fatalf("valid missing-sidecar source failure projection failed: %v", err)
	}
	wantSourceFailure := cloneProfilerContainerExtractionForTerminalTest(extraction)
	wantSourceFailure.publicationCaveatPending = false
	wantSourceFailure.terminalPublicationApplied = true
	if terminal != (profilerTerminalPublicationLedger{}) ||
		!reflect.DeepEqual(sourceFailure, wantSourceFailure) {
		t.Fatalf("source failure acquired substitute publication: terminal=%+v before=%+v after=%+v",
			terminal, extraction, sourceFailure)
	}

	nonemptyRows := cloneProfilerContainerExtractionForTerminalTest(extraction)
	nonemptyRows.TextRows = 1
	requireProfilerTerminalApplyErrorUnchanged(t, nonemptyRows, sink,
		"profiler_terminal_publication_source_fail_close_mismatch")

	nonemptyCoverage := cloneProfilerContainerExtractionForTerminalTest(extraction)
	nonemptyCoverage.TraceCoverage[0].RowsEmitted = 1
	requireProfilerTerminalApplyErrorUnchanged(t, nonemptyCoverage, sink,
		"profiler_terminal_publication_source_fail_coverage_mismatch")
}

func TestProfilerTerminalPublicationSourceFailureWithSidecarSkipsNormalMessageInvariant(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 2, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin non-pair source-failure message")
	}
	addProfilerSidecarOrdinaryRow(t, sink, renderedRow{
		tsNS: 1, seq: 1, line: "ordinary-source-failure-row",
	})
	if err := sink.endProfilerTextMessage(1); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin pair-bearing source-failure message")
	}
	if err := sink.addProfilerEventContext(context.Background(), renderedRow{
		tsNS: 2, seq: 2, line: "pair-source-failure-row",
		pairKind: pairRenderF2FS, pairLane: "source-failure-lane",
		pairTable: "f2fs_write_begin", profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
	}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	if err := sink.endProfilerTextMessage(1); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	sink.failCloseAllRows()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	// Mirror the already fail-closed public extraction state. StructuredFtrace is
	// a decoded-frame diagnostic (not a terminal row-class counter); setting it
	// here selects the source-failure wording while this fixture proves that the
	// normal-only fullyWithheld<=pairBearing rule is not applied after fail-close.
	extraction := profilerContainerExtraction{
		Messages:                 2,
		StructuredFtrace:         1,
		SourceFailClosed:         true,
		SourceFailReason:         "test_source_failure",
		Caveats:                  []string{"source failure resource verdict", "after publication"},
		publicationCaveatPending: true,
		publicationCaveatIndex:   1,
		TraceCoverage: []TraceDBCoverage{{
			RowsEmitted:  0,
			FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"},
		}},
	}
	if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherOtherText, 0) {
		t.Fatal("record source-failure publisher coverage")
	}
	terminal, err := applyProfilerTerminalPublication(&extraction, sink)
	if err != nil {
		t.Fatal(err)
	}
	wantCaveats := []string{
		"source failure resource verdict",
		"decoded 1 authoritative ftrace-plugin TracePluginResult message(s), but all structured rows were withheld by the profiler trace-body source fail-close",
		"after publication",
	}
	if terminal.textMessages != (profilerTerminalTextMessageLedger{
		staged: 2, fullyWithheld: 2, pairBearing: 1,
	}) || terminal.rows != (profilerTerminalPublicationCounts{staged: 2, withheld: 2}) ||
		extraction.TextRows != 0 || extraction.TextPluginMessages != 0 ||
		extraction.TraceCoverage[0].RowsEmitted != 0 || extraction.publicationCaveatPending ||
		!extraction.terminalPublicationApplied ||
		!reflect.DeepEqual(extraction.Caveats, wantCaveats) {
		t.Fatalf("source-failure terminal projection drifted: extraction=%+v terminal=%+v",
			extraction, terminal)
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
	requireProfilerTerminalApplyErrorUnchanged(t,
		profilerContainerExtraction{
			TextPluginMessages: 1, publicationCaveatPending: true,
		}, sink,
		"profiler_terminal_publication_zero_row_mismatch")
}

func TestProfilerTerminalPublicationZeroRowsValidatesRowlessPublisherCoverage(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extraction := profilerContainerExtraction{
		Caveats:                  []string{"after publication"},
		publicationCaveatPending: true,
		TraceCoverage:            []TraceDBCoverage{{RowsEmitted: 0}},
	}
	if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherOtherText, 0) {
		t.Fatal("record rowless publisher coverage")
	}
	projected := cloneProfilerContainerExtractionForTerminalTest(extraction)
	terminal, err := applyProfilerTerminalPublication(&projected, sink)
	if err != nil {
		t.Fatalf("valid rowless publisher coverage failed: %v", err)
	}
	wantCaveats := []string{
		"official profiler header was present, but no length-prefixed ProfilerPluginData messages were readable",
		"after publication",
	}
	if terminal != (profilerTerminalPublicationLedger{}) ||
		projected.publicationCaveatPending || !projected.terminalPublicationApplied ||
		!reflect.DeepEqual(projected.Caveats, wantCaveats) {
		t.Fatalf("rowless publication disclosure drifted: terminal=%+v before=%+v after=%+v",
			terminal, extraction, projected)
	}
	nonempty := cloneProfilerContainerExtractionForTerminalTest(extraction)
	nonempty.TraceCoverage[0].RowsEmitted = 1
	requireProfilerTerminalApplyErrorUnchanged(t, nonempty, sink,
		"profiler_terminal_publication_publisher_coverage_mismatch")
}

func TestProfilerTerminalPublicationZeroRowsRejectsUnattributedCoverageRows(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	requireProfilerTerminalApplyErrorUnchanged(t,
		profilerContainerExtraction{
			publicationCaveatPending: true,
			TraceCoverage:            []TraceDBCoverage{{RowsEmitted: 1}},
		}, sink,
		"profiler_terminal_publication_zero_row_coverage_mismatch")
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

func TestProfilerTerminalCoverageIndexesRejectEventIdentityCollisions(t *testing.T) {
	extraction := profilerContainerExtraction{
		TraceCoverage: []TraceDBCoverage{{RowsEmitted: 0}},
	}
	first := profilerFtraceEventSlot(4011)
	second := profilerFtraceEventSlot(4016)
	extraction.profilerEventCoverage.Present[first] = true
	extraction.profilerEventCoverage.Index[first] = 0
	extraction.profilerEventCoverage.Present[second] = true
	extraction.profilerEventCoverage.Index[second] = 0
	err := validateProfilerTerminalCoverageIndexes(
		extraction, profilerTerminalPublicationLedger{},
	)
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_terminal_publication_coverage_index_collision" {
		t.Fatalf("event/event coverage collision escaped: reason=%q ok=%t err=%v",
			reason, ok, err)
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
