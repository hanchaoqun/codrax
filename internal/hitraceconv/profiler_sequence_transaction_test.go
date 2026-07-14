package hitraceconv

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

type profilerSequenceAuthorityResult struct {
	rows     int
	coverage []TraceDBCoverage
	err      error
}

type profilerSequenceAuthorityCase struct {
	name             string
	publisher        profilerPairPublisherSlot
	text             bool
	structured       bool
	wholeStageAtomic bool
	run              func(context.Context, bool, *int, *traceDBRowSink) profilerSequenceAuthorityResult
}

func profilerSequenceStructuredFixture(t *testing.T, pairFirst bool) profilerTracePluginResult {
	t.Helper()
	pair := syntheticTracePluginFtraceEvent(1_000_000_000, 100, 100, "io", 4011, protoPayload(
		protoVarint(1, 260),
		protoVarint(2, 0xb9b8e),
		protoVarint(3, 0),
		protoVarint(4, 4096),
	))
	ordinary := syntheticTracePluginFtraceEvent(1_001_000_000, 100, 100, "app", 1109, protoPayload(
		protoVarint(1, 0),
		protoBytes(2, []byte("B|100|Frame")),
	))
	events := [][]byte{ordinary, pair}
	if pairFirst {
		events = [][]byte{pair, ordinary}
	}
	detail := protoPayload(protoVarint(1, 2), events[0], events[1])
	result := decodeProfilerTracePluginResult(protoMessage(2, detail))
	if result.Disposition != profilerFtracePayloadStructured {
		t.Fatalf("structured sequence fixture was not recognized: %+v", result)
	}
	return result
}

func profilerSequenceAuthorityCases(t *testing.T) []profilerSequenceAuthorityCase {
	t.Helper()
	pair, _, _ := profilerStrictTextF2FSFixture(t)
	ordinary := "worker-7 (7) [001] .... 0.999000: print: B|7|Frame"
	textPayload := func(pairFirst bool) []byte {
		lines := []string{ordinary, pair}
		if pairFirst {
			lines = []string{pair, ordinary}
		}
		return []byte(strings.Join(lines, "\n") + "\n")
	}
	return []profilerSequenceAuthorityCase{
		{
			name: "generic", publisher: profilerPairPublisherBytrace, text: true,
			run: func(ctx context.Context, pairFirst bool, seq *int, sink *traceDBRowSink) profilerSequenceAuthorityResult {
				rows, err := addSystraceRowsFromBytesContext(ctx, textPayload(pairFirst), seq, sink)
				return profilerSequenceAuthorityResult{rows: rows, err: err}
			},
		},
		{
			name: "strict", publisher: profilerPairPublisherExactFtrace, text: true, wholeStageAtomic: true,
			run: func(ctx context.Context, pairFirst bool, seq *int, sink *traceDBRowSink) profilerSequenceAuthorityResult {
				stage, err := stageProfilerStrictSystracePayloadContext(ctx, textPayload(pairFirst))
				if err != nil {
					return profilerSequenceAuthorityResult{err: err}
				}
				rows, recognized, err := addProfilerStrictSystraceStageContext(ctx, stage, seq, sink)
				if err == nil && !recognized {
					err = &traceDBOutputInvariantError{Reason: "strict_sequence_fixture_not_recognized"}
				}
				return profilerSequenceAuthorityResult{rows: rows, err: err}
			},
		},
		{
			name: "structured", publisher: profilerPairPublisherExactFtrace, structured: true,
			run: func(ctx context.Context, pairFirst bool, seq *int, sink *traceDBRowSink) profilerSequenceAuthorityResult {
				rows, coverage, err := renderProfilerFtraceStructuredResultWithEnvelopeCoverageContext(
					ctx, profilerSequenceStructuredFixture(t, pairFirst), seq, sink, true, nil)
				return profilerSequenceAuthorityResult{rows: rows, coverage: coverage, err: err}
			},
		},
	}
}

