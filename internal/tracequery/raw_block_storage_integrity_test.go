package tracequery

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func rawPairingFailureForLine(idx *Index, family durationOrderFamily, line int) *durationOrderViolation {
	if idx == nil {
		return nil
	}
	for i := range idx.durationOrderFailures {
		failure := &idx.durationOrderFailures[i]
		if failure.Family == family && failure.Line == line {
			return failure
		}
	}
	return nil
}

func TestRawBlockStorageKnownLaneBarrierCannotBeBridged(t *testing.T) {
	t.Run("block exact lane keeps sibling", func(t *testing.T) {
		idx := buildTraceIndex(t, "raw-block-known-lane.systrace", strings.Join([]string{
			`io-30 (30) [001] .... 1.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 [io]`,
			`io-30 (30) [bad] .... 1.100000: block_rq_complete: 8,0 R () 32 + 8 [0]`,
			`io-30 (30) [001] .... 1.200000: block_rq_complete: 8,0 R () 32 + 8 [0]`,
			`io-30 (30) [001] .... 1.300000: block_rq_issue: 8,0 R 4096 () 64 + 8 [io]`,
			`io-30 (30) [001] .... 1.400000: block_rq_complete: 8,0 R () 64 + 8 [0]`,
		}, "\n")+"\n")
		failure := rawPairingFailureForLine(idx, durationOrderBlockIO, 2)
		if failure == nil || failure.LaneKey == "" || failure.SourcePath == "" || !containsString(failure.Fields, "header_cpu") {
			t.Fatalf("parser-rejected Block row did not mint an exact physical-source barrier: %+v", failure)
		}

		stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.5})
		if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].Sector != 64 || stats.IOLatencies[0].IssueLine != 4 || stats.IOLatencies[0].CompleteLine != 5 {
			t.Fatalf("Block bad-row hole was bridged or legal sibling lost: latencies=%+v caveats=%v", stats.IOLatencies, stats.Caveats)
		}
		if !containsSubstring(stats.Caveats, "duration_pairing_exact_lane_quarantined=true; family=block_io") {
			t.Fatalf("Block exact-lane quarantine was not disclosed: %v", stats.Caveats)
		}
	})

	t.Run("storage exact lane keeps sibling", func(t *testing.T) {
		idx := buildTraceIndex(t, "raw-storage-known-lane.systrace", strings.Join([]string{
			`io-40 (40) [001] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096`,
			`io-40 (40) [bad] .... 1.100000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096`,
			`io-40 (40) [001] .... 1.200000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096`,
			`io-40 (40) [001] .... 1.300000: scsi_dispatch_cmd_start: dev=13,0 op=read bytes=4096`,
			`io-40 (40) [001] .... 1.400000: scsi_dispatch_cmd_done: dev=13,0 op=read bytes=4096`,
		}, "\n")+"\n")
		failure := rawPairingFailureForLine(idx, durationOrderStorage, 2)
		if failure == nil || failure.LaneKey == "" || failure.SourcePath == "" || !containsString(failure.Fields, "header_cpu") {
			t.Fatalf("parser-rejected Storage row did not mint an exact physical-source barrier: %+v", failure)
		}

		stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.5})
		var scsiRows []StorageLatencySummary
		for _, row := range stats.StorageLatencyByLayer {
			if row.Layer == "scsi" {
				scsiRows = append(scsiRows, row)
			}
		}
		if len(scsiRows) != 1 || scsiRows[0].Dev != "13,0" || scsiRows[0].PairedCount != 1 || scsiRows[0].LineStart != 4 || scsiRows[0].LineEnd != 5 {
			t.Fatalf("Storage bad-row hole was bridged or legal sibling lost: rows=%+v caveats=%v", scsiRows, stats.Caveats)
		}
		if !containsSubstring(stats.Caveats, "duration_pairing_exact_lane_quarantined=true; family=storage_latency") {
			t.Fatalf("Storage exact-lane quarantine was not disclosed: %v", stats.Caveats)
		}
	})
}

