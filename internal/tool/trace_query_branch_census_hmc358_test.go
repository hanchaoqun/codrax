package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The raw reference capture is an opt-in, read-only test input. Values come
// from an independent header walk, not from a Codrax preview or expected answer.
func TestHMC358CompleteRawBranchCensusPublic(t *testing.T) {
	source := os.Getenv("CODRAX_TEST_BRANCH_CENSUS_SOURCE")
	if source == "" {
		t.Skip("set CODRAX_TEST_BRANCH_CENSUS_SOURCE to the pinned complete test_brbe.data")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	sha, size := hmc176bRepresentativeDigest(t, source)
	if size != 30062585 || sha != "da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632" {
		t.Fatalf("not the pinned whole capture: %d %s", size, sha)
	}
	t.Cleanup(func() {
		if digest, bytes := hmc176bRepresentativeDigest(t, source); digest != sha || bytes != size {
			t.Error("binary original modified")
		}
	})
	hmc176DisableExternalProviders(t)
	bus, _, conversions := hmc17NamedPathContext(t)
	args := map[string]any{"source": "path", "path": source, "view": "perf_stats", "limit": 1}
	for _, tc := range []struct {
		name   string
		want   tracequery.PerfBranchRecordCensus
		weight int64
	}{
		// Slot counts can be added across disjoint selected records. Legacy
		// cross-cohort weight is withheld; it must not be restored by this view.
		{"whole", tracequery.PerfBranchRecordCensus{SamplesWithCount: 12000, SamplesWithEntries: 11978, RecordedEntries: 369677}, 0},
		{"window", tracequery.PerfBranchRecordCensus{SamplesWithCount: 164, SamplesWithEntries: 163, RecordedEntries: 5216}, 259},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "window" {
				args["pid"], args["time_start"], args["time_end"] = 1609, 486489.4, 486489.6
			}
			result := hmc17NamedQuery(t, bus, args)
			payload := hmc17NamedPayload(t, result)
			stats := payload.PerfStats
			if stats == nil || stats.Quality == nil || stats.Quality.BranchRecords == nil {
				t.Fatal("selected native census is unavailable")
			}
			if *stats.Quality.BranchRecords != tc.want || stats.SampleCount != tc.want.SamplesWithCount || stats.TotalPeriod != tc.weight {
				t.Fatalf("samples=%d weight=%d census=%+v; want=%+v weight=%d", stats.SampleCount, stats.TotalPeriod, stats.Quality.BranchRecords, tc.want, tc.weight)
			}
			if !strings.Contains(result.Summary, tc.want.Compact()) || conversions.Load() != 1 || payload.SourcePath == source {
				t.Fatal("summary lost native census or used the wrong source/prepare generation")
			}
			if tc.name != "window" {
				return
			}
			if len(stats.Cohorts) != 1 || stats.Cohorts[0].Quality.BranchRecords == nil || *stats.Cohorts[0].Quality.BranchRecords != tc.want {
				t.Fatal("physical selected cohort borrowed another population")
			}
			input := types.ObservationLedgerInputFromBusContext(bus, 0)
			input.ToolResults = []types.ToolResult{result}
			found := 0
			for _, table := range types.BuildRuntimeMeasurementContract(input).Choices() {
				if !table.HasNativeSamplingFacts() || table.View != types.RuntimeMeasurementDistribution {
					continue
				}
				found++
				title, valid := table.NativeSourceTitleBasis()
				if !valid || title.OriginalSource != source || title.QuerySource != payload.SourcePath {
					t.Fatalf("original/derived identities were swapped: %+v", title)
				}
				notes := strings.Join(table.Notes, "\n")
				if !strings.Contains(notes, "合计 5216 个槽位") || !strings.Contains(notes, "163 条非空") ||
					strings.Contains(notes, "原件："+payload.SourcePath) || table.HasCompleteNativeDisplayPopulation() {
					t.Fatalf("scope/provenance or permission drift in native display: %s", notes)
				}
			}
			if found == 0 {
				t.Fatal("original-bound native report has no selectable census")
			}
			t.Logf("samples=%d weight=%d %s", stats.SampleCount, stats.TotalPeriod, tc.want.Compact())
		})
	}
}
