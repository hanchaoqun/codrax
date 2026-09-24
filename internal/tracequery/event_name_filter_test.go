package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEventNamesSharedExactContract(t *testing.T) {
	names := []string{"mmc_request_start", "MMC_REQUEST_START", "mmc_request_start", " x "}
	got, err := NormalizeEventSearchNames("event_search", names)
	if err != nil || !reflect.DeepEqual(got, []string{"mmc_request_start", "MMC_REQUEST_START", " x "}) {
		t.Fatalf("exact names must retain case and bytes: %q %v", got, err)
	}
	got[0] = "changed"
	if names[0] != "mmc_request_start" {
		t.Fatal("normalization aliased input")
	}
	for _, tc := range []struct {
		view  string
		names []string
	}{
		{"window_stats", []string{"mmc_request_start"}}, {"event_search", []string{" "}},
		{"event_search", make([]string, EventSearchNameLimit+1)},
	} {
		if _, err := NormalizeEventSearchNames(tc.view, tc.names); err == nil {
			t.Fatalf("invalid name contract accepted: %+v", tc)
		}
	}
	if out, err := NormalizeEventSearchNames("window_stats", nil); out != nil || err != nil {
		t.Fatal("absent names changed old view behavior")
	}
}

func TestEventNamesPublicStreamIndexAndCensus(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_activity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		q     Query
		count int
	}{
		{"rq_only", Query{EventNames: []string{"block_rq_issue"}}, 9},
		{"bio_only", Query{EventNames: []string{"block_bio_queue", "block_bio_complete"}}, 2},
		{"mmc", Query{EventNames: []string{"mmc_request_start", "mmc_request_done"}}, 2},
		{"f2fs", Query{EventNames: []string{"f2fs_sync_file_enter", "f2fs_sync_file_exit"}}, 2},
		{"case_sensitive", Query{EventNames: []string{"MMC_REQUEST_START"}}, 0},
		{"no_prefix_match", Query{EventNames: []string{"mmc_request"}}, 0},
		{"no_payload_match", Query{EventNames: []string{"reader"}}, 0},
		{"and_category", Query{EventNames: []string{"block_bio_queue"}, EventTypes: []EventType{EventBlockComplete}}, 0},
		{"and_pattern", Query{EventNames: []string{"block_rq_issue"}, Pattern: "16384"}, 1},
		{"and_identity", Query{EventNames: []string{"block_rq_issue"}, PID: 45}, 2},
		{"time_closed_lookup_unchanged", Query{EventNames: []string{"block_rq_issue"}, TimeStart: 2, TimeEnd: 2.25, TimeStartSet: true, TimeEndSet: true}, 8},
		{"line_priority", Query{EventNames: []string{"block_rq_issue"}, LineStart: 4, LineEnd: 4, TimeStart: 50, TimeEnd: 60}, 1},
		{"legacy_category_contains_bio", Query{EventTypes: []EventType{EventBlockIssue}}, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.q
			q.View, q.Limit = "event_search", 1
			indexed := Run(idx, q)
			streamed, err := StreamEventSearch(context.Background(), path, q)
			if err != nil {
				t.Fatal(err)
			}
			for lane, res := range map[string]Result{"index": indexed, "stream": streamed} {
				if res.EventSearchCoverage == nil || res.EventSearchCoverage.MatchedTotal != tc.count || res.EventSearchCoverage.Emitted != len(res.Events) {
					t.Fatalf("%s count=%+v want=%d caveats=%v", lane, res.EventSearchCoverage, tc.count, res.Caveats)
				}
				for _, event := range res.Events {
					if !eventMatchesNames(event.Event, q.EventNames) {
						t.Fatalf("%s leaked %s", lane, event.Name)
					}
				}
			}
			if len(indexed.Events) != len(streamed.Events) || (len(indexed.Events) > 0 && indexed.Events[0].Line != streamed.Events[0].Line) {
				t.Fatal("stream/index membership differs")
			}
		})
	}
}

func TestEventNamesCarrierNotSpanOrBodyAndInvalidPublicViews(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marks.systrace")
	body := "app-20 (20) [001] .... 1.000000: tracing_mark_write: B|20|mmc_request_start\n" +
		"app-20 (20) [001] .... 1.001000: tracing_mark_write: E|20\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for name, count := range map[string]int{"tracing_mark_write": 2, "mmc_request_start": 0} {
		q := Query{View: "event_search", EventNames: []string{name}}
		if got := Run(idx, q); len(got.Events) != count {
			t.Fatalf("name %s got %d", name, len(got.Events))
		}
	}
	q := Query{View: "window_stats", EventNames: []string{"tracing_mark_write"}}
	if got := Run(idx, q); got.WindowStats != nil || !strings.Contains(strings.Join(got.Caveats, " "), "event_names") {
		t.Fatal("wrong view silently ignored names")
	}
	if _, err := StreamEventSearch(context.Background(), "/does-not-exist", q); err == nil || !strings.Contains(err.Error(), "event_names") {
		t.Fatalf("shared validation must precede source IO: %v", err)
	}
}
