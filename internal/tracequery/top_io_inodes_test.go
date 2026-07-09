package tracequery

// top_io_inodes_test.go — INODE (§28.6, 2026-07-09) engine pins for the
// whole-window (dev,inode) IO frequency carrier WindowStats.TopIOInodes:
//   - PID/op key dimensions collapse into ONE group per (dev,inode);
//   - additive dims (event counts, bytes) sum across threads, the latency
//     lane NEVER sums across threads (max single event + per-thread
//     within-thread sums only — the "quietly turn the max arm into a sum"
//     mutation dies here);
//   - Count-first ordering (frequency caliber) with Bytes/MaxLatency
//     tie-breaks;
//   - the fold consumes the FULL pre-truncation accumulator maps: a
//     count-champion group that the legacy top-8 latency-sorted carrier
//     truncates away still heads TopIOInodes;
//   - top-N truncation is disclosed via TotalGroups, never silent;
//   - inode-less events are counted as UnidentifiedEvents instead of being
//     folded into a fabricated pseudo-identity group.

import (
	"fmt"
	"strings"
	"testing"
)

func topIOInodeFoldFixtureIndex(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "top_io_inode_fold.systrace", `
      appA-100 (100) [000] .... 10.001000: android_fs_dataread_start: dev=259:1 ino=0xaa entry_name=hot.db offset=0 bytes=4096 rw=R
      appA-100 (100) [000] .... 10.001500: android_fs_dataread_end: dev=259:1 ino=0xaa bytes=4096 ret=4096 latency_us=4000 rw=R
      appA-100 (100) [000] .... 10.002000: android_fs_datawrite_start: dev=259:1 ino=0xaa offset=0 bytes=1024 rw=W
      appA-100 (100) [000] .... 10.002800: android_fs_datawrite_end: dev=259:1 ino=0xaa bytes=1024 ret=1024 latency_us=6000 rw=W
      appB-200 (200) [001] .... 10.003000: android_fs_dataread_start: dev=259:1 ino=0xaa offset=4096 bytes=2048 rw=R
      appB-200 (200) [001] .... 10.003700: android_fs_dataread_end: dev=259:1 ino=0xaa bytes=2048 ret=2048 latency_us=7000 rw=R
      appB-200 (200) [001] .... 10.004000: mm_filemap_add_to_page_cache: dev 259:1 ino 0xaa page=0000000000000000 pfn=1 ofs=0
      appC-300 (300) [002] .... 10.005000: android_fs_dataread_start: dev=259:1 ino=0xbb entry_name=cold.db offset=0 bytes=512 rw=RA
      appD-400 (400) [003] .... 10.006000: android_fs_dataread_start: dev=259:1 entry_name=noino.db offset=0 bytes=128 rw=R
	`)
}

