package hitraceconv

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func profilerBlockTestStructuredMessage(field int, payload []byte, ts, pid uint64) []byte {
	detail := protoPayload(
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(ts, pid, pid, "block", field, payload),
	)
	return protoMessage(2, detail)
}

func profilerBlockMalformedEventMessage(field, wire int, trailing []byte) []byte {
	event := protoPayload(
		protoVarint(1, 2_000), protoVarint(2, 40), protoBytes(3, []byte("block")),
		protoMessage(50, protoVarint(4, 40)), profilerBlockTypedRawKey(field, wire), trailing,
	)
	return protoMessage(2, protoPayload(protoVarint(1, 2), protoMessage(2, event)))
}

func writeProfilerBlockSinkForStats(t *testing.T, sink *traceDBRowSink) (string, traceDBRowSortStats) {
	t.Helper()
	var out bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	return out.String(), stats
}

func TestStructuredBlockExactLaneBarrierKeepsUnrelatedLaneAcrossSpill(t *testing.T) {
	t.Parallel()
	laneAStart := profilerBlockTypedPayload(211, nil)
	laneADone := profilerBlockTypedPayload(209, nil)
	laneBStart := profilerBlockTypedPayload(211, map[int]profilerBlockTypedValue{2: {wire: 0, u64: 20}})
	laneBDone := profilerBlockTypedPayload(209, map[int]profilerBlockTypedValue{2: {wire: 0, u64: 20}})
	badLaneA := profilerBlockTypedPayload(209, map[int]profilerBlockTypedValue{4: {wire: 0, u64: math.MaxInt32 + 1}})

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	for _, message := range [][]byte{
		profilerBlockTestStructuredMessage(211, laneAStart, 1_000, 40),
		profilerBlockTestStructuredMessage(211, laneBStart, 2_000, 0),
		profilerBlockTestStructuredMessage(209, badLaneA, 3_000, 41),
		profilerBlockTestStructuredMessage(209, laneADone, 4_000, 41),
		profilerBlockTestStructuredMessage(209, laneBDone, 5_000, 42),
	} {
		if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, sink); renderErr != nil {
			t.Fatal(renderErr)
		}
	}
	if sink.poisoned[pairRenderBlock] || len(profilerTestPoisonedLanes(sink)[pairRenderBlock]) != 1 ||
		sink.withheldPairRowsForKind(pairRenderBlock) != 2 || sink.publishableRows() != 2 || len(sink.runs) == 0 {
		t.Fatalf("Block exact-lane barrier drifted: accepted=%d withheld=%d publishable=%d family=%v lanes=%v chunks=%d",
			sink.stats.RowsAccepted, sink.withheldPairRowsForKind(pairRenderBlock), sink.publishableRows(),
			sink.poisoned, profilerTestPoisonedLanes(sink), len(sink.runs))
	}
	text, stats := writeProfilerBlockSinkForStats(t, sink)
	if stats.RowsAccepted != 4 || stats.RowsWritten != 2 || stats.RowsWithheld != 2 ||
		strings.Contains(text, " 2 + 3 ") || !strings.Contains(text, " 20 + 3 ") {
		t.Fatalf("Block exact-lane publication mismatch: stats=%+v\n%s", stats, text)
	}

	path := filepath.Join(t.TempDir(), "block-barrier.ftrace")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	window := tracequery.ComputeWindowStats(index, tracequery.Query{})
	if len(window.IOLatencies) != 1 {
		t.Fatalf("tracequery E2E expected one healthy Block duration, got %+v caveats=%+v", window.IOLatencies, window.Caveats)
	}
}

