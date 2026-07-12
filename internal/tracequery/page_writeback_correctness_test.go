package tracequery

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parsePageWritebackTestEvent(t *testing.T, line int, name, fields string) Event {
	t.Helper()
	raw := fmt.Sprintf("      task-42 (42) [001] .... %d.001000: %s: %s", line, name, fields)
	ev, ok := ParseLine(line, raw, newStringInterner())
	if !ok {
		t.Fatalf("ParseLine rejected inventory row %q", raw)
	}
	return ev
}

func TestPageCacheMutationTypedAdmissionProfiles(t *testing.T) {
	tests := []struct {
		name   string
		event  string
		fields string
		kind   pageCacheMutationKind
		offset int64
	}{
		{
			name:   "openharmony linux 5.10 pointer",
			event:  pageCacheAddEventName,
			fields: "dev 260:132 ino 0x259ff page=0xffffff8076e3ef40 pfn=2686717 ofs=109387776",
			kind:   pageCacheMutationAdd,
			offset: 109387776,
		},
		{
			name:   "legacy redacted pointer",
			event:  pageCacheDeleteEventName,
			fields: "dev 260:132 ino 0x259ff page=0000000000000000 pfn=2686717 ofs=4096",
			kind:   pageCacheMutationDelete,
			offset: 4096,
		},
		{
			name:   "legacy short zero pointer",
			event:  pageCacheAddEventName,
			fields: "dev 260:132 ino 0x259ff page=0 pfn=1 ofs=4096",
			kind:   pageCacheMutationAdd,
			offset: 4096,
		},
		{
			name:   "legacy short prefixed zero pointer",
			event:  pageCacheDeleteEventName,
			fields: "dev 260:132 ino 0x259ff page=0x0 pfn=1 ofs=4096",
			kind:   pageCacheMutationDelete,
			offset: 4096,
		},
		{
			name:   "32 bit pointer without prefix",
			event:  pageCacheAddEventName,
			fields: "dev 260:132 ino 0x259ff page=deadbeef pfn=1 ofs=4096",
			kind:   pageCacheMutationAdd,
			offset: 4096,
		},
		{
			name:   "32 bit pointer with prefix",
			event:  pageCacheDeleteEventName,
			fields: "dev 260:132 ino 0x259ff page=0xdeadbeef pfn=1 ofs=4096",
			kind:   pageCacheMutationDelete,
			offset: 4096,
		},
		{
			name:   "64 bit pointer without prefix",
			event:  pageCacheAddEventName,
			fields: "dev 260:132 ino 0x259ff page=ffffff8076e3ef40 pfn=1 ofs=4096",
			kind:   pageCacheMutationAdd,
			offset: 4096,
		},
		{
			name:   "kernel ptrval sentinel",
			event:  pageCacheAddEventName,
			fields: "dev 260:132 ino 0x259ff page=(____ptrval____) pfn=2686717 ofs=4096",
			kind:   pageCacheMutationAdd,
			offset: 4096,
		},
		{
			name:   "codrax honest no pointer",
			event:  pageCacheAddEventName,
			fields: "dev 12:48 ino 0x1234 pfn=77 ofs=0",
			kind:   pageCacheMutationAdd,
			offset: 0,
		},
		{
			name:   "linux 6.6 order zero",
			event:  pageCacheDeleteEventName,
			fields: "dev 12:48 ino 1234 pfn=0x4d ofs=8192 order=0",
			kind:   pageCacheMutationDelete,
			offset: 8192,
		},
		{
			name:   "linux 6.6 order max",
			event:  pageCacheAddEventName,
			fields: "dev 4095:1048575 ino 0xffffffffffffffff pfn=18446744073709551615 ofs=9223372036854771712 order=255",
			kind:   pageCacheMutationAdd,
			offset: math.MaxInt64 - 4095,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := parsePageWritebackTestEvent(t, i+1, tt.event, tt.fields)
			if ev.Type != EventMemory || ev.MemoryKind != "page_cache" || ev.SubsystemKind != "page_cache" {
				t.Fatalf("valid mutation lost memory inventory classification: %+v", ev)
			}
			if got := pageCacheMutationKindForEvent(ev); got != tt.kind || !isPageCacheEvent(ev) {
				t.Fatalf("typed mutation mismatch: kind=%v event=%+v", got, ev)
			}
			if ev.FileFields == nil || ev.FileFields.Dev == "" || ev.FileFields.Ino == "" || ev.FileFields.Offset != tt.offset {
				t.Fatalf("strict page tuple projection mismatch: %+v", ev.FileFields)
			}
			if ev.ResourceFields == nil || ev.ResourceFields.Path != "" || ev.ResourceFields.Bytes != 0 {
				t.Fatalf("page tuple leaked into generic resource projection: %+v", ev.ResourceFields)
			}
		})
	}
}