func beginProfilerSequencePublisher(t testing.TB, sink *traceDBRowSink, lane profilerSequenceAuthorityCase) {
	t.Helper()
	if !sink.beginPairRowCensusForPublisher(lane.publisher) {
		t.Fatalf("begin %s publisher census", lane.name)
	}
	if lane.text && !sink.beginProfilerTextMessage() {
		t.Fatalf("begin %s text message", lane.name)
	}
}

func requireProfilerSequenceInvariant(t testing.TB, err error, want string) {
	t.Helper()
	var invariant *traceDBOutputInvariantError
	if !errors.As(err, &invariant) {
		t.Fatalf("error=%T %v, want typed invariant %q", err, err, want)
	}
	if invariant.Reason != want {
		t.Fatalf("invariant reason=%q want=%q (err=%v)", invariant.Reason, want, err)
	}
}

func assertProfilerSequenceNoRowMutation(t testing.TB, sink *traceDBRowSink,
	lane profilerSequenceAuthorityCase,
) {
	t.Helper()
	registry := sink.pairLaneRegistries[pairRenderF2FS]
	census := sink.activePairCensus[pairRenderF2FS]
	if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(sink.rowIngestOrdinals) != 0 ||
		sink.nextIngestOrdinal != 0 || sink.bufferedBytes != 0 || len(sink.runs) != 0 ||
		sink.pairRows[pairRenderF2FS] != 0 || len(sink.pairLaneRows[pairRenderF2FS]) != 0 ||
		sink.structuredPairRows[pairRenderF2FS] != 0 || len(sink.structuredLaneRows[pairRenderF2FS]) != 0 ||
		len(sink.structuredEventRows[pairRenderF2FS]) != 0 ||
		len(registry.byKey) != 0 || len(registry.keys) != 0 || len(registry.states) != 0 ||
		census.total != 0 || len(census.byLane) != 0 || sink.poisoned[pairRenderF2FS] ||
		len(sink.poisonedLanes[pairRenderF2FS]) != 0 || sink.opaque[pairRenderF2FS] ||
		sink.legacyPairProof.observations != 0 || sink.legacyPairProof.laneKeys != 0 ||
		sink.pairAuthorityFailure != "" {
		t.Fatalf("%s invalid sequence mutated current row/delta/registry/census: stats=%+v rows=%d runs=%d next=%d bytes=%d pair=%d lanes=%v structured=%d/%v/%v registry=%+v census=%+v poisoned=%v/%v opaque=%v proof=%+v authority=%q",
			lane.name, sink.stats, len(sink.rows), len(sink.runs), sink.nextIngestOrdinal,
			sink.bufferedBytes, sink.pairRows[pairRenderF2FS], sink.pairLaneRows[pairRenderF2FS],
			sink.structuredPairRows[pairRenderF2FS], sink.structuredLaneRows[pairRenderF2FS],
			sink.structuredEventRows[pairRenderF2FS], registry, census, sink.poisoned[pairRenderF2FS],
			sink.poisonedLanes[pairRenderF2FS], sink.opaque[pairRenderF2FS], sink.legacyPairProof,
			sink.pairAuthorityFailure)
	}
	wantTextMessage := uint32(0)
	if lane.text {
		wantTextMessage = 1
	}
	if !sink.pairCensusActive || sink.activePairPublisher != lane.publisher ||
		sink.textMessageActive != lane.text || sink.activeTextMessage != wantTextMessage ||
		sink.activeTextRows != 0 || sink.nextTextMessage != 0 {
		t.Fatalf("%s invalid sequence drifted publisher context: census=%t publisher=%d text=%t/%d/%d next=%d",
			lane.name, sink.pairCensusActive, sink.activePairPublisher, sink.textMessageActive,
			sink.activeTextMessage, sink.activeTextRows, sink.nextTextMessage)
	}
}

