package tracequery

import (
	"context"
	"strings"
	"testing"
)

func TestRunRejectsUnknownViewWithoutEventSearchFallback(t *testing.T) {
	path := writeTraceMarkActionFilterFixture(t)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"storage_latency_by_layer", "invented_statistics_view"} {
		t.Run(view, func(t *testing.T) {
			got := Run(idx, Query{View: view, TimeStart: 1, TimeEnd: 14, TimeStartSet: true, TimeEndSet: true})
			if got.View != view || len(got.Events) != 0 || len(got.EvidencePack) != 0 || got.EventSearchCoverage != nil || !strings.Contains(strings.Join(got.Caveats, "\n"), "view_invalid=true") {
				t.Fatalf("unknown view was executed instead of rejected: %+v", got)
			}
		})
	}
}

func TestStreamingRejectsUnknownViewBeforeForcingItsView(t *testing.T) {
	path := writeTraceMarkActionFilterFixture(t)
	for _, stream := range []struct {
		name string
		run  func(context.Context, string, Query) (Result, error)
	}{
		{"event_search", StreamEventSearch},
		{"window_sweep", StreamWindowSweep},
		{"state_cluster", func(ctx context.Context, path string, q Query) (Result, error) {
			return StreamStateCluster(ctx, path, q, 8)
		}},
	} {
		t.Run(stream.name, func(t *testing.T) {
			for _, inputPath := range []string{path, path + ".missing"} {
				got, err := stream.run(context.Background(), inputPath, Query{View: "invented_statistics_view", TimeStart: 1, TimeEnd: 14})
				if err == nil || !strings.Contains(err.Error(), "unsupported view") || len(got.Events) != 0 || len(got.EvidencePack) != 0 {
					t.Fatalf("stream silently accepted unknown view or accessed the path first: result=%+v err=%v", got, err)
				}
			}
		})
	}
}

func TestRunDoesNotSearchForStreamingOnlyView(t *testing.T) {
	idx, err := BuildIndex(context.Background(), writeTraceMarkActionFilterFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	got := Run(idx, Query{View: ViewWindowSweep, TimeStart: 1, TimeEnd: 14})
	if got.View != ViewWindowSweep || len(got.Events) != 0 || len(got.EvidencePack) != 0 || !strings.Contains(strings.Join(got.Caveats, "\n"), "StreamWindowSweep") {
		t.Fatalf("streaming-only view silently fell back to search: %+v", got)
	}
}

func TestValidateViewNameAcceptsClosedUniverseDefaultsAndExistingAliases(t *testing.T) {
	views := append(CanonicalViewNames(), "", "  ", " frame_bundle ", "frame_rootcause_bundle", "frame_root_cause")
	for _, view := range views {
		if err := ValidateViewName(view); err != nil {
			t.Errorf("existing view %q rejected: %v", view, err)
		}
	}
	idx, err := BuildIndex(context.Background(), writeTraceMarkActionFilterFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"", "  ", "event_search"} {
		got := Run(idx, Query{View: view, TimeStart: 0, TimeEnd: 14, TimeStartSet: true, TimeEndSet: true})
		if got.View != FallbackViewEventSearch || len(got.Events) == 0 || got.TimeStart != 0 || got.TimeEnd != 14 {
			t.Fatalf("default search or explicit zero window changed: %+v", got)
		}
	}
}

func TestStreamStateClusterKeepsKnownHeavyViewFallback(t *testing.T) {
	for _, view := range []string{"", "wakeup_chain", "root_cause_rank"} {
		_, err := StreamStateCluster(context.Background(), writeTraceMarkActionFilterFixture(t), Query{View: view, TimeStart: 1, TimeEnd: 2}, 8)
		if err != nil {
			t.Fatalf("known heavy-view fallback %q rejected: %v", view, err)
		}
	}
}
