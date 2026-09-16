package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1710ConsumerEpisode(resource bool) tracequery.IOBurstEpisodeSummary {
	row := tracequery.IOBurstEpisodeSummary{
		Thread:               tracequery.ThreadRef{Comm: "worker", PID: 41},
		RootCauseEligibility: "context_only_derived_projection", DominantSignal: "scheduler_iowait",
		DurationMs: .09, IOWaitMs: .09, StartTs: 5.000120, EndTs: 5.000210,
		LineStart: 9, LineEnd: 11, Confidence: .82, Summary: "observed scheduler wait",
	}
	if resource {
		row.RootCauseEligibility = "eligible_exact_chain_host_work"
		row.DominantSignal = "inode_storage_latency"
		row.TopInode, row.TopDev, row.TopEntryName = "0xaa", "8,0", "data.db"
		row.BlockMaxLatencyMs, row.StorageMaxLatencyMs = .099, .020
		row.FileIOBytes, row.PageCacheChurn = 4096, 1
		row.Summary = "exact producer-owned storage work"
	}
	return row
}

func TestB1710IOBurstTextConsumersKeepResourcePresence(t *testing.T) {
	for _, resource := range []bool{false, true} {
		name := "scheduler_only"
		if resource {
			name = "exact_resources"
		}
		t.Run(name, func(t *testing.T) {
			row := b1710ConsumerEpisode(resource)
			stats := tracequery.WindowStats{IOBurstEpisodes: []tracequery.IOBurstEpisodeSummary{row}}
			ordinary := traceQuerySummary(tracequery.Result{View: "window_stats", WindowStats: &stats}, traceQueryParams{}, "capture.systrace", "")
			var bundle strings.Builder
			writeTraceFrameRootCauseBundleSummary(&bundle, &tracequery.FrameRootCauseBundle{IOBurstEpisodes: stats.IOBurstEpisodes})
			for _, body := range []string{
				traceQuerySummaryLineWithPrefix(ordinary, "- io_burst_episode "),
				traceQuerySummaryLineWithPrefix(bundle.String(), "- bundle_io_burst "),
			} {
				if !strings.Contains(body, "worker-41") || !strings.Contains(body, "duration=0.090ms") || !strings.Contains(body, row.Summary) {
					t.Fatalf("text consumer lost the episode's own facts: %s", body)
				}
				if resource {
					for _, field := range []string{"block_max=0.099ms", "storage_max=0.020ms", "inode=0xaa", "dev=8,0", "name=data.db", "file_bytes=4096", "page_cache_churn=1"} {
						if !strings.Contains(body, field) {
							t.Errorf("owned resource field %s missing: %s", field, body)
						}
					}
				} else {
					for _, field := range []string{"block_max=", "storage_max=", "inode=", "dev=", "name=", "file_bytes=", "page_cache_churn="} {
						if strings.Contains(body, field) {
							t.Errorf("unassigned resource printed as a measured field (%s): %s", field, body)
						}
					}
				}
			}
		})
	}
}

func TestB1710IOBurstTypedNotesAlreadyOmitAbsentResources(t *testing.T) {
	for _, resource := range []bool{false, true} {
		row := b1710ConsumerEpisode(resource)
		stats := tracequery.WindowStats{IOBurstEpisodes: []tracequery.IOBurstEpisodeSummary{row}}
		records := traceQueryTypedWindowStatsObservations(stats, types.ObservationSourceRef{Path: "capture.systrace"}, "test", "")
		found := false
		for _, record := range records {
			if record.Predicate != "io_burst_episode" {
				continue
			}
			found = true
			if record.Span.StartTs != row.StartTs || record.Span.EndTs != row.EndTs || record.Span.LineStart != row.LineStart || record.Span.LineEnd != row.LineEnd || record.Value != "0.090" || record.Object != row.TopInode {
				t.Fatalf("typed wait identity/value/span changed: %+v", record)
			}
			notes := strings.Join(record.RichNotes, "\n")
			for _, key := range []string{"block_max=", "storage_max=", "inode=", "dev=", "name=", "file_bytes=", "page_cache_churn="} {
				if strings.Contains(notes, key) != resource {
					t.Errorf("typed resource presence mismatch resource=%v key=%s notes=%v", resource, key, record.RichNotes)
				}
			}
		}
		if !found {
			t.Fatal("typed episode was dropped")
		}
	}
}