func assertProfilerSequencePrefix(t testing.TB, sink *traceDBRowSink,
	lane profilerSequenceAuthorityCase, pairFirst bool,
) {
	t.Helper()
	if sink.stats.RowsAccepted != 1 || len(sink.rows) != 1 || len(sink.rowIngestOrdinals) != 1 ||
		sink.nextIngestOrdinal != 1 || len(sink.runs) != 0 {
		t.Fatalf("%s prefix was not exactly one committed row: stats=%+v rows=%d ordinals=%d next=%d runs=%d",
			lane.name, sink.stats, len(sink.rows), len(sink.rowIngestOrdinals), sink.nextIngestOrdinal,
			len(sink.runs))
	}
	row := sink.rows[0]
	provenance := row.profilerProvenance()
	wantFlags := profilerPairRowProvenanceFlags(0)
	wantMessage := uint32(0)
	if lane.text {
		wantFlags = profilerPairRowProvenanceText
		wantMessage = 1
	}
	if lane.structured && pairFirst {
		wantFlags = profilerPairRowProvenanceStructured
	}
	if row.seq != math.MaxInt-1 || provenance.PublisherSlot != lane.publisher ||
		provenance.Flags != wantFlags || provenance.TextMessageOrdinal != wantMessage {
		t.Fatalf("%s prefix row/provenance drifted: row=%+v provenance=%+v want_publisher=%d flags=%d message=%d",
			lane.name, row, provenance, lane.publisher, wantFlags, wantMessage)
	}
	wantActiveRows := 0
	if lane.text {
		wantActiveRows = 1
	}
	if sink.activeTextRows != wantActiveRows {
		t.Fatalf("%s prefix text counter=%d want=%d", lane.name, sink.activeTextRows, wantActiveRows)
	}

	registry := sink.pairLaneRegistries[pairRenderF2FS]
	census := sink.activePairCensus[pairRenderF2FS]
	if !pairFirst {
		if provenance.PairKind != pairRenderUnknown || sink.pairRows[pairRenderF2FS] != 0 ||
			len(sink.pairLaneRows[pairRenderF2FS]) != 0 || sink.structuredPairRows[pairRenderF2FS] != 0 ||
			len(registry.byKey) != 0 || len(registry.keys) != 0 || len(registry.states) != 0 ||
			census.total != 0 || len(census.byLane) != 0 || sink.legacyPairProof.observations != 0 {
			t.Fatalf("%s rejected second pair leaked into ordinary prefix: row=%+v pair=%d lanes=%v structured=%d registry=%+v census=%+v proof=%+v",
				lane.name, row, sink.pairRows[pairRenderF2FS], sink.pairLaneRows[pairRenderF2FS],
				sink.structuredPairRows[pairRenderF2FS], registry, census, sink.legacyPairProof)
		}
		return
	}
	laneKey := ""
	if len(registry.keys) == 1 {
		laneKey = registry.keys[0]
	}
	if provenance.PairKind != pairRenderF2FS || laneKey == "" ||
		provenance.EndpointSlot != profilerPairEndpointF2FSWriteBegin || provenance.LaneID == 0 ||
		sink.pairRows[pairRenderF2FS] != 1 || sink.pairLaneRows[pairRenderF2FS][laneKey] != 1 ||
		census.total != 1 || census.byLane[laneKey] != 1 || len(registry.byKey) != 1 ||
		len(registry.keys) != 1 || len(registry.states) != 1 || sink.legacyPairProof.observations != 1 {
		t.Fatalf("%s pair prefix is incomplete: row=%+v provenance=%+v pair=%d lanes=%v registry=%+v census=%+v proof=%+v",
			lane.name, row, provenance, sink.pairRows[pairRenderF2FS], sink.pairLaneRows[pairRenderF2FS],
			registry, census, sink.legacyPairProof)
	}
	if lane.structured {
		if provenance.Flags != profilerPairRowProvenanceStructured ||
			sink.structuredPairRows[pairRenderF2FS] != 1 ||
			sink.structuredLaneRows[pairRenderF2FS][laneKey] != 1 ||
			sink.structuredEventRows[pairRenderF2FS][4011] != 1 {
			t.Fatalf("structured pair prefix counters drifted: row=%+v structured=%d lanes=%v events=%v",
				row, sink.structuredPairRows[pairRenderF2FS], sink.structuredLaneRows[pairRenderF2FS],
				sink.structuredEventRows[pairRenderF2FS])
		}
	} else if provenance.Flags&profilerPairRowProvenanceStructured != 0 ||
		sink.structuredPairRows[pairRenderF2FS] != 0 ||
		len(sink.structuredEventRows[pairRenderF2FS]) != 0 {
		t.Fatalf("%s text prefix entered structured accounting: row=%+v totals=%d events=%v",
			lane.name, row, sink.structuredPairRows[pairRenderF2FS], sink.structuredEventRows[pairRenderF2FS])
	}
}