func TestStructuredBlockUnknownKeyClosesOnlyBlockFamily(t *testing.T) {
	t.Parallel()
	blockStart := profilerBlockTypedPayload(211, nil)
	blockDone := profilerBlockTypedPayload(209, nil)
	missingDev := profilerBlockTypedPayload(209, nil, 1)
	aux := profilerAuxCasesByField()

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	for _, message := range [][]byte{
		profilerBlockTestStructuredMessage(211, blockStart, 1_000, 40),
		profilerBlockTestStructuredMessage(209, missingDev, 2_000, 41),
		profilerBlockTestStructuredMessage(209, blockDone, 3_000, 41),
		profilerMMCTestStructuredMessage(4016, aux[4016].values, 4_000),
		profilerMMCTestStructuredMessage(4015, aux[4015].values, 5_000),
	} {
		if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, sink); renderErr != nil {
			t.Fatal(renderErr)
		}
	}
	if !sink.poisoned[pairRenderBlock] || sink.poisoned[pairRenderMMC] ||
		sink.withheldPairRowsForKind(pairRenderBlock) != 3 || sink.publishableRows() != 2 {
		t.Fatalf("unknown Block key contaminated sibling family: accepted=%d withheld=%d publishable=%d poisoned=%v",
			sink.stats.RowsAccepted, sink.withheldPairRowsForKind(pairRenderBlock), sink.publishableRows(), sink.poisoned)
	}
	text, stats := writeProfilerBlockSinkForStats(t, sink)
	if stats.RowsAccepted != 5 || stats.RowsWritten != 2 || stats.RowsWithheld != 3 ||
		strings.Contains(text, "block_rq_") || !strings.Contains(text, "mmc_request_start:") ||
		!strings.Contains(text, "mmc_request_done:") {
		t.Fatalf("Block family filtering mismatch: stats=%+v\n%s", stats, text)
	}
}

func TestStructuredBlockSemanticRejectQuarantinesOnlyExactLane(t *testing.T) {
	t.Parallel()
	rejectedStart := profilerBlockTypedPayload(211, map[int]profilerBlockTypedValue{3: {wire: 0, u64: 0}})
	rejectedDone := profilerBlockTypedPayload(209, map[int]profilerBlockTypedValue{3: {wire: 0, u64: 0}})
	healthyStart := profilerBlockTypedPayload(211, map[int]profilerBlockTypedValue{2: {wire: 0, u64: 20}})
	healthyDone := profilerBlockTypedPayload(209, map[int]profilerBlockTypedValue{2: {wire: 0, u64: 20}})

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	for _, message := range [][]byte{
		profilerBlockTestStructuredMessage(211, rejectedStart, 1_000, 40),
		profilerBlockTestStructuredMessage(211, healthyStart, 2_000, 40),
		profilerBlockTestStructuredMessage(209, rejectedDone, 3_000, 41),
		profilerBlockTestStructuredMessage(209, healthyDone, 4_000, 41),
	} {
		if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, sink); renderErr != nil {
			t.Fatal(renderErr)
		}
	}
	if sink.poisoned[pairRenderBlock] || len(profilerTestPoisonedLanes(sink)[pairRenderBlock]) != 1 ||
		sink.withheldPairRowsForKind(pairRenderBlock) != 2 || sink.publishableRows() != 2 {
		t.Fatalf("semantic Block rejection widened beyond exact lane: family=%v lanes=%v withheld=%d publishable=%d",
			sink.poisoned, profilerTestPoisonedLanes(sink), sink.withheldPairRowsForKind(pairRenderBlock), sink.publishableRows())
	}
}

func TestStructuredBlockMalformedOneofKeyRetainsOnlyExactFamilyProvenance(t *testing.T) {
	t.Parallel()
	for _, endpointField := range []int{202, 204, 209, 211} {
		for _, malformed := range []struct {
			name     string
			wire     int
			trailing []byte
		}{
			{name: "unsupported_wire", wire: 3},
			{name: "truncated_length", wire: 2, trailing: []byte{0x01}},
		} {
			t.Run(profilerFtraceEventDescriptors[endpointField].Name+"/"+malformed.name, func(t *testing.T) {
				message := profilerBlockMalformedEventMessage(endpointField, malformed.wire, malformed.trailing)
				result := decodeProfilerTracePluginResult(message)
				events, err := profilerTracePluginResultEvents(result)
				if err != nil || len(events) != 1 || events[0].Field != 0 ||
					events[0].PairFamilies&pairCriticalFormatFamilyBlock == 0 || !events[0].PairCaptureOpaque {
					t.Fatalf("malformed exact Block oneof lost provenance: field=%d events=%+v err=%v", endpointField, events, err)
				}
			})
		}
	}
	for _, inventoryField := range []int{205, 210, 212} {
		message := profilerBlockMalformedEventMessage(inventoryField, 3, nil)
		result := decodeProfilerTracePluginResult(message)
		events, err := profilerTracePluginResultEvents(result)
		if err != nil || len(events) != 1 || events[0].PairFamilies&pairCriticalFormatFamilyBlock != 0 {
			t.Fatalf("inventory malformed oneof guessed Block family: field=%d events=%+v err=%v", inventoryField, events, err)
		}
	}
}

