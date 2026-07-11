package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestExportTraceDBDMAFenceHighLevelRowsFailClosed(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE dma_fence (id, ts, dur, cat, driver, timeline, context, seqno)",
		"INSERT INTO dma_fence VALUES (1, 1000000, 0, 'dma_fence_wait_start', 'drv', 'shared', 7, 9)",
		"INSERT INTO dma_fence VALUES (2, 2000000, 1000000, 'dma_fence_signaled', 'drv', 'shared', 8, 10)",
		"INSERT INTO dma_fence VALUES (3, 2.5, 1000, 'dma_fence_wait_end', 'drv', 'shared', 7, 9)",
		"INSERT INTO dma_fence VALUES (4, 3000000, NULL, 'dma_fence_wait_end', 'drv', 'shared', 7, 9)",
		"INSERT INTO dma_fence VALUES (5, 4000000, -1, NULL, 'drv', 'shared', 'bad', 'bad')",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBDMAFence(context.Background(), tdb, sink, traceDBThreadIndex{}, nil, nil)
	if err != nil {
		t.Fatalf("malformed high-level DMA rows must fail closed locally: %v", err)
	}
	if coverage.RowsRead != 5 || coverage.RowsEmitted != 0 || sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 {
		t.Fatalf("high-level DMA row minted an endpoint: coverage=%+v stats=%+v rows=%+v", coverage, sink.stats, sink.rows)
	}
	if coverage.Role != "unsupported_input" {
		t.Fatalf("withheld high-level DMA rows advertised query-ready output: %+v", coverage)
	}
	for _, want := range []string{
		"high_level_rows_withheld=5",
		"predecessor_delta_not_duration=true",
		"unresolved_emitter_identity_cpu=true",
		"raw_dma_path_only=true",
	} {
		if !strings.Contains(coverage.Skipped, want) {
			t.Fatalf("DMA coverage missing %q: %+v", want, coverage)
		}
	}
	if coverage.FieldSources["dur"] == "" || coverage.FieldSources["header_cpu"] == "" ||
		!containsString(coverage.ColumnsPresent, "id") {
		t.Fatalf("DMA provenance/profile incomplete: %+v", coverage)
	}
}

func TestExportTraceDBDMAFenceCoverageOnlyProfilesFailClosed(t *testing.T) {
	tests := []struct {
		name        string
		statements  []string
		wantFound   bool
		wantRows    int
		wantMissing string
		wantSkipped string
	}{
		{name: "missing table", statements: []string{"CREATE TABLE unrelated (id INT)"}, wantSkipped: "missing table"},
		{
			name:       "empty complete table",
			statements: []string{"CREATE TABLE dma_fence (ts, dur, cat, driver, timeline, context, seqno)"},
			wantFound:  true,
		},
		{
			name: "missing required column",
			statements: []string{
				"CREATE TABLE dma_fence (ts, dur, cat, driver, timeline, context)",
				"INSERT INTO dma_fence VALUES (1, 1, 'x', 'd', 't', 7)",
			},
			wantFound:   true,
			wantRows:    1,
			wantMissing: "seqno",
			wantSkipped: "missing required columns: seqno",
		},
		{
			name: "large opaque payload is never materialized",
			statements: []string{
				"CREATE TABLE dma_fence (ts, dur, cat, driver, timeline, context, seqno)",
				"INSERT INTO dma_fence VALUES (1, 1, zeroblob(2097152), zeroblob(2097152), zeroblob(2097152), zeroblob(2097152), zeroblob(2097152))",
			},
			wantFound: true,
			wantRows:  1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := createTraceDBFixture(t, test.statements)
			tdb, err := openTraceDB(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer tdb.close()
			sink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			coverage, err := exportTraceDBDMAFence(context.Background(), tdb, sink, traceDBThreadIndex{}, nil, nil)
			if err != nil {
				t.Fatalf("coverage-only profile must fail closed locally: %v", err)
			}
			if coverage.Role != "unsupported_input" || coverage.Found != test.wantFound ||
				coverage.RowsRead != test.wantRows || coverage.RowsEmitted != 0 ||
				sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 {
				t.Fatalf("coverage-only profile minted output or lost provenance: coverage=%+v stats=%+v rows=%+v", coverage, sink.stats, sink.rows)
			}
			if test.wantMissing != "" && !containsString(coverage.ColumnsMissing, test.wantMissing) {
				t.Fatalf("missing-column provenance lost: %+v", coverage)
			}
			if test.wantSkipped != "" && coverage.Skipped != test.wantSkipped {
				t.Fatalf("coverage reason mismatch: got %q want %q (%+v)", coverage.Skipped, test.wantSkipped, coverage)
			}
		})
	}
}