func TestProfilerSequenceRejectsInvalidBoundsBeforeCurrentRecordMutation(t *testing.T) {
	for _, lane := range profilerSequenceAuthorityCases(t) {
		for _, pairFirst := range []bool{false, true} {
			order := "ordinary-first"
			if pairFirst {
				order = "pair-first"
			}
			for _, start := range []int{-1, math.MaxInt} {
				t.Run(lane.name+"/"+order+"/"+sequenceBoundName(start), func(t *testing.T) {
					sink, err := newTraceDBRowSink(t.TempDir(), 128)
					if err != nil {
						t.Fatal(err)
					}
					defer sink.cleanup()
					defer sink.abortPairRowCensus()
					beginProfilerSequencePublisher(t, sink, lane)
					seq := start
					result := lane.run(context.Background(), pairFirst, &seq, sink)
					requireProfilerSequenceInvariant(t, result.err, "profiler_row_sequence_invalid")
					if result.rows != 0 || seq != start {
						t.Fatalf("%s start=%d returned rows=%d seq=%d err=%v", lane.name, start, result.rows, seq, result.err)
					}
					assertProfilerSequenceNoRowMutation(t, sink, lane)
				})
			}
		}
	}
}

func sequenceBoundName(value int) string {
	if value < 0 {
		return "negative"
	}
	return "max-int"
}

func TestProfilerSequenceRangeFailureIsWholeStageOrCompletedPrefixAtomic(t *testing.T) {
	for _, lane := range profilerSequenceAuthorityCases(t) {
		for _, pairFirst := range []bool{false, true} {
			order := "ordinary-first"
			if pairFirst {
				order = "pair-first"
			}
			t.Run(lane.name+"/"+order, func(t *testing.T) {
				sink, err := newTraceDBRowSink(t.TempDir(), 128)
				if err != nil {
					t.Fatal(err)
				}
				defer sink.cleanup()
				defer sink.abortPairRowCensus()
				beginProfilerSequencePublisher(t, sink, lane)
				seq := math.MaxInt - 1
				result := lane.run(context.Background(), pairFirst, &seq, sink)
				requireProfilerSequenceInvariant(t, result.err, "profiler_row_sequence_invalid")
				if lane.wholeStageAtomic {
					if result.rows != 0 || seq != math.MaxInt-1 {
						t.Fatalf("strict whole-stage range failure published a prefix: rows=%d seq=%d err=%v",
							result.rows, seq, result.err)
					}
					assertProfilerSequenceNoRowMutation(t, sink, lane)
					return
				}
				if result.rows != 1 || seq != math.MaxInt {
					t.Fatalf("%s completed-prefix result drifted: rows=%d seq=%d err=%v",
						lane.name, result.rows, seq, result.err)
				}
				assertProfilerSequencePrefix(t, sink, lane, pairFirst)
			})
		}
	}
}