func TestPageCacheMutationMalformedExactRowsFailClosedLocally(t *testing.T) {
	valid := "dev 260:132 ino 0x259ff page=0xffffff8076e3ef40 pfn=2686717 ofs=4096"
	tests := []struct {
		name   string
		fields string
	}{
		{name: "missing dev", fields: "ino 0x259ff pfn=1 ofs=0"},
		{name: "dev equals alias", fields: "dev=260:132 ino 0x259ff pfn=1 ofs=0"},
		{name: "major overflow", fields: "dev 4096:1 ino 0x259ff pfn=1 ofs=0"},
		{name: "minor overflow", fields: "dev 1:1048576 ino 0x259ff pfn=1 ofs=0"},
		{name: "dev sign", fields: "dev -1:1 ino 0x259ff pfn=1 ofs=0"},
		{name: "inode invalid separator", fields: "dev 1:1 ino 0x1_2 pfn=1 ofs=0"},
		{name: "inode malformed", fields: "dev 1:1 ino 0xzz pfn=1 ofs=0"},
		{name: "inode overflow", fields: "dev 1:1 ino 0x10000000000000000 pfn=1 ofs=0"},
		{name: "pfn sign", fields: "dev 1:1 ino 0x1 pfn=-1 ofs=0"},
		{name: "pfn overflow", fields: "dev 1:1 ino 0x1 pfn=18446744073709551616 ofs=0"},
		{name: "offset sign", fields: "dev 1:1 ino 0x1 pfn=1 ofs=-4096"},
		{name: "offset overflow", fields: "dev 1:1 ino 0x1 pfn=1 ofs=9223372036854775808"},
		{name: "offset not page aligned", fields: "dev 1:1 ino 0x1 pfn=1 ofs=1"},
		{name: "order overflow", fields: "dev 1:1 ino 0x1 pfn=1 ofs=0 order=256"},
		{name: "order sign", fields: "dev 1:1 ino 0x1 pfn=1 ofs=0 order=-1"},
		{name: "page malformed", fields: "dev 1:1 ino 0x1 page=(null) pfn=1 ofs=0"},
		{name: "page one digit", fields: "dev 1:1 ino 0x1 page=1 pfn=1 ofs=0"},
		{name: "page seven digits", fields: "dev 1:1 ino 0x1 page=1234567 pfn=1 ofs=0"},
		{name: "page nine digits", fields: "dev 1:1 ino 0x1 page=123456789 pfn=1 ofs=0"},
		{name: "page prefixed seven digits", fields: "dev 1:1 ino 0x1 page=0x1234567 pfn=1 ofs=0"},
		{name: "page prefixed nine digits", fields: "dev 1:1 ino 0x1 page=0x123456789 pfn=1 ofs=0"},
		{name: "page seventeen digits", fields: "dev 1:1 ino 0x1 page=0123456789abcdef0 pfn=1 ofs=0"},
		{name: "unattested page plus order", fields: "dev 1:1 ino 0x1 page=0 pfn=1 ofs=0 order=1"},
		{name: "duplicate tuple", fields: valid + " pfn=2686717"},
		{name: "bytes injection", fields: valid + " bytes=4096"},
		{name: "entry injection", fields: valid + " entry_name=secret"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := parsePageWritebackTestEvent(t, i+1, pageCacheAddEventName, tt.fields)
			if ev.Type != EventMemory {
				t.Fatalf("malformed exact row must remain searchable inventory: %+v", ev)
			}
			if got := pageCacheMutationKindForEvent(ev); got != pageCacheMutationNone || isPageCacheEvent(ev) {
				t.Fatalf("malformed tuple minted mutation kind %v: %+v", got, ev)
			}
			if ev.FileFields == nil || ev.FileFields.Dev != "" || ev.FileFields.Ino != "" || ev.FileFields.Offset != 0 ||
				ev.ResourceFields == nil || ev.ResourceFields.Path != "" || ev.ResourceFields.Bytes != 0 {
				t.Fatalf("malformed tuple published partial/generic fields: file=%+v resource=%+v", ev.FileFields, ev.ResourceFields)
			}
		})
	}

	good := parsePageWritebackTestEvent(t, len(tests)+1, pageCacheDeleteEventName,
		"dev 260:132 ino 0x259ff pfn=2686717 ofs=4096")
	stats := ComputeWindowStats(&Index{Events: append([]Event{
		parsePageWritebackTestEvent(t, len(tests)+2, pageCacheAddEventName, "dev 1:1 ino xyz pfn=1 ofs=0"),
	}, good)}, Query{TimeStart: 0, TimeEnd: 100, Limit: 20})
	if len(stats.PageCacheByInode) != 1 || stats.PageCacheByInode[0].Deletes != 1 || stats.PageCacheByInode[0].Adds != 0 {
		t.Fatalf("bad sibling poisoned or joined the valid mutation: %+v", stats.PageCacheByInode)
	}
}