func TestExportTraceDBDMAFenceRawAuthoritySurvivesOnce(t *testing.T) {
	statements := rawFtraceRootCauseFixtureStatements()
	statements = append(statements,
		"CREATE TABLE dma_fence (id INT, ts INT, dur INT, cat TEXT, driver TEXT, timeline TEXT, context INT, seqno INT)",
		"INSERT INTO dma_fence VALUES (1, 17000, 0, 'dma_fence_wait_start', 'display', 'present', 7, 9)",
		"INSERT INTO dma_fence VALUES (2, 5017000, 5000000, 'dma_fence_wait_end', 'display', 'present', 7, 9)",
		"INSERT INTO data_dict VALUES (9001, 'driver')",
		"INSERT INTO data_dict VALUES (9002, 'timeline')",
		"INSERT INTO data_dict VALUES (9003, 'context')",
		"INSERT INTO data_dict VALUES (9004, 'seqno')",
		"INSERT INTO data_dict VALUES (9011, 'display')",
		"INSERT INTO data_dict VALUES (9012, 'present')",
		"INSERT INTO args VALUES (900, 9001, 1, 9011)",
		"INSERT INTO args VALUES (900, 9002, 1, 9012)",
		"INSERT INTO args VALUES (900, 9003, 0, 7)",
		"INSERT INTO args VALUES (900, 9004, 0, 9)",
		"INSERT INTO args VALUES (901, 9001, 1, 9011)",
		"INSERT INTO args VALUES (901, 9002, 1, 9012)",
		"INSERT INTO args VALUES (901, 9003, 0, 7)",
		"INSERT INTO args VALUES (901, 9004, 0, 9)",
		"INSERT INTO raw VALUES (15, 17000, 'dma_fence_wait_start', 3, 3, 900)",
		"INSERT INTO raw VALUES (16, 5017000, 'dma_fence_wait_end', 3, 3, 901)",
	)
	path := createTraceDBFixture(t, statements)
	outPath := filepath.Join(t.TempDir(), "dma-raw-authority.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export DMA raw-authority fixture: %v", err)
	}
	if !coverageHasSkipped(result.Coverage, "slice", "dma_fence", "high_level_rows_withheld=2") {
		t.Fatalf("high-level DMA suppression missing: %+v", result.Coverage)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, endpoint := range []string{"dma_fence_wait_start:", "dma_fence_wait_end:"} {
		if count := strings.Count(body, endpoint); count != 1 {
			t.Fatalf("authoritative raw DMA endpoint %q must appear exactly once, got %d:\n%s", endpoint, count, body)
		}
	}
	for lineNo, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "<dma_fence>") {
			continue
		}
		for _, phase := range []string{"B", "E", "S", "F"} {
			// The old duration exporter emitted a name-less E marker. Its
			// <dma_fence> task header, rather than a payload name, is the
			// reliable discriminator for that bare endpoint.
			if strings.Contains(line, "tracing_mark_write: "+phase+"|") {
				t.Fatalf("high-level DMA leaked synthetic %s endpoint at output line %d: %s", phase, lineNo+1, line)
			}
		}
	}
	if strings.Contains(body, "<dma_fence>") {
		t.Fatalf("high-level DMA row leaked a synthetic PID0/CPU0 endpoint:\n%s", body)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse DMA raw authority: %v", err)
	}
	var events []tracequery.Event
	for _, event := range idx.Events {
		if event.Type == tracequery.EventDMAFence && strings.HasPrefix(event.Name, "dma_fence_wait_") {
			events = append(events, event)
		}
	}
	if len(events) != 2 || events[0].PID != 800 || events[0].TGID != 500 || events[0].CPU != 3 ||
		events[1].PID != 800 || events[1].TGID != 500 || events[1].CPU != 3 {
		t.Fatalf("DMA authority/provenance mismatch: %+v", events)
	}

	query := tracequery.Query{PID: 800}
	stats := tracequery.ComputeWindowStats(idx, query)
	var pairedActivities []tracequery.DMAFenceActivity
	var totalPaired int
	var totalWaitMs float64
	for _, candidate := range stats.DMAFenceActivity {
		totalPaired += candidate.PairedCount
		totalWaitMs += candidate.WaitMs
		if candidate.PairedCount > 0 {
			pairedActivities = append(pairedActivities, candidate)
		}
	}
	if len(pairedActivities) != 1 || totalPaired != 1 || totalWaitMs < 4.999 || totalWaitMs > 5.001 {
		t.Fatalf("raw DMA wait must pair exactly once without sibling double-counting: %+v", stats.DMAFenceActivity)
	}
	activity := pairedActivities[0]
	if activity.Count != 2 || activity.PairedCount != 1 || activity.UnpairedStartCount != 0 ||
		activity.UnpairedDoneCount != 0 || activity.PairingSuppressedCount != 0 ||
		activity.WaitMs < 4.999 || activity.WaitMs > 5.001 || activity.MaxWaitMs < 4.999 || activity.MaxWaitMs > 5.001 ||
		activity.Thread.PID != 800 || activity.Driver != "display" || activity.Timeline != "present" ||
		activity.Context != "7" || activity.Seqno != "9" || activity.SourcePath == "" {
		t.Fatalf("raw DMA pairing/caliber mismatch: %+v", activity)
	}

	rank := tracequery.BuildRootCauseRank(idx, query)
	var dmaRanks []tracequery.RootCauseRankItem
	for _, item := range rank.Items {
		if item.Type == "dma_fence_activity" {
			dmaRanks = append(dmaRanks, item)
		}
	}
	if len(dmaRanks) != 1 || dmaRanks[0].Rank <= 0 || dmaRanks[0].Thread.PID != 800 ||
		dmaRanks[0].ImpactMs < 4.999 || dmaRanks[0].ImpactMs > 5.001 ||
		dmaRanks[0].EffectiveImpactMs < 4.999 || dmaRanks[0].EffectiveImpactMs > 5.001 ||
		dmaRanks[0].PhysicalSourcePath == "" ||
		dmaRanks[0].MemberKey != "driver=display timeline=present ctx=7 seqno=9" {
		t.Fatalf("paired raw DMA wait must own exactly one root-cause seat: %+v (all=%+v)", dmaRanks, rank.Items)
	}
}
