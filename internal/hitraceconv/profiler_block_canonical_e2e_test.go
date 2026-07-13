package hitraceconv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func extractProfilerBlockCanonicalContainer(t *testing.T, structured []byte) (profilerContainerExtraction, *traceDBRowSink, error) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "block-canonical.htrace")
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData("ftrace-plugin", structured))
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink("", 128)
	if err != nil {
		t.Fatal(err)
	}
	extracted, conversionErr := extractProfilerContainerSystraceRows(context.Background(), input, info.Size(), sink)
	return extracted, sink, conversionErr
}

func profilerBlockCanonicalStructuredResult(record profilerFtraceEventRecord, comm string) []byte {
	return protoMessage(2,
		protoVarint(1, uint64(record.CPU)),
		syntheticTracePluginFtraceEvent(record.TSNS, uint64(record.TGID), uint64(record.PID),
			comm, record.Field, record.Payload),
	)
}

func TestProfilerBlockCanonicalBoundarySurvivesRealContainer(t *testing.T) {
	for _, item := range []struct {
		event, display int
	}{{204, 5}, {209, 6}, {210, 6}, {211, 6}} {
		t.Run(fmt.Sprintf("field%d", item.event), func(t *testing.T) {
			exact := profilerBlockTypedCanonicalFixture(t, item.event, item.display, 0)
			over := profilerBlockTypedCanonicalFixture(t, item.event, item.display, 1)
			for _, test := range []struct {
				name        string
				record      profilerFtraceEventRecord
				wantEmitted int
			}{
				{name: "exact-cap", record: exact, wantEmitted: 1},
				{name: "cap-plus-one", record: over, wantEmitted: 0},
			} {
				t.Run(test.name, func(t *testing.T) {
					extracted, sink, conversionErr := extractProfilerBlockCanonicalContainer(t,
						profilerBlockCanonicalStructuredResult(test.record, test.record.Comm))
					defer sink.cleanup()
					if conversionErr != nil {
						t.Fatalf("canonical boundary became a container conversion error: %v", conversionErr)
					}
					coverage, entries := profilerEventCoverageByField(extracted, item.event)
					if entries != 1 || coverage.RowsRead != 1 || coverage.RowsEmitted != test.wantEmitted {
						t.Fatalf("container canonical accounting drifted: entries=%d coverage=%+v", entries, coverage)
					}
					if test.wantEmitted == 1 {
						if extracted.SourceFailClosed || extracted.StructuredRows != 1 ||
							sink.stats.RowsAccepted != 1 || sink.publishableRows() != 1 || len(sink.rows) != 1 ||
							len(sink.rows[0].line) != maxTraceDBSystraceLineBytes ||
							coverage.FieldSources["degraded_invalid_canonical_block_line_occurrences"] != "" {
							t.Fatalf("exact-cap row did not survive the real container unchanged: extracted=%+v sink=%+v coverage=%+v rows=%+v",
								extracted, sink.stats, coverage, sink.rows)
						}
						return
					}
					if extracted.SourceFailClosed || extracted.StructuredRows != 0 ||
						sink.stats.RowsAccepted != 0 || sink.publishableRows() != 0 || len(sink.rows) != 0 ||
						coverage.FieldSources["degraded_invalid_canonical_block_line_occurrences"] != "1" ||
						coverage.FieldSources["degraded_invalid_canonical_block_line_affected_frames"] != "1" {
						t.Fatalf("cap-plus-one row was not rejected locally by the real container: extracted=%+v sink=%+v coverage=%+v rows=%+v",
							extracted, sink.stats, coverage, sink.rows)
					}
				})
			}
		})
	}
}