func TestProfilerStrictSequenceRangePrecedesWholeKindPrePoison(t *testing.T) {
	_, rejectedMMC, _ := profilerStrictTextMMCFixture(t)
	ordinary := "worker-7 (7) [001] .... 0.999000: print: B|7|Frame"
	stage, err := stageProfilerStrictSystracePayloadContext(
		context.Background(), []byte(rejectedMMC+"\n"+ordinary+"\n"))
	if err != nil || stage.scan.rows != 2 || !stage.scan.wholeKindPoison[pairRenderMMC] {
		t.Fatalf("strict sequence fixture lacks real MMC whole-kind pre-poison: scan=%+v err=%v",
			stage.scan, err)
	}

	lane := profilerSequenceAuthorityCase{
		name: "strict-whole-kind", publisher: profilerPairPublisherExactFtrace, text: true,
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	defer sink.abortPairRowCensus()
	beginProfilerSequencePublisher(t, sink, lane)
	seq := math.MaxInt - 1
	rows, classified, err := addProfilerStrictSystraceStageContext(
		context.Background(), stage, &seq, sink)
	requireProfilerSequenceInvariant(t, err, "profiler_row_sequence_invalid")
	if !classified || rows != 0 || seq != math.MaxInt-1 {
		t.Fatalf("strict whole-kind range result drifted: classified=%t rows=%d seq=%d err=%v",
			classified, rows, seq, err)
	}
	registry := sink.pairLaneRegistries[pairRenderMMC]
	census := sink.activePairCensus[pairRenderMMC]
	if sink.poisoned[pairRenderMMC] || len(sink.poisonedLanes[pairRenderMMC]) != 0 ||
		sink.opaque[pairRenderMMC] || sink.pairRows[pairRenderMMC] != 0 ||
		len(sink.pairLaneRows[pairRenderMMC]) != 0 || len(registry.byKey) != 0 ||
		len(registry.keys) != 0 || len(registry.states) != 0 || census.total != 0 ||
		len(census.byLane) != 0 || sink.legacyPairProof.observations != 0 ||
		sink.legacyPairProof.laneKeys != 0 || sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 ||
		sink.activeTextRows != 0 || sink.nextTextMessage != 0 {
		t.Fatalf("sequence range failure leaked MMC pre-poison or row state: poisoned=%t lanes=%v opaque=%t pair=%d lane_rows=%v registry=%+v census=%+v proof=%+v stats=%+v rows=%d text=%d next=%d",
			sink.poisoned[pairRenderMMC], sink.poisonedLanes[pairRenderMMC], sink.opaque[pairRenderMMC],
			sink.pairRows[pairRenderMMC], sink.pairLaneRows[pairRenderMMC], registry, census,
			sink.legacyPairProof, sink.stats, len(sink.rows), sink.activeTextRows, sink.nextTextMessage)
	}
}

func TestProfilerSequenceNilInputsFailWithoutPanic(t *testing.T) {
	for _, lane := range profilerSequenceAuthorityCases(t) {
		t.Run(lane.name+"/nil-sequence", func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			defer sink.abortPairRowCensus()
			beginProfilerSequencePublisher(t, sink, lane)
			result := lane.run(context.Background(), true, nil, sink)
			requireProfilerSequenceInvariant(t, result.err, "profiler_row_sequence_missing")
			if result.rows != 0 {
				t.Fatalf("%s nil sequence rows=%d err=%v", lane.name, result.rows, result.err)
			}
			assertProfilerSequenceNoRowMutation(t, sink, lane)
		})

		t.Run(lane.name+"/nil-sink", func(t *testing.T) {
			seq := 0
			result := lane.run(context.Background(), true, &seq, nil)
			if result.err == nil || result.rows != 0 || seq != 0 {
				t.Fatalf("%s nil sink did not fail without mutation: rows=%d seq=%d err=%T %v",
					lane.name, result.rows, seq, result.err, result.err)
			}
		})
	}
}