func TestPageCacheNearFaultAndCaseDriftStayInventoryOnly(t *testing.T) {
	rows := []Event{
		parsePageWritebackTestEvent(t, 1, "mm_filemap_fault", "dev 1:1 ino 0x1 pfn=1 ofs=0"),
		parsePageWritebackTestEvent(t, 2, pageCacheAddEventName+"_vendor", "dev 1:1 ino 0x1 pfn=1 ofs=0"),
		parsePageWritebackTestEvent(t, 3, strings.ToUpper(pageCacheDeleteEventName), "dev 1:1 ino 0x1 pfn=1 ofs=0"),
	}
	for _, ev := range rows {
		if isPageCacheEvent(ev) || pageCacheMutationKindForEvent(ev) != pageCacheMutationNone {
			t.Fatalf("near/fault/case drift acquired page mutation: %+v", ev)
		}
	}
	idx := &Index{Events: rows}
	q := Query{TimeStart: 0, TimeEnd: 100, Limit: 20}
	stats := ComputeWindowStats(idx, q)
	if len(stats.PageCacheByInode) != 0 || stats.TopIOInodes != nil || stats.IOPressureSummary != nil || len(stats.IOBurstEpisodes) != 0 || len(stats.BlockIOByInode) != 0 {
		t.Fatalf("inventory-only filemap rows leaked into IO aggregates: page=%+v top=%+v pressure=%+v burst=%+v block=%+v",
			stats.PageCacheByInode, stats.TopIOInodes, stats.IOPressureSummary, stats.IOBurstEpisodes, stats.BlockIOByInode)
	}
	if len(stats.MemoryKinds) != 0 {
		t.Fatalf("unadmitted filemap rows regained evidence/rank through MemoryKind: %+v", stats.MemoryKinds)
	}
	for _, item := range BuildRootCauseRank(idx, q).Items {
		if strings.Contains(item.Type, "page") || strings.Contains(item.Type, "memory") || strings.Contains(item.Type, "io_") {
			t.Fatalf("inventory-only filemap row acquired root-cause seat: %+v", item)
		}
	}
}