func TestStructuredBlockMalformedOneofCannotBridgeHealthyEndpoints(t *testing.T) {
	t.Parallel()
	start := profilerBlockTestStructuredMessage(211, profilerBlockTypedPayload(211, nil), 1_000, 40)
	done := profilerBlockTestStructuredMessage(209, profilerBlockTypedPayload(209, nil), 3_000, 41)
	for _, hole := range [][]byte{
		profilerBlockMalformedEventMessage(209, 3, nil),
		profilerBlockMalformedEventMessage(209, 2, []byte{0x01}),
	} {
		sink, err := newTraceDBRowSink(t.TempDir(), 1)
		if err != nil {
			t.Fatal(err)
		}
		seq := 0
		for _, message := range [][]byte{start, hole, done} {
			if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, sink); renderErr != nil {
				sink.cleanup()
				t.Fatal(renderErr)
			}
		}
		if !sink.poisoned[pairRenderBlock] || sink.withheldPairRowsForKind(pairRenderBlock) != 2 ||
			sink.publishableRows() != 0 {
			sink.cleanup()
			t.Fatalf("malformed Block oneof bridged endpoints: accepted=%d withheld=%d opaque=%v poisoned=%v",
				sink.stats.RowsAccepted, sink.withheldPairRowsForKind(pairRenderBlock), sink.opaque, sink.poisoned)
		}
		sink.cleanup()
	}
}

func TestStructuredAndStrictTextBlockShareOneBarrier(t *testing.T) {
	t.Parallel()
	start := profilerBlockTestStructuredMessage(211, profilerBlockTypedPayload(211, nil), 1_000, 40)
	done := profilerBlockTestStructuredMessage(209, profilerBlockTypedPayload(209, nil), 3_000, 41)
	badText := []byte("worker-88 [002] d..2 0.000002: block_rq_issue: 0,1 R 4294967296 () 2 + 3 []\n")
	printText := []byte("worker-7 [002] d..2 0.000004: print: B|7|Frame\n")

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	if _, _, err := renderProfilerFtraceStructuredRows(start, &seq, sink); err != nil {
		t.Fatal(err)
	}
	if _, handled, err := addStrictSystraceRowsFromBytes(badText, &seq, sink); err != nil || !handled {
		t.Fatalf("strict Block hole not handled: handled=%t err=%v", handled, err)
	}
	if _, _, err := renderProfilerFtraceStructuredRows(done, &seq, sink); err != nil {
		t.Fatal(err)
	}
	if _, handled, err := addStrictSystraceRowsFromBytes(printText, &seq, sink); err != nil || !handled {
		t.Fatalf("strict print sibling not handled: handled=%t err=%v", handled, err)
	}
	if sink.poisoned[pairRenderBlock] || len(profilerTestPoisonedLanes(sink)[pairRenderBlock]) != 1 ||
		sink.withheldPairRowsForKind(pairRenderBlock) != 3 || sink.publishableRows() != 1 {
		t.Fatalf("structured/text source barrier split: accepted=%d withheld=%d publishable=%d family=%v lanes=%v",
			sink.stats.RowsAccepted, sink.withheldPairRowsForKind(pairRenderBlock), sink.publishableRows(),
			sink.poisoned, profilerTestPoisonedLanes(sink))
	}
	text, stats := writeProfilerBlockSinkForStats(t, sink)
	if stats.RowsWritten != 1 || stats.RowsWithheld != 3 || strings.Contains(text, "block_rq_") ||
		!strings.Contains(text, "print: B|7|Frame") {
		t.Fatalf("mixed structured/text barrier output mismatch: stats=%+v\n%s", stats, text)
	}
}