func TestProfilerSequencedAuthorityRejectsPreassignedRowBeforeDelta(t *testing.T) {
	lane := profilerSequenceAuthorityCase{
		name: "sequenced-helper", publisher: profilerPairPublisherBytrace, text: true,
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	defer sink.abortPairRowCensus()
	beginProfilerSequencePublisher(t, sink, lane)
	seq := 7
	row := renderedRow{tsNS: 1, seq: 99, line: "ordinary"}
	var delta traceDBProfilerEventDelta
	delta.poisonKind(pairRenderF2FS)
	err = sink.addSequencedProfilerEventContext(context.Background(), &seq, row, delta)
	requireProfilerSequenceInvariant(t, err, "profiler_row_sequence_preassigned")
	if seq != 7 {
		t.Fatalf("preassigned row advanced sequence: %d", seq)
	}
	assertProfilerSequenceNoRowMutation(t, sink, lane)
}

func TestProfilerSessionRowCounterAuthorityBoundsAndParity(t *testing.T) {
	t.Run("rows-read-overflow", func(t *testing.T) {
		next, err := nextProfilerSessionRowsRead(math.MaxInt)
		requireProfilerSequenceInvariant(t, err, "profiler_session_rows_read_overflow")
		if next != 0 {
			t.Fatalf("overflow returned next rows-read=%d", next)
		}
	})

	for _, test := range []struct {
		name     string
		coverage TraceDBCoverage
		out      profilerContainerExtraction
		seq      int
	}{
		{name: "negative-sequence", coverage: TraceDBCoverage{}, out: profilerContainerExtraction{}, seq: -1},
		{name: "emitted-sequence-drift", coverage: TraceDBCoverage{RowsRead: 4, RowsEmitted: 2}, out: profilerContainerExtraction{TextRows: 3}, seq: 3},
		{name: "text-sequence-drift", coverage: TraceDBCoverage{RowsRead: 4, RowsEmitted: 3}, out: profilerContainerExtraction{TextRows: 2}, seq: 3},
		{name: "read-below-emitted", coverage: TraceDBCoverage{RowsRead: 2, RowsEmitted: 3}, out: profilerContainerExtraction{TextRows: 3}, seq: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			beforeCoverage := test.coverage
			beforeText := test.out.TextRows
			err := validateProfilerSessionRowCounterState(&test.coverage, &test.out, test.seq)
			requireProfilerSequenceInvariant(t, err, "profiler_session_row_counter_state_invalid")
			if test.coverage.RowsRead != beforeCoverage.RowsRead ||
				test.coverage.RowsEmitted != beforeCoverage.RowsEmitted || test.out.TextRows != beforeText {
				t.Fatalf("parity validation mutated counters: coverage=%+v want=%+v text=%d want=%d",
					test.coverage, beforeCoverage, test.out.TextRows, beforeText)
			}
		})
	}

	for _, lineRows := range []int{0, 1} {
		name := "no-row"
		if lineRows == 1 {
			name = "one-row"
		}
		t.Run("commit-"+name, func(t *testing.T) {
			coverage := TraceDBCoverage{RowsRead: 7, RowsEmitted: 3}
			out := profilerContainerExtraction{TextRows: 3}
			nextSeq := 3 + lineRows
			if err := commitProfilerSessionRowCounters(&coverage, &out, 3, nextSeq, lineRows, 8); err != nil {
				t.Fatal(err)
			}
			if coverage.RowsRead != 8 || coverage.RowsEmitted != nextSeq || out.TextRows != nextSeq {
				t.Fatalf("committed counters drifted: coverage=%+v out=%+v", coverage, out)
			}
		})
	}

	t.Run("invalid-commit-is-atomic", func(t *testing.T) {
		coverage := TraceDBCoverage{RowsRead: 7, RowsEmitted: 3}
		out := profilerContainerExtraction{TextRows: 3}
		beforeCoverage := coverage
		beforeText := out.TextRows
		err := commitProfilerSessionRowCounters(&coverage, &out, 3, 5, 1, 8)
		requireProfilerSequenceInvariant(t, err, "profiler_session_row_counter_commit_invalid")
		if coverage.RowsRead != beforeCoverage.RowsRead || coverage.RowsEmitted != beforeCoverage.RowsEmitted ||
			out.TextRows != beforeText {
			t.Fatalf("invalid commit mutated counters: coverage=%+v want=%+v text=%d want=%d",
				coverage, beforeCoverage, out.TextRows, beforeText)
		}
	})
}