func TestWritebackExactAdmissionDedicatedProjectionAndNegativeLanes(t *testing.T) {
	set := parsePageWritebackTestEvent(t, 1, writebackSetEventName,
		"dev=260:132 ino=0x259ff errseq=0xffffffff")
	advance := parsePageWritebackTestEvent(t, 2, writebackAdvanceEventName,
		"file=0xffffff8076e3ef40 dev=260:132 ino=0x259ff old=0x1 new=0x2")
	malformed := parsePageWritebackTestEvent(t, 3, writebackSetEventName,
		"dev=260:132 ino=0x259ff errseq=0x100000000 bytes=4096 path=/secret")
	for _, ev := range []Event{set, advance, malformed} {
		if ev.Type != EventFilesystem || ev.SubsystemKind != "writeback" || !isWritebackObservation(ev) {
			t.Fatalf("exact writeback observation classification mismatch: %+v", ev)
		}
		if ev.ResourceFields == nil || ev.ResourceFields.Path != "" || ev.ResourceFields.Op != "" || ev.ResourceFields.Bytes != 0 || ev.ResourceFields.LatencyMs != 0 || ev.ResourceFields.Address != "" {
			t.Fatalf("writeback pointer/scalars leaked into generic resources: %+v", ev.ResourceFields)
		}
		if ev.FileFields == nil || ev.FileFields.Entry != "" || ev.FileFields.Len != 0 || ev.FileFields.Offset != 0 || ev.FileFields.RW != "" {
			t.Fatalf("writeback pointer/scalars leaked into file IO: %+v", ev.FileFields)
		}
		if isFileIOEvent(ev) || isPageCacheEvent(ev) || isStorageLatencyEvent(ev) || runtimeResourceKind(ev) != "" {
			t.Fatalf("writeback observation entered a forbidden derived lane: %+v", ev)
		}
	}
	if set.FileFields.Dev != "260:132" || set.FileFields.Ino != "0x259ff" ||
		advance.FileFields.Dev != "260:132" || advance.FileFields.Ino != "0x259ff" {
		t.Fatalf("valid writeback dedicated dev/inode projection mismatch: set=%+v advance=%+v", set.FileFields, advance.FileFields)
	}
	if malformed.FileFields.Dev != "" || malformed.FileFields.Ino != "" {
		t.Fatalf("malformed writeback projected a partial tuple: %+v", malformed.FileFields)
	}

	near := []Event{
		parsePageWritebackTestEvent(t, 4, writebackSetEventName+"_start", "dev=260:132 ino=0x259ff errseq=0x1"),
		parsePageWritebackTestEvent(t, 5, writebackAdvanceEventName+"_end", "file=0x1 dev=260:132 ino=0x259ff old=0x1 new=0x2"),
		parsePageWritebackTestEvent(t, 6, strings.ToUpper(writebackSetEventName), "dev=260:132 ino=0x259ff errseq=0x1"),
	}
	for _, ev := range near {
		if ev.Type != EventUnknown || isWritebackObservation(ev) || isStorageLatencyEvent(ev) {
			t.Fatalf("writeback near/case name acquired filesystem/pair authority: %+v", ev)
		}
	}

	idx := &Index{Events: append([]Event{set, advance, malformed}, near...)}
	q := Query{TimeStart: 0, TimeEnd: 100, Limit: 20}
	stats := ComputeWindowStats(idx, q)
	if stats.FilesystemEventCount != 3 || stats.EventCounts[EventFilesystem] != 3 {
		t.Fatalf("exact writeback inventory count mismatch: %+v", stats.EventCounts)
	}
	if len(stats.SubsystemEvents) != 0 || len(stats.FilesystemResources) != 0 || len(stats.FileIOByInode) != 0 ||
		len(stats.PageCacheByInode) != 0 || stats.TopIOInodes != nil || stats.IOPressureSummary != nil ||
		len(stats.StorageLatencyByLayer) != 0 || len(stats.BlockIOByInode) != 0 || len(stats.IOBurstEpisodes) != 0 {
		t.Fatalf("writeback leaked into derived evidence/IO lanes: subsystem=%+v resources=%+v file=%+v page=%+v top=%+v pressure=%+v storage=%+v block=%+v burst=%+v",
			stats.SubsystemEvents, stats.FilesystemResources, stats.FileIOByInode, stats.PageCacheByInode, stats.TopIOInodes,
			stats.IOPressureSummary, stats.StorageLatencyByLayer, stats.BlockIOByInode, stats.IOBurstEpisodes)
	}
	if facts := evidenceFromStats(stats); len(facts) != 0 {
		t.Fatalf("writeback observation minted evidence facts: %+v", facts)
	}
	if rank := BuildRootCauseRank(idx, q); len(rank.Items) != 0 {
		t.Fatalf("writeback observation minted root-cause rank: %+v", rank.Items)
	}
	if got := EventSearch(idx, Query{EventTypes: []EventType{EventFilesystem}, TimeStart: 0, TimeEnd: 100, Limit: 20}); len(got) != 3 {
		t.Fatalf("exact writeback observations must remain searchable: %+v", got)
	}
}

func TestWritebackCanonicalScalarBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		event  string
		fields string
		valid  bool
	}{
		{name: "set max", event: writebackSetEventName, fields: "dev=4095:1048575 ino=0xffffffffffffffff errseq=0xffffffff", valid: true},
		{name: "set errseq overflow", event: writebackSetEventName, fields: "dev=1:1 ino=0x1 errseq=0x100000000"},
		{name: "set negative", event: writebackSetEventName, fields: "dev=1:1 ino=0x1 errseq=-1"},
		{name: "set alias injection", event: writebackSetEventName, fields: "dev=1:1 ino=0x1 errseq=0x1 offset=0"},
		{name: "advance 32 bit pointer", event: writebackAdvanceEventName, fields: "file=0xffffffff dev=1:1 ino=0x1 old=0x0 new=0xffffffff", valid: true},
		{name: "advance 64 bit pointer", event: writebackAdvanceEventName, fields: "file=0xffffffffffffffff dev=1:1 ino=0x1 old=0x0 new=0x1", valid: true},
		{name: "advance fixed width pointer", event: writebackAdvanceEventName, fields: "file=fffffffffffffffe dev=1:1 ino=0x1 old=0x0 new=0x1", valid: true},
		{name: "advance ptrval sentinel", event: writebackAdvanceEventName, fields: "file=(____ptrval____) dev=1:1 ino=0x1 old=0x0 new=0x1", valid: true},
		{name: "advance null pointer", event: writebackAdvanceEventName, fields: "file=0x0 dev=1:1 ino=0x1 old=0x0 new=0x1"},
		{name: "advance pointer overflow", event: writebackAdvanceEventName, fields: "file=0x10000000000000000 dev=1:1 ino=0x1 old=0x0 new=0x1"},
		{name: "advance old overflow", event: writebackAdvanceEventName, fields: "file=0x1 dev=1:1 ino=0x1 old=0x100000000 new=0x1"},
		{name: "advance duplicate", event: writebackAdvanceEventName, fields: "file=0x1 dev=1:1 ino=0x1 old=0x0 new=0x1 new=0x1"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := parsePageWritebackTestEvent(t, i+1, tt.event, tt.fields)
			projected := ev.FileFields != nil && ev.FileFields.Dev != "" && ev.FileFields.Ino != ""
			if projected != tt.valid {
				t.Fatalf("dedicated writeback projection=%v, want %v: %+v", projected, tt.valid, ev)
			}
		})
	}
}

func TestWritebackEventSearchStaysSearchableWithoutEvidenceOrVirtualIOLanes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "writeback-search.systrace")
	body := strings.Join([]string{
		"task-42 (42) [001] .... 1.001000: filemap_set_wb_err: dev=260:132 ino=0x259ff errseq=0x1",
		"task-42 (42) [001] .... 1.002000: file_check_and_advance_wb_err: file=0x1 dev=260:132 ino=0x259ff old=0x1 new=0x2",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", EventTypes: []EventType{EventFilesystem}, Limit: 20}
	indexed := Run(idx, q)
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	for label, result := range map[string]Result{"indexed": indexed, "streamed": streamed} {
		if len(result.Events) != 2 {
			t.Fatalf("%s exact filesystem search lost writeback observations: %+v", label, result.Events)
		}
		if len(result.EvidencePack) != 0 {
			t.Fatalf("%s writeback search minted generic evidence: %+v", label, result.EvidencePack)
		}
	}

	for _, virtualType := range []EventType{"file_io", "page_cache", "storage_latency", "io_pressure"} {
		query := Query{View: "event_search", EventTypes: []EventType{virtualType}, Limit: 20}
		indexed = Run(idx, query)
		streamed, err = StreamEventSearch(context.Background(), path, query)
		if err != nil {
			t.Fatal(err)
		}
		if len(indexed.Events) != 0 || len(streamed.Events) != 0 ||
			len(indexed.EvidencePack) != 0 || len(streamed.EvidencePack) != 0 {
			t.Fatalf("virtual %s filter admitted writeback: indexed=%+v streamed=%+v",
				virtualType, indexed, streamed)
		}
	}
}

func TestWarmDerivedWindowPreservesPageCacheMutationAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page-cache-warm-derived.systrace")
	body := strings.Join([]string{
		"task-42 (42) [001] .... 1.001000: mm_filemap_add_to_page_cache: dev 260:132 ino 0x259ff pfn=1 ofs=0",
		"task-42 (42) [001] .... 2.001000: mm_filemap_delete_from_page_cache: dev 260:132 ino 0x259ff pfn=1 ofs=0",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	full, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if stats := ComputeWindowStats(full, Query{}); len(stats.PageCacheByInode) != 1 ||
		stats.PageCacheByInode[0].Adds != 1 || stats.PageCacheByInode[0].Deletes != 1 {
		t.Fatalf("full index lost page mutation authority: %+v", stats.PageCacheByInode)
	}
	warm, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 0.9, TimeStartSet: true,
		TimeEnd: 1.1, TimeEndSet: true,
		AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	stats := ComputeWindowStats(warm, Query{TimeStart: 0.9, TimeEnd: 1.1})
	if !warm.Windowed || len(stats.PageCacheByInode) != 1 ||
		stats.PageCacheByInode[0].Adds != 1 || stats.PageCacheByInode[0].Deletes != 0 {
		t.Fatalf("warm full-cache derived window lost private mutation authority: windowed=%v page=%+v",
			warm.Windowed, stats.PageCacheByInode)
	}
}
