package tracequery

import (
	"strings"
	"testing"
)

func TestF2FSClosedEndpointNameCandidateScope(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"f2fs_sync_file_enter", "f2fs_sync_file_exit",
		"f2fs_direct_IO_enter", "f2fs_direct_IO_exit",
		"f2fs_write_begin", "f2fs_write_end",
		" F2FS_SYNC_FILE_ENTER ", "f2fs_direct_io_exit_suffix",
		"f2fs_syncfileenter", "F2FS_SYNCFILEEXIT_suffix",
		"f2fs_directIOenter", "F2FS_DIRECTIOCOMPLETE_suffix",
		"f2fs_writebegin", "F2FS_WRITEEND_suffix",
		"f2fs_write_start", "f2fs_write_done_suffix",
	} {
		if !F2FSClosedEndpointNameCandidate(name) {
			t.Errorf("closed F2FS hard-family name escaped the negative gate: %q", name)
		}
	}
	for _, name := range []string{
		"f2fs_dataread_start", "f2fs_dataread_end",
		"f2fs_datawrite_start", "f2fs_datawrite_end",
		"f2fs_readpage", "f2fs_readpages", "f2fs_writepages",
		"f2fs_submit_read_bio", "hmfs_write_begin", "ext4_write_begin",
	} {
		if F2FSClosedEndpointNameCandidate(name) {
			t.Errorf("legitimate non-hard F2FS observation entered the closed negative gate: %q", name)
		}
	}
}

func TestF2FSDataReadRetainsSoftEvidenceWithoutElapsedAuthority(t *testing.T) {
	t.Parallel()
	idx := buildTraceIndex(t, "f2fs-dataread-soft.systrace",
		"app-40 (40) [003] .... 1.000000: f2fs_dataread_start: dev=8:0 ino=0x9 offset=0 bytes=4096\n"+
			"app-40 (40) [003] .... 1.001000: f2fs_dataread_end: dev=8:0 ino=0x9 offset=0 bytes=4096 ret=4096\n")
	q := Query{TimeStart: .9, TimeEnd: 1.1, Limit: 20}
	stats := ComputeWindowStats(idx, q)
	for _, ev := range idx.Events {
		if isStorageLatencyEvent(ev) {
			t.Fatalf("dataread entered the storage-latency soft classifier: %+v", ev)
		}
		if _, _, endpoint := genericStorageEndpoint(ev); endpoint {
			t.Fatalf("dataread entered the generic elapsed pairing registry: %+v", ev)
		}
		if verdict := DecodePairingEndpoint(ev.Name, ev.FieldText, int64(ev.PID)); verdict.Recognized || verdict.PayloadAdmitted {
			t.Fatalf("dataread entered the hard endpoint authority: %+v", verdict)
		}
	}

	if len(stats.FileIOByInode) != 1 {
		t.Fatalf("dataread lost its inode IO aggregate: %+v", stats.FileIOByInode)
	}
	file := stats.FileIOByInode[0]
	if file.Operation != "read" || file.Count != 1 || file.CompletionCount != 1 || file.Bytes != 4096 {
		t.Fatalf("dataread soft request accounting drifted: %+v", file)
	}
	if len(stats.FilesystemResources) != 1 || stats.FilesystemResources[0].Operation != "read" ||
		stats.FilesystemResources[0].Count != 2 || stats.FilesystemResources[0].Bytes != 8192 {
		t.Fatalf("dataread lost filesystem resource evidence: %+v", stats.FilesystemResources)
	}
	if len(stats.SubsystemEvents) != 1 || !strings.Contains(stats.SubsystemEvents[0].Kind, "f2fs") || stats.SubsystemEvents[0].Count != 2 {
		t.Fatalf("dataread lost subsystem evidence: %+v", stats.SubsystemEvents)
	}

	assertF2FSSearchCount(t, idx, q, nil, 2)
	assertF2FSSearchCount(t, idx, q, []EventType{EventFilesystem}, 2)
	assertF2FSSearchCount(t, idx, q, []EventType{"f2fs"}, 2)
	fileViews := assertF2FSSearchCount(t, idx, q, []EventType{"file_io"}, 2)
	for _, view := range fileViews {
		if view.FileFields == nil || view.FileFields.RW != "read" {
			t.Fatalf("dataread file_io EventView lost typed read operation: %+v", view)
		}
	}
	if !hasEvidencePredicate(evidenceFromStats(stats), "file_io_by_inode") ||
		countEvidencePredicate(evidenceFromEvents(fileViews), string(EventFilesystem)) != 2 {
		t.Fatalf("dataread lost stats/event evidence facts: stats=%+v events=%+v", evidenceFromStats(stats), evidenceFromEvents(fileViews))
	}

	if len(stats.StorageLatencyByLayer) != 0 || len(stats.IOBurstEpisodes) != 0 {
		t.Fatalf("dataread acquired unauthorized elapsed authority: storage=%+v bursts=%+v", stats.StorageLatencyByLayer, stats.IOBurstEpisodes)
	}
	rank := BuildRootCauseRank(idx, q)
	if !hasRootCauseType(rank, "file_io_hot_inode") {
		t.Fatalf("dataread soft file IO evidence did not reach its established advisory rank: %+v", rank.Items)
	}
	for _, forbidden := range []string{"io_latency", "io_burst_episode"} {
		if hasRootCauseType(rank, forbidden) {
			t.Fatalf("dataread minted unauthorized elapsed rank %q: %+v", forbidden, rank.Items)
		}
	}
}