func TestProfilerTextDelimiterDriftCannotMintDownstreamBlockDuration(t *testing.T) {
	t.Parallel()
	canonicalStart := traceDBFormatLine("worker", 40, 40, 2, 5_000_000_000, 0, 0,
		"block_rq_issue: 0,1 R 4 () 2 + 3 []")
	canonicalDone := traceDBFormatLine("worker", 41, 41, 2, 5_001_000_000, 0, 0,
		"block_rq_complete: 0,1 R () 2 + 3 [0]")
	healthyStart := traceDBFormatLine("worker", 42, 42, 2, 5_002_000_000, 0, 0,
		"block_rq_issue: 0,1 R 4 () 10 + 3 []")
	healthyDone := traceDBFormatLine("worker", 43, 43, 2, 5_003_000_000, 0, 0,
		"block_rq_complete: 0,1 R () 10 + 3 [0]")
	print := traceDBFormatLine("worker", 7, 7, 2, 5_004_000_000, 0, 0, "print: B|7|Frame")

	for _, drift := range []struct {
		name string
		row  string
	}{
		{name: "missing_colon", row: strings.Replace(canonicalStart, "block_rq_issue:", "block_rq_issue", 1)},
		{name: "space_before_colon", row: strings.Replace(canonicalStart, "block_rq_issue:", "block_rq_issue :", 1)},
	} {
		for _, scalar := range []struct {
			name   string
			mutate func(string) string
		}{
			{name: "valid", mutate: func(line string) string { return line }},
			{name: "malformed_pid", mutate: func(line string) string {
				return strings.Replace(line, "worker-40", "worker-bad", 1)
			}},
			{name: "malformed_cpu", mutate: func(line string) string {
				return strings.Replace(line, "[002]", "[bad]", 1)
			}},
			{name: "malformed_timestamp", mutate: func(line string) string {
				return strings.Replace(line, "5.000000:", "NaN:", 1)
			}},
		} {
			rows := []string{scalar.mutate(drift.row), canonicalDone, healthyStart, healthyDone, print}
			for _, source := range []struct {
				name  string
				input func([]string) []byte
			}{
				{name: "binary_bytrace", input: func(lines []string) []byte {
					messages := make([][]byte, 0, len(lines))
					for _, line := range lines {
						messages = append(messages, syntheticProfilerPluginData("bytrace_plugin", []byte(line+"\n")))
					}
					return syntheticProfilerTraceFile(messages...)
				}},
				{name: "session", input: func(lines []string) []byte {
					return []byte(profilerSessionJSONTag + "\n" + strings.Join(lines, "\n") + "\n")
				}},
			} {
				t.Run(drift.name+"/"+scalar.name+"/"+source.name, func(t *testing.T) {
					input := source.input(rows)
					dir := t.TempDir()
					inputPath := filepath.Join(dir, "input.trace")
					outputPath := filepath.Join(dir, "output.ftrace")
					if err := os.WriteFile(inputPath, input, 0o600); err != nil {
						t.Fatal(err)
					}
					result, handled, err := tryConvertProfilerContainer(context.Background(), Options{InputPath: inputPath},
						int64(len(input)), outputPath, nil, nil, nil, nil, nil)
					if err != nil || !handled || result.EventsWritten != 4 {
						t.Fatalf("delimiter-drift conversion: handled=%t result=%+v err=%v", handled, result, err)
					}
					body, err := os.ReadFile(outputPath)
					if err != nil {
						t.Fatal(err)
					}
					text := string(body)
					if strings.Contains(text, "block_rq_issue 0,1 R 4 () 2 + 3") ||
						strings.Contains(text, "block_rq_issue : 0,1 R 4 () 2 + 3") ||
						!strings.Contains(text, "block_rq_complete: 0,1 R () 2 + 3 [0]") ||
						!strings.Contains(text, "block_rq_issue: 0,1 R 4 () 10 + 3 []") ||
						!strings.Contains(text, "block_rq_complete: 0,1 R () 10 + 3 [0]") {
						t.Fatalf("delimiter drift was normalized or canonical sibling lost:\n%s", text)
					}
					index, err := tracequery.BuildIndex(context.Background(), outputPath)
					if err != nil {
						t.Fatal(err)
					}
					stats := tracequery.ComputeWindowStats(index, tracequery.Query{})
					if len(stats.IOLatencies) != 1 {
						t.Fatalf("delimiter drift minted a Block duration: latencies=%+v caveats=%+v", stats.IOLatencies, stats.Caveats)
					}
					for _, coverage := range result.TraceCoverage {
						if coverage.Family == "builtin_modern_ftrace:block" && coverage.Table == "__complete_capture_barrier__" {
							t.Fatalf("near-name delimiter drift poisoned Block family: %+v", coverage)
						}
					}
				})
			}
		}
	}
}

