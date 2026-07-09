package tool

// trace_query_top_io_inode_test.go — INODE (§28.6, 2026-07-09) exposure pins
// for the whole-window (dev,inode) IO frequency carrier: the window_stats
// banner block (per-group line + mandatory group-total tail), the typed
// top_io_inode observation family (claim key, notes, groups_total
// truncation-honesty note, latency red-line form), the tool-description
// enumeration teaching, and the synthetic-trace end-to-end chain
// (BuildIndex → Run → banner + typed observations). Fixtures are minted by
// the ENGINE (§28.7 discipline), never hand-shaped.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func topIOInodeEngineResult(t *testing.T) tracequery.Result {
	t.Helper()
	content := `
      appA-100 (100) [000] .... 10.001000: android_fs_dataread_start: dev=259:1 ino=0xaa entry_name=hot.db offset=0 bytes=4096 rw=R
      appA-100 (100) [000] .... 10.001500: android_fs_dataread_end: dev=259:1 ino=0xaa bytes=4096 ret=4096 latency_us=4000 rw=R
      appA-100 (100) [000] .... 10.002000: android_fs_datawrite_start: dev=259:1 ino=0xaa offset=0 bytes=1024 rw=W
      appA-100 (100) [000] .... 10.002800: android_fs_datawrite_end: dev=259:1 ino=0xaa bytes=1024 ret=1024 latency_us=6000 rw=W
      appB-200 (200) [001] .... 10.003000: android_fs_dataread_start: dev=259:1 ino=0xaa offset=4096 bytes=2048 rw=R
      appB-200 (200) [001] .... 10.003700: android_fs_dataread_end: dev=259:1 ino=0xaa bytes=2048 ret=2048 latency_us=7000 rw=R
      appB-200 (200) [001] .... 10.004000: mm_filemap_add_to_page_cache: dev 259:1 ino 0xaa page=0000000000000000 pfn=1 ofs=0
      appC-300 (300) [002] .... 10.005000: android_fs_dataread_start: dev=259:1 ino=0xbb entry_name=cold.db offset=0 bytes=512 rw=R
      appD-400 (400) [003] .... 10.006000: android_fs_dataread_start: dev=259:1 entry_name=noino.db offset=0 bytes=128 rw=R
	`
	path := filepath.Join(t.TempDir(), "top_io_inode_e2e.systrace")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return tracequery.Run(idx, tracequery.Query{View: "window_stats", TimeStart: 10.0, TimeEnd: 11.0})
}

