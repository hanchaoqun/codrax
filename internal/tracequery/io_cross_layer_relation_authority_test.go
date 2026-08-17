package tracequery

import "testing"

func TestIOCrossLayerRelationRejectsNearestThreadTimeJoin(t *testing.T) {
	thread := ThreadRef{Comm: "issuer", PID: 42, TGID: 42}
	stats := WindowStats{
		FileIOByInode: []FileIOSummary{{
			Dev: "12:80", Inode: "0xabc", Thread: thread, Operation: "read",
			Bytes: 4096, StartTs: 10.000, EndTs: 10.100, LineStart: 10, LineEnd: 20,
		}},
		StorageLatencyByLayer: []StorageLatencySummary{{
			Dev: "12:80", Thread: thread, Operation: "read", MaxLatencyMs: 7,
			StartTs: 10.010, EndTs: 10.017, LineStart: 12, LineEnd: 13,
		}},
		IOLatencies: []IOLatencySummary{{
			Dev: "12:80", IssueThread: thread, DurationMs: 9,
			IssueTs: 10.011, CompleteTs: 10.020, IssueLine: 14, CompleteLine: 15,
		}},
	}

	rows := computeBlockIOByInode(stats, 8)
	if len(rows) != 1 {
		t.Fatalf("inode-local inventory should remain visible: %+v", rows)
	}
	row := rows[0]
	if row.RelationStatus != blockIOInodeRelationLocalActivity || row.BlockMaxLatencyMs != 0 || row.StorageMaxLatencyMs != 0 || row.NearestBlockThread.PID != 0 {
		t.Fatalf("inode-less storage and block requests must not attach by temporal proximity: %+v", row)
	}
	if episodes := computeIOBurstEpisodes(WindowStats{BlockIOByInode: rows}, 8); len(episodes) != 0 {
		t.Fatalf("inode-local activity without an exact latency relation must not mint a latency burst: %+v", episodes)
	}
}

func TestIOCrossLayerRelationAdmitsProducerOwnedStorageIdentity(t *testing.T) {
	thread := ThreadRef{Comm: "worker", PID: 77, TGID: 77}
	stats := WindowStats{
		FileIOByInode: []FileIOSummary{{
			Dev: "12:80", Inode: "0xdef", EntryName: "cache.db", Thread: thread,
			Operation: "read", Bytes: 8192, StartTs: 20.000, EndTs: 20.030, LineStart: 30, LineEnd: 40,
		}},
		StorageLatencyByLayer: []StorageLatencySummary{{
			Dev: "12,80", Inode: "0xdef", EntryName: "cache.db", Thread: thread,
			Operation: "read", MaxLatencyMs: 11, StartTs: 20.005, EndTs: 20.016, LineStart: 32, LineEnd: 35,
		}},
	}

	rows := computeBlockIOByInode(stats, 8)
	if len(rows) != 1 || rows[0].RelationStatus != blockIOInodeRelationExactStorageIdentity || rows[0].StorageMaxLatencyMs != 11 {
		t.Fatalf("producer-owned storage inode identity should join exactly: %+v", rows)
	}
	episodes := computeIOBurstEpisodes(WindowStats{BlockIOByInode: rows}, 8)
	if len(episodes) != 1 || episodes[0].RootCauseEligibility != ioBurstRootCauseExactChainHostWork || episodes[0].DurationMs != 11 {
		t.Fatalf("exact paired inode/storage work should carry the typed host-work eligibility: %+v", episodes)
	}
}