func TestProfilerCommentLookingBlockEndpointsNeverEnterPairAuthority(t *testing.T) {
	start := traceDBFormatLine("worker", 40, 40, 2, 5_000_000_000, 0, 0,
		"block_rq_issue: 0,1 R 4 () 2 + 3 []")
	done := traceDBFormatLine("worker", 41, 41, 2, 5_001_000_000, 0, 0,
		"block_rq_complete: 0,1 R () 2 + 3 [0]")
	keep := traceDBFormatLine("worker", 7, 7, 2, 5_002_000_000, 0, 0,
		"print: B|7|CommentGate")
	commentStart := "#" + strings.TrimLeft(start, " ")
	commentDone := "#" + strings.TrimLeft(done, " ")

	// Strict text elects origin from the first real physical row. A later
	// exact-looking comment and a malformed sibling reject the payload, but the
	// comment must never become Block provenance for delayed opacity/poison.
	strictRejected := []byte(keep + "\n" + commentStart + "\nnot a physical trace row\n")
	scan := scanProfilerStrictSystracePayload(strictRejected, nil)
	if !scan.originText || !scan.rejected || scan.observed[pairRenderBlock] ||
		profilerPayloadContainsExactBlockEndpoint(strictRejected) {
		t.Fatalf("strict comment entered Block provenance: scan=%+v", scan)
	}
	for _, prefix := range []string{"", "   "} {
		payload := []byte(keep + "\n" + prefix + commentStart + "\n")
		scan := scanProfilerStrictSystracePayload(payload, nil)
		if !scan.originText || scan.rejected || scan.observed[pairRenderBlock] ||
			profilerPayloadContainsExactBlockEndpoint(payload) {
			t.Fatalf("valid strict comment prefix %q entered Block provenance: scan=%+v", prefix, scan)
		}
	}
	tabComment := []byte("\t" + commentStart + "\n" + keep + "\n")
	tabScan := scanProfilerStrictSystracePayload(tabComment, nil)
	if !tabScan.rejected || tabScan.observed[pairRenderBlock] ||
		profilerPayloadContainsExactBlockEndpoint(tabComment) {
		t.Fatalf("control-prefixed strict comment escaped rejection or minted provenance: scan=%+v", tabScan)
	}
	oversizedTabComment := []byte("\t" + commentStart + strings.Repeat("x", maxProfilerTextLineBytes) + "\n" + keep + "\n")
	oversizedTabScan := scanProfilerStrictSystracePayload(oversizedTabComment, nil)
	if !oversizedTabScan.rejected || oversizedTabScan.observed[pairRenderBlock] ||
		profilerPayloadContainsExactBlockEndpoint(oversizedTabComment) {
		t.Fatalf("oversized control-prefixed comment escaped rejection or minted provenance: scan=%+v", oversizedTabScan)
	}
	malformedExactRows := []struct {
		name string
		data []byte
	}{
		{
			name: "short exact endpoint with trailing control",
			data: []byte(start + "\x01\n"),
		},
		{
			name: "oversized exact endpoint with trailing control",
			data: []byte(start + strings.Repeat("x", maxProfilerTextLineBytes) + "\x01\n"),
		},
	}
	for _, malformed := range malformedExactRows {
		t.Run("strict-malformed-endpoint/"+malformed.name, func(t *testing.T) {
			scan := scanProfilerStrictSystracePayload(malformed.data, nil)
			if !scan.originText || !scan.rejected || !scan.observed[pairRenderBlock] ||
				!profilerPayloadContainsExactBlockEndpoint(malformed.data) {
				t.Fatalf("malformed non-comment exact endpoint became an invisible hole: scan=%+v", scan)
			}
		})
	}
	for _, payload := range []struct {
		name string
		data []byte
	}{
		{
			name: "oversized exact-looking comment",
			data: []byte(commentStart + strings.Repeat("x", maxProfilerTextLineBytes) + "\n" + keep + "\n"),
		},
		{
			name: "invalid-control exact-looking comment",
			data: []byte(commentStart + "\x01\n" + keep + "\n"),
		},
	} {
		t.Run("strict-negative/"+payload.name, func(t *testing.T) {
			scan := scanProfilerStrictSystracePayload(payload.data, nil)
			if !scan.rejected || scan.observed[pairRenderBlock] ||
				profilerPayloadContainsExactBlockEndpoint(payload.data) {
				t.Fatalf("invalid comment minted Block provenance: scan=%+v", scan)
			}
		})
	}

	// Exact strict, noncanonical and rejected Profiler frames all consult the
	// same provenance probe. Put each comment-only hole between a real start and
	// completion: the real pair must survive and no Block barrier may appear.
	for _, middle := range []struct {
		name  string
		frame []byte
	}{
		{
			name:  "strict rejected payload",
			frame: syntheticProfilerPluginData("ftrace-plugin", strictRejected),
		},
		{
			name:  "strict oversized tab comment",
			frame: syntheticProfilerPluginData("ftrace-plugin", oversizedTabComment),
		},
		{
			name:  "noncanonical ftrace payload",
			frame: syntheticProfilerPluginData("FTRACE-PLUGIN", []byte(commentStart+"\n")),
		},
		{
			name:  "rejected profiler frame",
			frame: profilerRejectedFrameWithData([]byte(commentStart + "\n")),
		},
	} {
		t.Run("frame-provenance/"+middle.name, func(t *testing.T) {
			body := syntheticProfilerTraceFile(
				syntheticProfilerPluginData("bytrace_plugin", []byte(start+"\n")),
				middle.frame,
				syntheticProfilerPluginData("bytrace_plugin", []byte(done+"\n")),
				syntheticProfilerPluginData("bytrace_plugin", []byte(keep+"\n")),
			)
			assertProfilerCommentBlockResult(t, body, 3, 1)
		})
	}

	for _, malformed := range malformedExactRows {
		t.Run("anti-rescue/"+malformed.name, func(t *testing.T) {
			body := syntheticProfilerTraceFile(
				syntheticProfilerPluginData("bytrace_plugin", []byte(start+"\n")),
				syntheticProfilerPluginData("ftrace-plugin", malformed.data),
				syntheticProfilerPluginData("bytrace_plugin", []byte(done+"\n")),
				syntheticProfilerPluginData("bytrace_plugin", []byte(keep+"\n")),
			)
			assertProfilerMalformedBlockHoleResult(t, body)
		})
	}

	// Session and bytrace share addSystraceRowsFromBytes. Both comment rows
	// would form a complete fake duration if the leading-# negative gate moved
	// below ParseLine/admission.
	commentPayload := "  " + commentStart + "\n\t" + commentDone + "\n" + keep + "\n"
	for _, source := range []struct {
		name string
		body []byte
	}{
		{
			name: "binary bytrace",
			body: syntheticProfilerTraceFile(
				syntheticProfilerPluginData("bytrace_plugin", []byte(commentPayload)),
			),
		},
		{
			name: "binary bytrace oversized comment",
			body: syntheticProfilerTraceFile(
				syntheticProfilerPluginData("bytrace_plugin", []byte(
					commentStart+strings.Repeat("x", maxProfilerTextLineBytes)+"\n"+keep+"\n")),
			),
		},
		{
			name: "SessionJSON",
			body: []byte(profilerSessionJSONTag + "\n" + commentPayload),
		},
	} {
		t.Run("general-text/"+source.name, func(t *testing.T) {
			assertProfilerCommentBlockResult(t, source.body, 1, 0)
		})
	}
}