// TestTraceQuerySummaryTopIOInodeBannerBlock pins the banner exposure
// field-by-field: the per-group "- top_io_inode …" line (count-first order,
// closed-set read/write decomposition, single-event max latency, per-thread
// within-thread roster, entry label) and the mandatory trailing group-total
// disclosure line.
func TestTraceQuerySummaryTopIOInodeBannerBlock(t *testing.T) {
	result := topIOInodeEngineResult(t)
	if result.WindowStats == nil || result.WindowStats.TopIOInodes == nil {
		t.Fatalf("engine fixture must mint TopIOInodes: %+v", result.WindowStats)
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	// The busy group line, exact fields. max_latency is the largest SINGLE
	// event (7ms) — a cross-thread latency sum would print 17.000ms and die.
	if !strings.Contains(summary, "- top_io_inode dev=259:1 inode=0xaa events=7 reads=2 writes=1 completions=3 bytes=7168 page_cache_adds=1 page_cache_deletes=0 max_latency=7.000ms threads=2 top_threads=appA-100:10.000ms|appB-200:7.000ms entry=hot.db lines=") {
		t.Fatalf("top_io_inode banner line missing or drifted:\n%s", summary)
	}
	if !strings.Contains(summary, "- top_io_inode dev=259:1 inode=0xbb events=1 ") {
		t.Fatalf("second group line missing:\n%s", summary)
	}
	// §28.6 ④: the group-total tail is unconditional and discloses the
	// inode-less events instead of folding them into a pseudo-group.
	if !strings.Contains(summary, "- top_io_inode_groups total=2 shown=2 unidentified_io_events=1") {
		t.Fatalf("group-total disclosure tail missing:\n%s", summary)
	}
	// The banner appears in the window_stats section, before the legacy
	// per-(pid,op) file_io rows.
	if !strings.Contains(summary, "## Window stats") {
		t.Fatalf("window stats section missing:\n%s", summary)
	}
	topIdx := strings.Index(summary, "- top_io_inode ")
	fileIdx := strings.Index(summary, "- file_io ")
	if topIdx < 0 || fileIdx < 0 || topIdx > fileIdx {
		t.Fatalf("top_io_inode block must lead the legacy file_io rows (top=%d file=%d):\n%s", topIdx, fileIdx, summary)
	}
}

// TestTraceQuerySummaryTopIOInodeBannerAbsentWithoutCarrier pins the omission
// form: no TopIOInodes carrier → no banner block, no phantom tail line.
func TestTraceQuerySummaryTopIOInodeBannerAbsentWithoutCarrier(t *testing.T) {
	stats := tracequery.WindowStats{}
	result := tracequery.Result{View: "window_stats", WindowStats: &stats}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	if strings.Contains(summary, "top_io_inode") {
		t.Fatalf("no carrier must render no top_io_inode lines:\n%s", summary)
	}
}

// TestTraceQueryTypedTopIOInodeObservations pins the typed observation family:
// claim-key prefix, subject/predicate/object identity, the frequency-caliber
// value, and the notes contract (including the groups_total truncation
// disclosure and the within-thread-only latency roster).
func TestTraceQueryTypedTopIOInodeObservations(t *testing.T) {
	result := topIOInodeEngineResult(t)
	records := traceQueryTypedObservations(result, "top_io_inode_e2e.systrace", "/tmp/payload.json", "", "q1", time.Unix(0, 0).UTC())
	var rows []types.ObservationRecord
	for _, record := range records {
		if record.Predicate == "top_io_inode" {
			rows = append(rows, record)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 top_io_inode observations, got %d: %+v", len(rows), rows)
	}
	hot := rows[0]
	if hot.ClaimKey != "top_io_inode:0xaa" {
		t.Fatalf("claim key wrong: %+v", hot)
	}
	if hot.Subject != "hot.db" || hot.Object != "259:1" {
		t.Fatalf("subject/object identity wrong: %+v", hot)
	}
	if hot.Value != "7" || hot.Unit != "events" {
		t.Fatalf("frequency-caliber value wrong: %+v", hot)
	}
	notes := strings.Join(hot.RichNotes, "\n")
	for _, want := range []string{
		"inode=0xaa",
		"dev=259:1",
		"name=hot.db",
		"count=7",
		"reads=2",
		"writes=1",
		"completions=3",
		"bytes=7168",
		"adds=1",
		"max_latency=7.000",
		"threads=2",
		"top_threads=appA-100:10.000ms|appB-200:7.000ms",
		"groups_total=2",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("top_io_inode notes missing %q:\n%s", want, notes)
		}
	}
	// RED LINE: no note may carry a cross-thread latency sum (4+6+7=17ms).
	if strings.Contains(notes, "17.000") {
		t.Fatalf("cross-thread latency sum leaked into notes:\n%s", notes)
	}
	if hot.Origin != types.AnswerEvidenceOriginRuntimeArtifact || hot.GroundingPolicy != types.ClaimGroundingHard ||
		hot.Producer != "trace_query" {
		t.Fatalf("observation lane fields wrong: %+v", hot)
	}
	if !strings.HasPrefix(hot.Summary, "top_io_inode inode=0xaa dev=259:1 name=hot.db count=7") {
		t.Fatalf("typed summary wrong: %q", hot.Summary)
	}
	if hot.Span.LineStart <= 0 || hot.Span.LineEnd < hot.Span.LineStart {
		t.Fatalf("span envelope wrong: %+v", hot.Span)
	}
}

// TestTraceQueryTypedTopIOInodeObservationsGolden pins one full note slice
// verbatim (wire-format golden) so silent note reshuffles surface in review.
func TestTraceQueryTypedTopIOInodeObservationsGolden(t *testing.T) {
	result := topIOInodeEngineResult(t)
	records := traceQueryTypedObservations(result, "top_io_inode_e2e.systrace", "/tmp/payload.json", "", "q1", time.Unix(0, 0).UTC())
	for _, record := range records {
		if record.Predicate != "top_io_inode" || record.ClaimKey != "top_io_inode:0xaa" {
			continue
		}
		got := strings.Join(record.RichNotes, "\n")
		want := strings.Join([]string{
			"inode=0xaa",
			"dev=259:1",
			"name=hot.db",
			"count=7",
			"reads=2",
			"writes=1",
			"completions=3",
			"bytes=7168",
			"adds=1",
			"max_latency=7.000",
			"threads=2",
			"top_threads=appA-100:10.000ms|appB-200:7.000ms",
			"groups_total=2",
		}, "\n")
		if got != want {
			t.Fatalf("top_io_inode notes golden drifted:\n got:\n%s\nwant:\n%s", got, want)
		}
		return
	}
	t.Fatal("top_io_inode:0xaa observation missing")
}

// TestTraceQueryDescriptionDocumentsTopIOInodes pins the enumeration
// teaching sentence in the tool description (§28.6 ⑩ routing gap).
func TestTraceQueryDescriptionDocumentsTopIOInodes(t *testing.T) {
	description := (&TraceQuery{}).Description()
	for _, want := range []string{
		"which-inodes-have-the-most-IO ranking or enumeration questions",
		"window_stats top_io_inodes section",
		"before any per-section row truncation",
		"orders groups by total event count",
		"latency is never summed across threads",
		"discloses how many (dev,inode) groups exist beyond the listed rows",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("trace_query description missing top_io_inodes teaching %q", want)
		}
	}
}