func TestF2FSDataWriteAndReadPageSoftEvidenceDoesNotRegress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		trace      string
		operation  string
		count      int
		completion int
		bytes      int64
	}{
		{
			name: "datawrite",
			trace: "app-40 (40) [003] .... 1.000000: f2fs_datawrite_start: dev=8:0 ino=0xa offset=0 bytes=4096\n" +
				"app-40 (40) [003] .... 1.001000: f2fs_datawrite_end: dev=8:0 ino=0xa offset=0 bytes=4096 ret=4096\n",
			operation: "write", count: 1, completion: 1, bytes: 4096,
		},
		{
			name: "readpage families",
			trace: "app-40 (40) [003] .... 1.000000: f2fs_readpage: dev=8:0 ino=0xb offset=0 bytes=4096 rw=read\n" +
				"app-40 (40) [003] .... 1.001000: f2fs_readpages: dev=8:0 ino=0xb offset=4096 bytes=4096 rw=read\n",
			operation: "read", count: 2, bytes: 8192,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idx := buildTraceIndex(t, "f2fs-other-soft.systrace", tc.trace)
			q := Query{TimeStart: .9, TimeEnd: 1.1, Limit: 20}
			stats := ComputeWindowStats(idx, q)
			if len(stats.FileIOByInode) != 1 {
				t.Fatalf("soft F2FS observations lost inode aggregate: %+v", stats.FileIOByInode)
			}
			file := stats.FileIOByInode[0]
			if file.Operation != tc.operation || file.Count != tc.count || file.CompletionCount != tc.completion || file.Bytes != tc.bytes {
				t.Fatalf("soft F2FS request accounting drifted: %+v", file)
			}
			if len(stats.FilesystemResources) == 0 || len(stats.SubsystemEvents) == 0 ||
				!hasEvidencePredicate(evidenceFromStats(stats), "file_io_by_inode") {
				t.Fatalf("soft F2FS evidence carriers regressed: resources=%+v subsystem=%+v evidence=%+v",
					stats.FilesystemResources, stats.SubsystemEvents, evidenceFromStats(stats))
			}
			assertF2FSSearchCount(t, idx, q, []EventType{"file_io"}, 2)
			if len(stats.StorageLatencyByLayer) != 0 || len(stats.IOBurstEpisodes) != 0 {
				t.Fatalf("soft F2FS observation acquired elapsed authority: storage=%+v bursts=%+v", stats.StorageLatencyByLayer, stats.IOBurstEpisodes)
			}
		})
	}
}

func TestF2FSCompactHardFamilyNearNamesRemainInventoryOnly(t *testing.T) {
	t.Parallel()
	rows := []string{
		"f2fs_syncfileenter: " + f2fsSyncEnterBody,
		"F2FS_SYNCFILEEXIT_suffix: " + f2fsSyncExitBody,
		"f2fs_directIOenter: " + f2fsDIOEnter510Body,
		"F2FS_DIRECTIOCOMPLETE_suffix: " + f2fsDIOExitBody,
		"f2fs_writebegin: " + f2fsWriteBegin66Body,
		"F2FS_WRITEEND_suffix: " + f2fsWriteEndBody,
	}
	var trace strings.Builder
	for i, row := range rows {
		trace.WriteString("io-40 (40) [003] .... ")
		trace.WriteString("1.00")
		trace.WriteByte(byte('0' + i))
		trace.WriteString("000: ")
		trace.WriteString(row)
		trace.WriteByte('\n')
	}
	idx := buildTraceIndex(t, "f2fs-compact-near-inventory.systrace", trace.String())
	assertF2FSInventoryOnly(t, idx, len(rows))
	q := Query{TimeStart: .9, TimeEnd: 1.1, Limit: 20}
	assertF2FSSearchCount(t, idx, q, nil, len(rows))
	assertF2FSSearchCount(t, idx, q, []EventType{EventFilesystem}, len(rows))
	assertF2FSSearchCount(t, idx, q, []EventType{"f2fs"}, len(rows))
	assertF2FSSearchCount(t, idx, q, []EventType{"file_io"}, 0)
}

func assertF2FSSearchCount(t *testing.T, idx *Index, q Query, eventTypes []EventType, want int) []EventView {
	t.Helper()
	q.EventTypes = eventTypes
	views := EventSearch(idx, q)
	if len(views) != want {
		t.Fatalf("F2FS EventView search parity drifted for types=%v: got=%d want=%d views=%+v", eventTypes, len(views), want, views)
	}
	return views
}

func hasEvidencePredicate(facts []EvidenceFact, predicate string) bool {
	return countEvidencePredicate(facts, predicate) > 0
}

func countEvidencePredicate(facts []EvidenceFact, predicate string) int {
	count := 0
	for _, fact := range facts {
		if fact.Predicate == predicate {
			count++
		}
	}
	return count
}

func hasRootCauseType(rank RootCauseRankResult, typ string) bool {
	for _, item := range rank.Items {
		if item.Type == typ {
			return true
		}
	}
	return false
}