func assertProfilerMalformedBlockHoleResult(t *testing.T, input []byte) {
	t.Helper()
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.trace")
	outputPath := filepath.Join(dir, "output.ftrace")
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal(err)
	}
	result, handled, err := tryConvertProfilerContainer(context.Background(), Options{InputPath: inputPath},
		int64(len(input)), outputPath, nil, nil, nil, nil, nil)
	if err != nil || !handled || result.EventsWritten != 1 {
		t.Fatalf("malformed endpoint anti-rescue conversion: handled=%t result=%+v err=%v", handled, result, err)
	}
	published, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(published), "block_rq_") || !strings.Contains(string(published), "print: B|7|CommentGate") {
		t.Fatalf("malformed endpoint hole published a Block sibling or lost control row:\n%s", published)
	}
	barriers := 0
	for _, coverage := range result.TraceCoverage {
		if coverage.Family == "builtin_modern_ftrace:block" && coverage.Table == "__complete_capture_barrier__" {
			barriers++
			if coverage.RowsRead != 2 || coverage.RowsEmitted != 0 {
				t.Fatalf("malformed endpoint barrier accounting drifted: %+v", coverage)
			}
		}
	}
	if barriers != 1 {
		t.Fatalf("malformed endpoint did not close exactly one Block barrier: coverage=%+v", result.TraceCoverage)
	}
	index, err := tracequery.BuildIndex(context.Background(), outputPath)
	if err != nil {
		t.Fatal(err)
	}
	stats := tracequery.ComputeWindowStats(index, tracequery.Query{})
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("malformed exact endpoint allowed cross-hole rescue: %+v", stats.IOLatencies)
	}
}