// TestTraceQueryTopIOInodeFullChainBeyondTruncation is the end-to-end §28.6
// witness: a synthetic trace with 12 (dev,inode) groups where the count
// champion carries no latency — the legacy top-8 latency-sorted rows drop it,
// yet the banner block and the typed observation family both surface it,
// with the honest 12-group disclosure.
func TestTraceQueryTopIOInodeFullChainBeyondTruncation(t *testing.T) {
	var b strings.Builder
	ts := 10.0
	for i := 1; i <= 11; i++ {
		ino := fmt.Sprintf("0x%02x", i)
		fmt.Fprintf(&b, "      app-%d (%d) [000] .... %.6f: android_fs_dataread_start: dev=259:1 ino=%s offset=0 bytes=4096 rw=R\n", 100+i, 100+i, ts, ino)
		ts += 0.0001
		fmt.Fprintf(&b, "      app-%d (%d) [000] .... %.6f: android_fs_dataread_end: dev=259:1 ino=%s bytes=4096 ret=4096 latency_us=5000 rw=R\n", 100+i, 100+i, ts, ino)
		ts += 0.0001
	}
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "      hotapp-500 (500) [001] .... %.6f: android_fs_dataread_start: dev=259:1 ino=0xcc offset=0 bytes=16 rw=R\n", ts)
		ts += 0.0001
	}
	path := filepath.Join(t.TempDir(), "top_io_inode_wide.systrace")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "window_stats", TimeStart: 10.0, TimeEnd: 11.0})
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	if !strings.Contains(summary, "- top_io_inode dev=259:1 inode=0xcc events=20 ") {
		t.Fatalf("the count champion the legacy carrier truncates must lead the banner:\n%s", summary)
	}
	if !strings.Contains(summary, "- top_io_inode_groups total=12 shown=10") {
		t.Fatalf("the 12-group disclosure must survive end-to-end:\n%s", summary)
	}
	// The legacy rows really did drop the champion (fixture invariant).
	if strings.Contains(summary, "- file_io inode=0xcc") {
		t.Fatalf("fixture invariant broken — champion must be truncated from legacy rows:\n%s", summary)
	}
	records := traceQueryTypedObservations(result, "top_io_inode_wide.systrace", "/tmp/payload.json", "", "q1", time.Unix(0, 0).UTC())
	var champion *types.ObservationRecord
	for i := range records {
		if records[i].ClaimKey == "top_io_inode:0xcc" {
			champion = &records[i]
			break
		}
	}
	if champion == nil {
		t.Fatalf("champion observation missing from typed lane")
	}
	if champion.Value != "20" {
		t.Fatalf("champion frequency wrong: %+v", champion)
	}
	if !strings.Contains(strings.Join(champion.RichNotes, "\n"), "groups_total=12") {
		t.Fatalf("typed lane must carry the honest group total: %+v", champion.RichNotes)
	}
}