// TestComputeTopIOInodes_FoldSemantics is the core fold pin: (dev,inode)
// identity (PID/op collapsed), additive counts/bytes, closed-set read/write
// split, latency max-not-sum, per-thread contributors, opportunistic entry
// name, and unidentified-event disclosure.
func TestComputeTopIOInodes_FoldSemantics(t *testing.T) {
	idx := topIOInodeFoldFixtureIndex(t)
	stats := ComputeWindowStats(idx, Query{TimeStart: 10.0, TimeEnd: 11.0})
	top := stats.TopIOInodes
	if top == nil {
		t.Fatal("TopIOInodes must publish when the window carries IO evidence")
	}
	if top.TotalGroups != 2 || len(top.Groups) != 2 {
		t.Fatalf("want 2 identified (dev,inode) groups, got total=%d rows=%d: %+v", top.TotalGroups, len(top.Groups), top.Groups)
	}
	if top.UnidentifiedEvents != 1 {
		t.Fatalf("the inode-less event must be disclosed as unidentified, not folded: %+v", top)
	}
	hot := top.Groups[0]
	if hot.Dev != "259:1" || hot.Inode != "0xaa" {
		t.Fatalf("count-first ordering must put the busy inode first: %+v", hot)
	}
	// Frequency caliber: 6 file-IO events (3 activity + 3 completions across
	// two PIDs and two ops) + 1 page-cache add = 7 total events.
	if hot.Count != 7 || hot.FileIOCount != 3 || hot.CompletionCount != 3 {
		t.Fatalf("cross-thread additive event counts wrong: %+v", hot)
	}
	if hot.ReadCount != 2 || hot.WriteCount != 1 {
		t.Fatalf("closed-set read/write split wrong: %+v", hot)
	}
	if hot.Bytes != 4096+1024+2048 {
		t.Fatalf("cross-thread additive bytes wrong: %+v", hot)
	}
	if hot.PageCacheAdds != 1 || hot.PageCacheDeletes != 0 || hot.PageCacheChurn != 1 {
		t.Fatalf("page-cache fold wrong: %+v", hot)
	}
	// RED LINE: latency is the largest SINGLE event (7ms from appB), never
	// the cross-thread sum (4+6+7=17ms). A sum arm makes this 17 and dies.
	if hot.MaxLatencyMs != 7 {
		t.Fatalf("MaxLatencyMs must be the max single member event (7ms), got %.3f — a cross-thread latency sum is forbidden", hot.MaxLatencyMs)
	}
	if hot.ThreadCount != 2 {
		t.Fatalf("distinct-thread census wrong: %+v", hot)
	}
	// Per-thread contributors: within-thread sums only — appA-100 summed its
	// OWN read(4ms)+write(6ms)=10ms, appB-200 has 7ms; ordered by per-thread
	// total.
	if len(hot.TopThreadLatencies) != 2 {
		t.Fatalf("want 2 per-thread latency contributors: %+v", hot.TopThreadLatencies)
	}
	if hot.TopThreadLatencies[0].Thread.PID != 100 || hot.TopThreadLatencies[0].TotalLatencyMs != 10 {
		t.Fatalf("appA within-thread sum must lead (10ms): %+v", hot.TopThreadLatencies)
	}
	if hot.TopThreadLatencies[1].Thread.PID != 200 || hot.TopThreadLatencies[1].TotalLatencyMs != 7 {
		t.Fatalf("appB within-thread sum wrong: %+v", hot.TopThreadLatencies)
	}
	if hot.EntryName != "hot.db" {
		t.Fatalf("opportunistic entry name must come from the earliest non-empty member label: %+v", hot)
	}
	cold := top.Groups[1]
	if cold.Inode != "0xbb" || cold.Count != 1 {
		t.Fatalf("second group wrong: %+v", cold)
	}
	// Open op domain: rw=RA normalizes outside the read/write closed set —
	// it counts toward the total only, never guessed into a bucket.
	if cold.ReadCount != 0 || cold.WriteCount != 0 || cold.FileIOCount != 1 {
		t.Fatalf("out-of-closed-set op must stay in the total only: %+v", cold)
	}
	if cold.EntryName != "cold.db" {
		t.Fatalf("cold entry name wrong: %+v", cold)
	}
}

// TestComputeTopIOInodes_FullMapInputBeyondLegacyTruncation builds 12
// (dev,inode) groups where the COUNT champion carries zero latency/low bytes
// — the legacy latency-sorted top-8 file_io_by_inode carrier truncates it
// away entirely, while TopIOInodes (fed the FULL pre-truncation maps) still
// ranks it first and discloses all 12 groups. This is the anti-regression pin
// for "built on truncated inputs" (§28.6 block_io_by_inode lesson).
func TestComputeTopIOInodes_FullMapInputBeyondLegacyTruncation(t *testing.T) {
	var b strings.Builder
	ts := 10.0
	// 11 low-count high-latency groups: 1 start + 1 end (latency 5ms) each.
	for i := 1; i <= 11; i++ {
		ino := fmt.Sprintf("0x%02x", i)
		fmt.Fprintf(&b, "      app-%d (%d) [000] .... %.6f: android_fs_dataread_start: dev=259:1 ino=%s offset=0 bytes=4096 rw=R\n", 100+i, 100+i, ts, ino)
		ts += 0.0001
		fmt.Fprintf(&b, "      app-%d (%d) [000] .... %.6f: android_fs_dataread_end: dev=259:1 ino=%s bytes=4096 ret=4096 latency_us=5000 rw=R\n", 100+i, 100+i, ts, ino)
		ts += 0.0001
	}
	// The count champion: 20 starts, no latency, tiny bytes.
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "      hotapp-500 (500) [001] .... %.6f: android_fs_dataread_start: dev=259:1 ino=0xcc offset=0 bytes=16 rw=R\n", ts)
		ts += 0.0001
	}
	idx := buildTraceIndex(t, "top_io_inode_fullmap.systrace", b.String())
	stats := ComputeWindowStats(idx, Query{TimeStart: 10.0, TimeEnd: 11.0})
	if len(stats.FileIOByInode) != 8 {
		t.Fatalf("legacy carrier must stay top-8 truncated (fixture invariant), got %d", len(stats.FileIOByInode))
	}
	for _, legacy := range stats.FileIOByInode {
		if legacy.Inode == "0xcc" {
			t.Fatalf("fixture must keep the count champion OUT of the legacy latency-sorted top-8 (otherwise this pin proves nothing): %+v", stats.FileIOByInode)
		}
	}
	top := stats.TopIOInodes
	if top == nil {
		t.Fatal("TopIOInodes missing")
	}
	if top.TotalGroups != 12 {
		t.Fatalf("TotalGroups must disclose every folded group (12), got %d", top.TotalGroups)
	}
	if len(top.Groups) != topIOInodeGroupLimit {
		t.Fatalf("published rows must cap at the group limit %d, got %d", topIOInodeGroupLimit, len(top.Groups))
	}
	if top.Groups[0].Inode != "0xcc" || top.Groups[0].Count != 20 {
		t.Fatalf("the count champion the legacy carrier truncated away must head TopIOInodes: %+v", top.Groups[0])
	}
}