func assertProfilerCommentBlockResult(t *testing.T, input []byte, wantEvents, wantLatencies int) {
	t.Helper()
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.trace")
	outputPath := filepath.Join(dir, "output.ftrace")
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal(err)
	}
	result, handled, err := tryConvertProfilerContainer(context.Background(), Options{InputPath: inputPath},
		int64(len(input)), outputPath, nil, nil, nil, nil, nil)
	if err != nil || !handled || result.EventsWritten != wantEvents {
		t.Fatalf("comment-gate conversion: handled=%t result=%+v want_events=%d err=%v",
			handled, result, wantEvents, err)
	}
	published, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(published), "#worker") {
		t.Fatalf("comment-looking endpoint was published:\n%s", published)
	}
	for _, coverage := range result.TraceCoverage {
		if coverage.Family == "builtin_modern_ftrace:block" && coverage.Table == "__complete_capture_barrier__" {
			t.Fatalf("comment-looking endpoint poisoned Block authority: %+v", coverage)
		}
	}
	index, err := tracequery.BuildIndex(context.Background(), outputPath)
	if err != nil {
		t.Fatal(err)
	}
	stats := tracequery.ComputeWindowStats(index, tracequery.Query{})
	if len(stats.IOLatencies) != wantLatencies {
		t.Fatalf("comment-looking endpoints changed Block durations: got=%+v want=%d caveats=%+v",
			stats.IOLatencies, wantLatencies, stats.Caveats)
	}
}

func TestProfilerBlockTextOwnerIsProvenButNotPartOfRequestLane(t *testing.T) {
	t.Parallel()
	idleStart := traceDBFormatLine("idle", 0, 0, 2, 5_000_000_000, 0, 0,
		"block_rq_issue: 0,1 R 4 () 20 + 3 []")
	positiveDone := traceDBFormatLine("worker", 41, 41, 2, 5_001_000_000, 0, 0,
		"block_rq_complete: 0,1 R () 20 + 3 [0]")
	startPair := profilerTextPairAdmission(idleStart)
	donePair := profilerTextPairAdmission(positiveDone)
	if !startPair.Admitted || !donePair.Admitted || !startPair.HeaderOwnerKnown || !donePair.HeaderOwnerKnown ||
		startPair.Lane == "" || startPair.Lane != donePair.Lane {
		t.Fatalf("text owner entered request identity or lost provenance: start=%+v done=%+v", startPair, donePair)
	}

	for _, source := range []struct {
		name  string
		input []byte
	}{
		{
			name: "binary_bytrace",
			input: syntheticProfilerTraceFile(
				syntheticProfilerPluginData("bytrace_plugin", []byte(idleStart+"\n")),
				syntheticProfilerPluginData("bytrace_plugin", []byte(positiveDone+"\n")),
			),
		},
		{
			name:  "session",
			input: []byte(profilerSessionJSONTag + "\n" + idleStart + "\n" + positiveDone + "\n"),
		},
	} {
		t.Run(source.name, func(t *testing.T) {
			dir := t.TempDir()
			inputPath := filepath.Join(dir, "input.trace")
			outputPath := filepath.Join(dir, "output.ftrace")
			if err := os.WriteFile(inputPath, source.input, 0o600); err != nil {
				t.Fatal(err)
			}
			result, handled, err := tryConvertProfilerContainer(context.Background(), Options{InputPath: inputPath},
				int64(len(source.input)), outputPath, nil, nil, nil, nil, nil)
			if err != nil || !handled || result.EventsWritten != 2 {
				t.Fatalf("owner-crossing text conversion: handled=%t result=%+v err=%v", handled, result, err)
			}
			index, err := tracequery.BuildIndex(context.Background(), outputPath)
			if err != nil {
				t.Fatal(err)
			}
			stats := tracequery.ComputeWindowStats(index, tracequery.Query{})
			if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].IssueThread.PID != 0 ||
				stats.IOLatencies[0].CompleteThread.PID != 41 {
				t.Fatalf("idle-to-positive text endpoints did not form one request duration: %+v caveats=%+v",
					stats.IOLatencies, stats.Caveats)
			}
		})
	}
}