func TestProfilerBlockCanonicalRejectIsSiblingLocalAndPairNeutralInBothOrders(t *testing.T) {
	aux := profilerAuxCasesByField()
	over := profilerBlockTypedCanonicalFixture(t, 210, 6, 1)
	healthy := profilerBlockTypedRecord(210, profilerBlockTypedPayload(210, nil))
	for _, reverse := range []bool{false, true} {
		name := "bad-then-healthy"
		blockEvents := [][]byte{
			syntheticTracePluginFtraceEvent(1_000_003_000, 40, 40, "bad", over.Field, over.Payload),
			syntheticTracePluginFtraceEvent(1_000_004_000, 40, 40, "healthy", healthy.Field, healthy.Payload),
		}
		if reverse {
			name = "healthy-then-bad"
			blockEvents = [][]byte{
				syntheticTracePluginFtraceEvent(1_000_003_000, 40, 40, "healthy", healthy.Field, healthy.Payload),
				syntheticTracePluginFtraceEvent(1_000_004_000, 40, 40, "bad", over.Field, over.Payload),
			}
		}
		t.Run(name, func(t *testing.T) {
			parts := [][]byte{
				protoVarint(1, 2),
				syntheticTracePluginFtraceEvent(1_000_001_000, 40, 40, "f2fs", 4011,
					profilerAuxEncodeValues(aux[4011].values)),
				syntheticTracePluginFtraceEvent(1_000_002_000, 40, 40, "mmc", 4016,
					profilerAuxEncodeValues(aux[4016].values)),
			}
			parts = append(parts, blockEvents...)
			parts = append(parts,
				syntheticTracePluginFtraceEvent(1_000_005_000, 40, 40, "f2fs", 4012,
					profilerAuxEncodeValues(aux[4012].values)),
				syntheticTracePluginFtraceEvent(1_000_006_000, 40, 40, "mmc", 4015,
					profilerAuxEncodeValues(aux[4015].values)),
			)
			extracted, sink, conversionErr := extractProfilerBlockCanonicalContainer(t, protoMessage(2, parts...))
			defer sink.cleanup()
			if conversionErr != nil {
				t.Fatalf("block-local reject became a container conversion error: %v", conversionErr)
			}
			coverage, entries := profilerEventCoverageByField(extracted, 210)
			if entries != 1 || coverage.RowsRead != 2 || coverage.RowsEmitted != 1 ||
				coverage.FieldSources["degraded_invalid_canonical_block_line_occurrences"] != "1" ||
				coverage.FieldSources["degraded_invalid_canonical_block_line_affected_frames"] != "1" {
				t.Fatalf("block sibling accounting drifted: entries=%d coverage=%+v", entries, coverage)
			}
			if extracted.SourceFailClosed || extracted.StructuredRows != 5 || sink.stats.RowsAccepted != 5 ||
				sink.publishableRows() != 5 || sink.pairKindPoisoned(pairRenderMMC) ||
				sink.pairKindPoisoned(pairRenderF2FS) || sink.withheldPairRows() != 0 {
				t.Fatalf("bad block contaminated a healthy sibling or pair state: extracted=%+v accepted=%d publishable=%d withheld=%d poisoned=%v lanes=%v",
					extracted, sink.stats.RowsAccepted, sink.publishableRows(), sink.withheldPairRows(), sink.poisoned, sink.poisonedLanes)
			}

			var out bytes.Buffer
			stats, writeErr := sink.writeTo(context.Background(), &out)
			if writeErr != nil {
				t.Fatal(writeErr)
			}
			text := out.String()
			if stats.RowsAccepted != 5 || stats.RowsWritten != 5 || stats.RowsWithheld != 0 ||
				strings.Contains(text, "bad") || !strings.Contains(text, "healthy") ||
				!strings.Contains(text, "f2fs_write_begin:") || !strings.Contains(text, "f2fs_write_end:") ||
				!strings.Contains(text, "mmc_request_start:") || !strings.Contains(text, "mmc_request_done:") {
				t.Fatalf("pair publication or block sibling locality drifted: stats=%+v\n%s", stats, text)
			}

			output := filepath.Join(t.TempDir(), "pair-neutral.ftrace")
			if err := os.WriteFile(output, out.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			index, err := tracequery.BuildIndex(context.Background(), output)
			if err != nil {
				t.Fatal(err)
			}
			paired := map[string]int{}
			for _, row := range tracequery.ComputeWindowStats(index, tracequery.Query{}).StorageLatencyByLayer {
				paired[row.Layer] += row.PairedCount
			}
			if paired["mmc"] != 1 || paired["f2fs"] != 1 {
				t.Fatalf("bad block damaged real storage pairing: paired=%v", paired)
			}
		})
	}
}