// TestComputeTopIOInodes_CountPriorityOrdering pins the three-level sort:
// Count first (frequency caliber — the byte- or latency-first mutation dies
// on group Y), then Bytes, then MaxLatency.
func TestComputeTopIOInodes_CountPriorityOrdering(t *testing.T) {
	idx := buildTraceIndex(t, "top_io_inode_order.systrace", `
      app-100 (100) [000] .... 10.001000: android_fs_dataread_start: dev=259:1 ino=0x0x offset=0 bytes=5 rw=R
      app-100 (100) [000] .... 10.001100: android_fs_dataread_start: dev=259:1 ino=0x0x offset=5 bytes=5 rw=R
      app-100 (100) [000] .... 10.001200: android_fs_dataread_start: dev=259:1 ino=0x0z offset=0 bytes=2 rw=R
      app-100 (100) [000] .... 10.001300: android_fs_dataread_start: dev=259:1 ino=0x0z offset=2 bytes=2 rw=R
      app-100 (100) [000] .... 10.002000: android_fs_dataread_start: dev=259:1 ino=0x0y offset=0 bytes=1048576 rw=R
      app-100 (100) [000] .... 10.002500: android_fs_dataread_end: dev=259:1 ino=0x0y bytes=0 ret=1048576 latency_us=90000 rw=R
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 10.0, TimeEnd: 11.0})
	top := stats.TopIOInodes
	if top == nil || len(top.Groups) != 3 {
		t.Fatalf("want 3 groups: %+v", top)
	}
	// X (2 events, 10 bytes) and Z (2 events, 4 bytes) beat Y (2 events file
	// activity=1+completion=1 ... Y also has 2 events but 1MiB bytes).
	// Count tie across all three (2 events each) → Bytes breaks: Y(1MiB) >
	// X(10) > Z(4).
	if top.Groups[0].Inode != "0x0y" || top.Groups[1].Inode != "0x0x" || top.Groups[2].Inode != "0x0z" {
		t.Fatalf("count-then-bytes ordering wrong: %+v", top.Groups)
	}
	// Now the count dimension must dominate: X gains 2 more events and must
	// overtake Y despite Y's byte/latency lead.
	idx2 := buildTraceIndex(t, "top_io_inode_order2.systrace", `
      app-100 (100) [000] .... 10.001000: android_fs_dataread_start: dev=259:1 ino=0x0x offset=0 bytes=5 rw=R
      app-100 (100) [000] .... 10.001100: android_fs_dataread_start: dev=259:1 ino=0x0x offset=5 bytes=5 rw=R
      app-100 (100) [000] .... 10.001150: android_fs_dataread_start: dev=259:1 ino=0x0x offset=10 bytes=5 rw=R
      app-100 (100) [000] .... 10.002000: android_fs_dataread_start: dev=259:1 ino=0x0y offset=0 bytes=1048576 rw=R
      app-100 (100) [000] .... 10.002500: android_fs_dataread_end: dev=259:1 ino=0x0y bytes=0 ret=1048576 latency_us=90000 rw=R
	`)
	stats2 := ComputeWindowStats(idx2, Query{TimeStart: 10.0, TimeEnd: 11.0})
	top2 := stats2.TopIOInodes
	if top2 == nil || len(top2.Groups) != 2 {
		t.Fatalf("want 2 groups: %+v", top2)
	}
	if top2.Groups[0].Inode != "0x0x" || top2.Groups[0].Count != 3 {
		t.Fatalf("Count must outrank Bytes/MaxLatency (frequency caliber): %+v", top2.Groups)
	}
}

// TestComputeTopIOInodes_PageCacheOnlyInodeRanksByChurn pins the mm_filemap
// lane (§28.6 ⑤ — Harmony-shaped traces surface FS activity only through
// page-cache rows): an inode with ONLY page-cache adds/deletes still ranks by
// its real event frequency.
func TestComputeTopIOInodes_PageCacheOnlyInodeRanksByChurn(t *testing.T) {
	idx := buildTraceIndex(t, "top_io_inode_pagecache.systrace", `
      OS_FFRT_0-100 (100) [000] .... 10.001000: mm_filemap_add_to_page_cache: dev 260:84 ino 0xpc page=0000000000000000 pfn=1 ofs=0
      OS_FFRT_0-100 (100) [000] .... 10.001100: mm_filemap_add_to_page_cache: dev 260:84 ino 0xpc page=0000000000000000 pfn=2 ofs=4096
      OS_FFRT_1-101 (101) [001] .... 10.001200: mm_filemap_add_to_page_cache: dev 260:84 ino 0xpc page=0000000000000000 pfn=3 ofs=8192
      OS_FFRT_1-101 (101) [001] .... 10.001300: mm_filemap_delete_from_page_cache: dev 260:84 ino 0xpc page=0000000000000000 pfn=3 ofs=8192
      app-200 (200) [002] .... 10.002000: android_fs_dataread_start: dev=259:1 ino=0xfa entry_name=small.db offset=0 bytes=64 rw=R
      app-200 (200) [002] .... 10.002100: android_fs_dataread_end: dev=259:1 ino=0xfa bytes=64 ret=64 latency_us=100 rw=R
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 10.0, TimeEnd: 11.0})
	top := stats.TopIOInodes
	if top == nil || len(top.Groups) != 2 {
		t.Fatalf("want 2 groups: %+v", top)
	}
	pc := top.Groups[0]
	if pc.Inode != "0xpc" || pc.Count != 4 {
		t.Fatalf("page-cache-only inode must rank first by its 4 events: %+v", top.Groups)
	}
	if pc.PageCacheAdds != 3 || pc.PageCacheDeletes != 1 || pc.PageCacheChurn != 4 {
		t.Fatalf("page-cache decomposition wrong: %+v", pc)
	}
	if pc.FileIOCount != 0 || pc.ReadCount != 0 || pc.WriteCount != 0 {
		t.Fatalf("page-cache-only group must not fabricate file-IO counts: %+v", pc)
	}
	if pc.ThreadCount != 2 {
		t.Fatalf("distinct threads across page-cache members wrong: %+v", pc)
	}
	// Latency lane stays empty — page-cache rows carry no latency and no
	// contributor row may be invented.
	if pc.MaxLatencyMs != 0 || len(pc.TopThreadLatencies) != 0 {
		t.Fatalf("page-cache-only group must not invent latency: %+v", pc)
	}
}