func TestRawBlockStorageUnknownKeyFailsOnlyPhysicalSourceFamilyClosed(t *testing.T) {
	tests := []struct {
		name   string
		family durationOrderFamily
		trace  string
		gone   func(WindowStats) bool
	}{
		{
			name: "block", family: durationOrderBlockIO,
			trace: strings.Join([]string{
				`io-30 (30) [bad] .... 1.000000: block_rq_issue: malformed`,
				`io-30 (30) [001] .... 1.100000: block_rq_issue: 8,0 R 4096 () 64 + 8 [io]`,
				`io-30 (30) [001] .... 1.200000: block_rq_complete: 8,0 R () 64 + 8 [0]`,
			}, "\n") + "\n",
			gone: func(stats WindowStats) bool { return len(stats.IOLatencies) == 0 },
		},
		{
			name: "storage", family: durationOrderStorage,
			trace: strings.Join([]string{
				`io-40 (40) [bad] .... 1.000000: scsi_dispatch_cmd_start: malformed`,
				`io-40 (40) [001] .... 1.100000: scsi_dispatch_cmd_start: dev=13,0 op=read bytes=4096`,
				`io-40 (40) [001] .... 1.200000: scsi_dispatch_cmd_done: dev=13,0 op=read bytes=4096`,
			}, "\n") + "\n",
			gone: func(stats WindowStats) bool {
				for _, row := range stats.StorageLatencyByLayer {
					if row.Layer == "scsi" {
						return false
					}
				}
				return true
			},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idx := buildTraceIndex(t, "raw-"+tc.name+"-unknown-key.systrace", tc.trace)
			failure := rawPairingFailureForLine(idx, tc.family, 1)
			if failure == nil || failure.LaneKey != "" || failure.SourcePath == "" || !containsString(failure.Fields, "canonical_pairing_identity") {
				t.Fatalf("unknown-key raw row did not mint a source-family barrier: %+v", failure)
			}
			stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.3})
			if !tc.gone(stats) || !containsSubstring(stats.Caveats, "duration_pairing_source_fail_closed=true; family="+string(tc.family)) {
				t.Fatalf("unknown-key raw %s row survived source-family fail-close: stats=%+v caveats=%v", tc.name, stats, stats.Caveats)
			}
		})
	}
}

func TestRawStorageBarrierKeepsCompositePhysicalCoordinates(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "raw-storage-child.systrace")
	bundle := filepath.Join(dir, "raw-storage.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, strings.Join([]string{
		`io-40 (40) [001] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096`,
		`io-40 (40) [bad] .... 1.100000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096`,
		`io-40 (40) [001] .... 1.200000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096`,
	}, "\n")+"\n")
	writeBundleProvenanceFixture(t, bundle, `{"version":"test","systrace":"raw-storage-child.systrace","artifacts":[{"type":"systrace","path":"raw-storage-child.systrace"}]}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	var failure *durationOrderViolation
	for i := range idx.durationOrderFailures {
		candidate := &idx.durationOrderFailures[i]
		if candidate.Family == durationOrderStorage && candidate.LocalLine == 2 {
			failure = candidate
			break
		}
	}
	if failure == nil || failure.LaneKey == "" || failure.SourcePath != canonicalTraceIndexPath(systrace) || failure.LocalLine != 2 {
		t.Fatalf("composite raw barrier lost physical source/local-line provenance: %+v all=%+v", failure, idx.durationOrderFailures)
	}
}

func TestRawBlockStorageMalformedTimestampUsesUnknownCoordinate(t *testing.T) {
	badTimestamp := strings.Repeat("9", 400) + ".0"
	tests := []struct {
		name      string
		family    durationOrderFamily
		validLine string
		malformed string
	}{
		{
			name: "block", family: durationOrderBlockIO,
			validLine: `io-30 (30) [001] .... 1.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 [io]`,
			malformed: `io-30 (30) [001] .... ` + badTimestamp + `: block_rq_complete: 8,0 R () 32 + 8 [0]`,
		},
		{
			name: "storage", family: durationOrderStorage,
			validLine: `io-40 (40) [001] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096`,
			malformed: `io-40 (40) [001] .... ` + badTimestamp + `: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096`,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name+"/direct", func(t *testing.T) {
			idx := buildTraceIndex(t, "malformed-ts-"+tc.name+".systrace", tc.validLine+"\n"+tc.malformed+"\n")
			failure := rawPairingFailureForLine(idx, tc.family, 2)
			if failure == nil || !failure.TsUnknown || failure.CurrentTs != 0 || failure.SourcePath != canonicalTraceIndexPath(idx.Path) || failure.Line != 2 || failure.LaneKey == "" {
				t.Fatalf("direct malformed timestamp violated unknown-coordinate/source contract: %+v", failure)
			}
		})
		t.Run(tc.name+"/composite", func(t *testing.T) {
			dir := t.TempDir()
			child := filepath.Join(dir, "malformed-ts-"+tc.name+".systrace")
			bundle := filepath.Join(dir, "malformed-ts-"+tc.name+".tracebundle.json")
			writeBundleProvenanceFixture(t, child, tc.validLine+"\n"+tc.malformed+"\n")
			writeBundleProvenanceFixture(t, bundle, `{"version":"test","systrace":"malformed-ts-`+tc.name+`.systrace","artifacts":[{"type":"systrace","path":"malformed-ts-`+tc.name+`.systrace"}]}`)
			idx, err := BuildIndex(context.Background(), bundle)
			if err != nil {
				t.Fatalf("composite build rejected a source row whose timestamp is explicitly unknown: %v", err)
			}
			var failure *durationOrderViolation
			for i := range idx.durationOrderFailures {
				candidate := &idx.durationOrderFailures[i]
				if candidate.Family == tc.family && candidate.LocalLine == 2 {
					failure = candidate
					break
				}
			}
			if failure == nil || !failure.TsUnknown || failure.CurrentTs != 0 || failure.SourcePath != canonicalTraceIndexPath(child) || failure.LocalLine != 2 || failure.LaneKey == "" {
				t.Fatalf("composite malformed timestamp lost unknown-coordinate/physical provenance: %+v all=%+v", failure, idx.durationOrderFailures)
			}
		})
	}
}