// TestComputeTopIOInodes_NilWithoutIOEvidence pins the omission form: a
// window with no IO-family events publishes no TopIOInodes carrier at all.
func TestComputeTopIOInodes_NilWithoutIOEvidence(t *testing.T) {
	idx := buildTraceIndex(t, "top_io_inode_none.systrace", `
        app-100 (100) [000] .... 10.001000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
        app-100 (100) [000] .... 10.002000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 10.0, TimeEnd: 11.0})
	if stats.TopIOInodes != nil {
		t.Fatalf("no IO evidence must publish no carrier: %+v", stats.TopIOInodes)
	}
	if computeTopIOInodes(nil, nil, topIOInodeGroupLimit) != nil {
		t.Fatal("empty maps must fold to nil")
	}
}

// TestComputeTopIOInodes_DeterministicAcrossRuns re-folds the same fixture
// repeatedly: group order, entry-name donation, and contributor rosters must
// not depend on map iteration order.
func TestComputeTopIOInodes_DeterministicAcrossRuns(t *testing.T) {
	idx := topIOInodeFoldFixtureIndex(t)
	baseline := ComputeWindowStats(idx, Query{TimeStart: 10.0, TimeEnd: 11.0}).TopIOInodes
	for run := 0; run < 5; run++ {
		got := ComputeWindowStats(idx, Query{TimeStart: 10.0, TimeEnd: 11.0}).TopIOInodes
		if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", baseline) {
			t.Fatalf("fold not deterministic:\nrun %d: %+v\nbase:  %+v", run, got, baseline)
		}
	}
}
