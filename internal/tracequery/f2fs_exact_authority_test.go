package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	f2fsSyncEnterBody     = "dev=8:0 ino=0x9 pino=0x1 i_mode=0x81a4 i_size=4096 i_nlink=1 i_blocks=8 i_advise=0x0"
	f2fsSyncExitBody      = "dev=8:0 ino=0x9 cp_reason=0 datasync=1 ret=0"
	f2fsDIOEnter510Body   = "dev=8:0 ino=0x9 pos=0 len=4096 rw=read"
	f2fsDIOEnter66Body    = "dev=8:0 ino=0x9 pos=0 len=4096 ki_flags=0x0 ki_ioprio=0x0 rw=read"
	f2fsDIOExitBody       = "dev=8:0 ino=0x9 pos=0 len=4096 rw=read ret=4096"
	f2fsWriteBegin510Body = "dev=8:0 ino=0x9 pos=0 len=4096 flags=0"
	f2fsWriteBegin66Body  = "dev=8:0 ino=0x9 pos=0 len=4096"
	f2fsWriteEndBody      = "dev=8:0 ino=0x9 pos=0 len=4096 copied=4096"
)

func TestF2FSExactWireProfilesPairAndCountOneRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, startEvent, startBody, doneEvent, doneBody, base string
		wantBytes                                              int64
	}{
		{name: "sync", startEvent: "f2fs_sync_file_enter", startBody: f2fsSyncEnterBody, doneEvent: "f2fs_sync_file_exit", doneBody: f2fsSyncExitBody, base: "f2fs_sync_file"},
		{name: "direct 5.10", startEvent: "f2fs_direct_IO_enter", startBody: f2fsDIOEnter510Body, doneEvent: "f2fs_direct_IO_exit", doneBody: f2fsDIOExitBody, base: "f2fs_direct_io", wantBytes: 4096},
		{name: "direct 6.6", startEvent: "f2fs_direct_IO_enter", startBody: f2fsDIOEnter66Body, doneEvent: "f2fs_direct_IO_exit", doneBody: f2fsDIOExitBody, base: "f2fs_direct_io", wantBytes: 4096},
		{name: "write 5.10", startEvent: "f2fs_write_begin", startBody: f2fsWriteBegin510Body, doneEvent: "f2fs_write_end", doneBody: f2fsWriteEndBody, base: "f2fs_write", wantBytes: 4096},
		{name: "write 6.6", startEvent: "f2fs_write_begin", startBody: f2fsWriteBegin66Body, doneEvent: "f2fs_write_end", doneBody: f2fsWriteEndBody, base: "f2fs_write", wantBytes: 4096},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start := DecodePairingEndpoint(tc.startEvent, tc.startBody, 40)
			done := DecodePairingEndpoint(tc.doneEvent, tc.doneBody, 40)
			if !start.Recognized || !start.KeyKnown || !start.PayloadAdmitted ||
				!done.Recognized || !done.KeyKnown || !done.PayloadAdmitted ||
				start.SemanticKey == "" || start.SemanticKey != done.SemanticKey {
				t.Fatalf("exact F2FS endpoints did not share one admitted lane: start=%+v done=%+v", start, done)
			}
			idx := buildTraceIndex(t, "f2fs-exact.systrace",
				"io-40 (40) [003] .... 1.000000: "+tc.startEvent+": "+tc.startBody+"\n"+
					"io-40 (40) [003] .... 1.002000: "+tc.doneEvent+": "+tc.doneBody+"\n")
			stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.1})
			row := storageLatencyRow(stats.StorageLatencyByLayer, "f2fs", tc.base)
			if row == nil || row.PairedCount != 1 || !near(row.MaxLatencyMs, 2, .001) || row.Bytes != tc.wantBytes {
				t.Fatalf("exact F2FS pair/byte account drifted: row=%+v all=%+v caveats=%v", row, stats.StorageLatencyByLayer, stats.Caveats)
			}
			foundEvidence := false
			for _, fact := range evidenceFromStats(stats) {
				if fact.Predicate == "storage_latency_by_layer" && fact.Subject == "f2fs" && fact.Object == tc.base {
					foundEvidence = true
					break
				}
			}
			if !foundEvidence {
				t.Fatalf("valid exact F2FS pair did not reach typed evidence: %+v", evidenceFromStats(stats))
			}
			foundRank := false
			for _, item := range BuildRootCauseRank(idx, Query{TimeStart: .9, TimeEnd: 1.1}).Items {
				if strings.Contains(item.Type, "io_") && item.EffectiveImpactMs > 0 {
					foundRank = true
					break
				}
			}
			if !foundRank {
				t.Fatalf("valid exact F2FS latency did not reach root-rank IO candidates: stats=%+v", stats)
			}
		})
	}
}

func TestF2FSClosedWireProfilesRejectMalformedAndNameDrift(t *testing.T) {
	t.Parallel()
	malformed := []struct{ name, event, body string }{
		{name: "extra", event: "f2fs_direct_IO_enter", body: f2fsDIOEnter510Body + " extra=1"},
		{name: "reordered", event: "f2fs_direct_IO_enter", body: "ino=0x9 dev=8:0 pos=0 len=4096 rw=read"},
		{name: "duplicate", event: "f2fs_direct_IO_enter", body: f2fsDIOEnter510Body + " rw=read"},
		{name: "alias", event: "f2fs_direct_IO_enter", body: "fs_dev=8:0 ino=0x9 pos=0 len=4096 rw=read"},
		{name: "spaced equals", event: "f2fs_direct_IO_enter", body: "dev = 8:0 ino=0x9 pos=0 len=4096 rw=read"},
		{name: "zero inode", event: "f2fs_direct_IO_enter", body: strings.Replace(f2fsDIOEnter510Body, "ino=0x9", "ino=0x0", 1)},
		{name: "negative position", event: "f2fs_direct_IO_enter", body: strings.Replace(f2fsDIOEnter510Body, "pos=0", "pos=-1", 1)},
		{name: "oversized length", event: "f2fs_direct_IO_enter", body: strings.Replace(f2fsDIOEnter510Body, "len=4096", "len=9223372036854775808", 1)},
		{name: "noncanonical hex", event: "f2fs_sync_file_enter", body: strings.Replace(f2fsSyncEnterBody, "i_mode=0x81a4", "i_mode=0X81A4", 1)},
		{name: "device leading zero", event: "f2fs_direct_IO_enter", body: strings.Replace(f2fsDIOEnter510Body, "dev=8:0", "dev=08:00", 1)},
		{name: "inode leading zero", event: "f2fs_direct_IO_enter", body: strings.Replace(f2fsDIOEnter510Body, "ino=0x9", "ino=0x09", 1)},
		{name: "parent inode leading zero", event: "f2fs_sync_file_enter", body: strings.Replace(f2fsSyncEnterBody, "pino=0x1", "pino=0x01", 1)},
		{name: "write flags overflow", event: "f2fs_write_begin", body: strings.Replace(f2fsWriteBegin510Body, "flags=0", "flags=4294967296", 1)},
	}
	for _, tc := range malformed {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DecodePairingEndpoint(tc.event, tc.body, 40)
			if !got.Recognized || got.PayloadAdmitted {
				t.Fatalf("malformed exact F2FS body gained semantic authority: %+v body=%q", got, tc.body)
			}
		})
	}
	for _, event := range []string{
		"f2fs_direct_io_enter", "F2FS_direct_IO_enter", "f2fs_direct_IO_enter_extra", " f2fs_direct_IO_enter",
		"f2fs_directIO_enter", "f2fs_syncfile_exit", "f2fs_write_enter", "f2fs_directIO_enter_extra",
	} {
		if got := DecodePairingEndpoint(event, f2fsDIOEnter510Body, 40); got.Recognized || got.KeyKnown || got.PayloadAdmitted {
			t.Fatalf("non-byte-exact F2FS name entered the endpoint registry: event=%q verdict=%+v", event, got)
		}
	}
}

func TestF2FSUnregisteredEndpointShapesStayRawInventoryOnly(t *testing.T) {
	t.Parallel()
	idx := buildTraceIndex(t, "f2fs-near-inventory.systrace",
		"io-40 (40) [003] .... 1.000000: f2fs_directIO_enter: "+f2fsDIOEnter510Body+"\n"+
			"io-40 (40) [003] .... 1.001000: f2fs_directIO_exit: "+f2fsDIOExitBody+"\n"+
			"io-40 (40) [003] .... 1.002000: f2fs_syncfile_enter: "+f2fsSyncEnterBody+"\n"+
			"io-40 (40) [003] .... 1.003000: f2fs_syncfile_exit: "+f2fsSyncExitBody+"\n"+
			"io-40 (40) [003] .... 1.004000: f2fs_write_enter: "+f2fsWriteBegin66Body+"\n"+
			"io-40 (40) [003] .... 1.005000: f2fs_write_exit: "+f2fsWriteEndBody+"\n")
	q := Query{TimeStart: .9, TimeEnd: 1.1, Limit: 20}
	stats := ComputeWindowStats(idx, q)
	if stats.FilesystemEventCount != 6 || stats.EventCounts[EventFilesystem] != 6 {
		t.Fatalf("unregistered F2FS rows lost raw inventory: count=%d events=%v", stats.FilesystemEventCount, stats.EventCounts)
	}
	if len(stats.StorageLatencyByLayer) != 0 || len(stats.FileIOByInode) != 0 || len(stats.FilesystemResources) != 0 ||
		stats.TopIOInodes != nil || len(stats.SubsystemEvents) != 0 || stats.IOPressureSummary != nil || len(stats.IOBurstEpisodes) != 0 {
		t.Fatalf("unregistered F2FS endpoint shapes leaked semantic carriers: storage=%+v file=%+v resources=%+v subsystem=%+v pressure=%+v bursts=%+v",
			stats.StorageLatencyByLayer, stats.FileIOByInode, stats.FilesystemResources, stats.SubsystemEvents, stats.IOPressureSummary, stats.IOBurstEpisodes)
	}
	for _, fact := range append(evidenceFromStats(stats), evidenceFromEvents(EventSearch(idx, q))...) {
		if strings.Contains(strings.ToLower(fact.Summary+" "+fact.Subject+" "+fact.Object), "f2fs") || fact.Predicate == "storage_latency_by_layer" {
			t.Fatalf("unregistered F2FS endpoint leaked evidence: %+v", fact)
		}
	}
	for _, item := range BuildRootCauseRank(idx, q).Items {
		if strings.Contains(strings.ToLower(item.Type+" "+item.Source+" "+item.Summary), "f2fs") || strings.Contains(item.Type, "io_") {
			t.Fatalf("unregistered F2FS endpoint leaked root-rank seat: %+v", item)
		}
	}
}

func TestF2FSMalformedExactRowsStayInventoryOnlyAcrossAllSemanticSurfaces(t *testing.T) {
	t.Parallel()
	malformed := []struct {
		name, event, body string
	}{
		{name: "known key bad nonkey", event: "f2fs_direct_IO_enter", body: strings.Replace(f2fsDIOEnter510Body, "len=4096", "len=9223372036854775808", 1)},
		{name: "unknown key", event: "f2fs_direct_IO_enter", body: strings.Replace(f2fsDIOEnter510Body, "ino=0x9", "ino=0x0", 1)},
		{name: "truncated canonical", event: "f2fs_sync_file_enter", body: "dev=8:0 ino=0x9"},
		{name: "profile union rescue", event: "f2fs_direct_IO_enter", body: "dev=8:0 ino=0x9 len=4096 rw=read"},
		{name: "official TP_printk text remains deferred", event: "f2fs_direct_IO_enter", body: "dev = (8,0), ino = 9 pos = 0 len = 4096 rw = 0"},
	}
	for _, tc := range malformed {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idx := buildTraceIndex(t, "f2fs-malformed-inventory.systrace",
				"io-40 (40) [003] .... 1.000000: "+tc.event+": "+tc.body+"\n")
			assertF2FSInventoryOnly(t, idx, 1)
		})
	}
}

func assertF2FSInventoryOnly(t *testing.T, idx *Index, wantRows int) {
	t.Helper()
	q := Query{TimeStart: .9, TimeEnd: 1.1, Limit: 20}
	stats := ComputeWindowStats(idx, q)
	if stats.FilesystemEventCount != wantRows || stats.EventCounts[EventFilesystem] != wantRows {
		t.Fatalf("F2FS rejected rows lost raw inventory: count=%d events=%v", stats.FilesystemEventCount, stats.EventCounts)
	}
	if len(stats.StorageLatencyByLayer) != 0 || len(stats.FilesystemResources) != 0 || len(stats.FileIOByInode) != 0 ||
		stats.TopIOInodes != nil || stats.IOPressureSummary != nil || len(stats.IOBurstEpisodes) != 0 || len(stats.SubsystemEvents) != 0 {
		t.Fatalf("inventory-only F2FS row leaked semantic carriers: storage=%+v resources=%+v file=%+v top=%+v pressure=%+v bursts=%+v subsystem=%+v caveats=%v",
			stats.StorageLatencyByLayer, stats.FilesystemResources, stats.FileIOByInode, stats.TopIOInodes,
			stats.IOPressureSummary, stats.IOBurstEpisodes, stats.SubsystemEvents, stats.Caveats)
	}
	for _, fact := range append(evidenceFromStats(stats), evidenceFromEvents(EventSearch(idx, q))...) {
		if strings.Contains(strings.ToLower(fact.Summary+" "+fact.Subject+" "+fact.Object), "f2fs") || fact.Predicate == "storage_latency_by_layer" {
			t.Fatalf("inventory-only F2FS row leaked evidence: %+v", fact)
		}
	}
	for _, item := range BuildRootCauseRank(idx, q).Items {
		if strings.Contains(strings.ToLower(item.Type+" "+item.Source+" "+item.Summary), "f2fs") || strings.Contains(item.Type, "io_") {
			t.Fatalf("inventory-only F2FS row leaked root-rank seat: %+v", item)
		}
	}
}

func TestF2FSKeyAdmissionIsSeparateFromNonKeyPayloadAndRejectsTruncatedRosters(t *testing.T) {
	t.Parallel()
	base := DecodePairingEndpoint("f2fs_direct_IO_enter", f2fsDIOEnter510Body, 40)
	duplicateLen := DecodePairingEndpoint("f2fs_direct_IO_enter", f2fsDIOEnter510Body+" len=4096", 40)
	duplicateDev := DecodePairingEndpoint("f2fs_direct_IO_enter", f2fsDIOEnter510Body+" dev=8:0", 40)
	if !base.KeyKnown || !base.PayloadAdmitted || !duplicateLen.KeyKnown || duplicateLen.PayloadAdmitted || duplicateLen.SemanticKey != base.SemanticKey {
		t.Fatalf("non-key duplicate did not remain an exact-lane payload rejection: base=%+v duplicate=%+v", base, duplicateLen)
	}
	if duplicateDev.KeyKnown || duplicateDev.PayloadAdmitted || duplicateDev.SemanticKey != "" {
		t.Fatalf("duplicate hard key retained request identity: %+v", duplicateDev)
	}

	truncated := []struct {
		name, event, body string
	}{
		{name: "sync enter key only", event: "f2fs_sync_file_enter", body: "dev=8:0 ino=0x9"},
		{name: "sync enter legacy decimal inode", event: "f2fs_sync_file_enter", body: "dev=8:0 ino=9"},
		{name: "sync exit compact", event: "f2fs_sync_file_exit", body: "dev=8:0 ino=0x9 ret=0"},
		{name: "direct enter missing position", event: "f2fs_direct_IO_enter", body: "dev=8:0 ino=0x9 len=4096 rw=read"},
		{name: "direct enter key and direction only", event: "f2fs_direct_IO_enter", body: "dev=8:0 ino=0x9 rw=read"},
		{name: "direct exit compact", event: "f2fs_direct_IO_exit", body: "dev=8:0 ino=0x9 len=4096 rw=read ret=4096"},
	}
	for _, tc := range truncated {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DecodePairingEndpoint(tc.event, tc.body, 40)
			if !got.Recognized || got.PayloadAdmitted {
				t.Fatalf("truncated/compatibility F2FS body escaped the canonical closed roster: got=%+v body=%q", got, tc.body)
			}
		})
	}
}

func TestF2FSInvalidHardKeyDuplicatesCannotRetainIdentity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{name: "valid then comma dev", body: f2fsDIOEnter510Body + " dev=8:0,"},
		{name: "comma dev then valid", body: "dev=8:0, " + f2fsDIOEnter510Body},
		{name: "valid then spaced dev", body: f2fsDIOEnter510Body + " dev = 9:0"},
		{name: "spaced dev then valid", body: "dev = 9:0 " + f2fsDIOEnter510Body},
		{name: "valid then tab spaced dev", body: f2fsDIOEnter510Body + " dev\t=\t9:0"},
		{name: "valid then punctuated dev", body: f2fsDIOEnter510Body + " garbage=1,dev=9:0"},
		{name: "punctuated dev then valid", body: "garbage=1,dev=9:0 " + f2fsDIOEnter510Body},
		{name: "valid then unicode bounded dev", body: f2fsDIOEnter510Body + " 中dev=9:0"},
		{name: "valid then empty inode", body: f2fsDIOEnter510Body + " ino="},
		{name: "empty inode then valid", body: "ino= " + f2fsDIOEnter510Body},
		{name: "valid then spaced inode", body: f2fsDIOEnter510Body + " ino = 0xa"},
		{name: "spaced inode then valid", body: "ino = 0xa " + f2fsDIOEnter510Body},
		{name: "valid then quoted rw", body: f2fsDIOEnter510Body + ` rw="read"`},
		{name: "quoted rw then valid", body: `dev=8:0 ino=0x9 pos=0 len=4096 rw="read" rw=read`},
		{name: "valid then spaced rw", body: f2fsDIOEnter510Body + " rw = write"},
		{name: "spaced rw then valid", body: "rw = write " + f2fsDIOEnter510Body},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DecodePairingEndpoint("f2fs_direct_IO_enter", tc.body, 40)
			if !got.Recognized || got.KeyKnown || got.PayloadAdmitted || got.SemanticKey != "" {
				t.Fatalf("invalid hard-key duplicate retained F2FS identity: verdict=%+v body=%q", got, tc.body)
			}
		})
	}
}

func TestF2FSInvalidHardKeyOccurrenceDoesNotAuthorizeValue(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		"dev=8:0, ino=0x9 pos=0 len=4096 rw=read",
		"dev = 8:0 ino=0x9 pos=0 len=4096 rw=read",
		"garbage=1,dev=8:0 ino=0x9 pos=0 len=4096 rw=read",
	} {
		tokens, _, duplicates, closed := f2fsClosedTokens(body)
		if closed || duplicates["dev"] {
			t.Fatalf("one invalid physical declaration was misclassified: body=%q closed=%v duplicates=%v", body, closed, duplicates)
		}
		if _, authorized := tokens["dev"]; authorized {
			t.Fatalf("negative hard-key occurrence census authorized an invalid value: body=%q tokens=%v", body, tokens)
		}
		if got := DecodePairingEndpoint("f2fs_direct_IO_enter", body, 40); !got.Recognized || got.KeyKnown || got.PayloadAdmitted {
			t.Fatalf("invalid-only hard key gained semantic authority: body=%q verdict=%+v", body, got)
		}
	}
}

func TestF2FSHardKeyOccurrenceCensusRejectsOnlyExactLabels(t *testing.T) {
	t.Parallel()
	body := f2fsDIOEnter510Body + " xdev = 9:0 device = 9:0 prose_dev = 9:0 note=dev plain_dev_word"
	occurrences := f2fsHardIdentityOccurrences(body)
	if occurrences["dev"] != 1 || occurrences["ino"] != 1 || occurrences["rw"] != 1 {
		t.Fatalf("near-label/prose text changed the exact hard-key census: occurrences=%v body=%q", occurrences, body)
	}
	base := DecodePairingEndpoint("f2fs_direct_IO_enter", f2fsDIOEnter510Body, 40)
	withNoise := DecodePairingEndpoint("f2fs_direct_IO_enter", body, 40)
	if !withNoise.Recognized || !withNoise.KeyKnown || withNoise.PayloadAdmitted || withNoise.SemanticKey != base.SemanticKey {
		t.Fatalf("near-label/prose text acquired hard-key authority or erased the proven lane: base=%+v noisy=%+v", base, withNoise)
	}

	unicodeBoundary := f2fsDIOEnter510Body + " 中dev=9:0"
	occurrences = f2fsHardIdentityOccurrences(unicodeBoundary)
	if occurrences["dev"] != 2 {
		t.Fatalf("non-ASCII predecessor hid an exact ASCII hard-key declaration: occurrences=%v body=%q", occurrences, unicodeBoundary)
	}
	if got := DecodePairingEndpoint("f2fs_direct_IO_enter", unicodeBoundary, 40); !got.Recognized || got.KeyKnown || got.PayloadAdmitted || got.SemanticKey != "" {
		t.Fatalf("Unicode-bounded hard-key duplicate retained F2FS identity: %+v", got)
	}
}

func TestF2FSInvalidHardKeyDuplicatePoisonsSourceScopeAcrossPair(t *testing.T) {
	t.Parallel()
	malformedBodies := []struct {
		name string
		body string
	}{
		{name: "comma duplicate", body: f2fsDIOEnter510Body + " dev=8:0,"},
		{name: "spaced hidden after", body: "dev=9:0 ino=0xa pos=0 len=4096 rw=read dev = 8:0"},
		{name: "spaced hidden before", body: "dev = 8:0 dev=9:0 ino=0xa pos=0 len=4096 rw=read"},
		{name: "punctuated hidden after", body: "dev=9:0 ino=0xa pos=0 len=4096 rw=read garbage=1,dev=8:0"},
		{name: "punctuated hidden before", body: "garbage=1,dev=8:0 dev=9:0 ino=0xa pos=0 len=4096 rw=read"},
	}
	for _, tc := range malformedBodies {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idx := buildTraceIndex(t, "f2fs-invalid-duplicate-anti-rescue.systrace",
				"io-40 (40) [003] .... 1.000000: f2fs_direct_IO_enter: "+f2fsDIOEnter510Body+"\n"+
					"io-40 (40) [003] .... 1.001000: f2fs_direct_IO_enter: "+tc.body+"\n"+
					"io-40 (40) [003] .... 1.002000: f2fs_direct_IO_exit: "+f2fsDIOExitBody+"\n")
			stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.1, Limit: 20})
			if row := storageLatencyRow(stats.StorageLatencyByLayer, "f2fs", "f2fs_direct_io"); row != nil && row.PairedCount != 0 {
				t.Fatalf("invalid duplicate was deleted and the surrounding endpoints paired: row=%+v caveats=%v", row, stats.Caveats)
			}
			if len(stats.FileIOByInode) == 0 {
				t.Fatalf("source-scope pairing quarantine erased independent clean endpoint facts: %+v", stats)
			}
			for _, inode := range stats.BlockIOByInode {
				if inode.StorageMaxLatencyMs != 0 {
					t.Fatalf("invalid duplicate minted a storage-latency projection: %+v", inode)
				}
			}
			if stats.IOPressureSummary != nil && stats.IOPressureSummary.StorageMaxLatencyMs != 0 {
				t.Fatalf("invalid duplicate minted storage-latency pressure: %+v", stats.IOPressureSummary)
			}
			foundCleanFileRank := false
			for _, item := range BuildRootCauseRank(idx, Query{TimeStart: .9, TimeEnd: 1.1, Limit: 20}).Items {
				switch item.Type {
				case "file_io_hot_inode":
					foundCleanFileRank = true
				case "io_burst_episode":
					t.Fatalf("invalid duplicate minted an elapsed storage-latency rank: %+v", item)
				}
			}
			if !foundCleanFileRank {
				t.Fatal("pairing quarantine erased the positive-control rank from independently valid endpoint activity")
			}
			if !containsSubstring(stats.Caveats, "duration_pairing_source_fail_closed=true") ||
				!containsSubstring(stats.Caveats, "sources=0 source_scopes=1") {
				t.Fatalf("unknown duplicate identity did not poison the source-local F2FS scope: %v", stats.Caveats)
			}
		})
	}
}

func TestF2FSWireScalarBoundariesKeepU64BlocksButBoundLengths(t *testing.T) {
	t.Parallel()
	maxBlocks := strings.Replace(f2fsSyncEnterBody, "i_blocks=8", "i_blocks=18446744073709551615", 1)
	if got := DecodePairingEndpoint("f2fs_sync_file_enter", maxBlocks, 40); !got.KeyKnown || !got.PayloadAdmitted {
		t.Fatalf("source-valid blkcnt_t MaxUint64 was rejected: %+v", got)
	}
	maxLen := strings.Replace(f2fsDIOEnter510Body, "len=4096", "len=9223372036854775807", 1)
	if got := DecodePairingEndpoint("f2fs_direct_IO_enter", maxLen, 40); !got.KeyKnown || !got.PayloadAdmitted {
		t.Fatalf("MaxInt64 DIO length was rejected: %+v", got)
	}
	overLen := strings.Replace(f2fsDIOEnter510Body, "len=4096", "len=9223372036854775808", 1)
	if got := DecodePairingEndpoint("f2fs_direct_IO_enter", overLen, 40); !got.KeyKnown || got.PayloadAdmitted {
		t.Fatalf("consumer-unsafe DIO length did not remain an exact-lane rejection: %+v", got)
	}
}

func TestF2FSTypedIdentityCannotSubstituteForClosedPayloadAdmission(t *testing.T) {
	t.Parallel()
	input := PairingEndpointTypedInput{
		Name: "f2fs_direct_IO_enter", HeaderTID: 40,
		StorageIdentityKnown: true, StorageDevice: "8:0", StorageInode: "0x9", StorageOperation: "read",
	}
	identityOnly := FingerprintPairingEndpoint(input)
	if !identityOnly.Recognized || !identityOnly.KeyKnown || identityOnly.PayloadAdmitted || identityOnly.SemanticKey == "" {
		t.Fatalf("typed F2FS identity substituted for an unknown closed-body verdict: %+v", identityOnly)
	}
	input.StoragePayloadAdmissionKnown = true
	input.StoragePayloadAdmitted = true
	admitted := FingerprintPairingEndpoint(input)
	if !admitted.KeyKnown || !admitted.PayloadAdmitted || admitted.SemanticKey != identityOnly.SemanticKey {
		t.Fatalf("explicit F2FS closed-body verdict did not admit the same lane: identity=%+v admitted=%+v", identityOnly, admitted)
	}
}

func TestF2FSByteAggregatesSaturateAcrossMaxInt64Cohorts(t *testing.T) {
	t.Parallel()
	start := strings.Replace(f2fsDIOEnter510Body, "len=4096", "len=9223372036854775807", 1)
	done := strings.Replace(f2fsDIOExitBody, "len=4096", "len=9223372036854775807", 1)
	idx := buildTraceIndex(t, "f2fs-byte-saturation.systrace",
		"io-40 (40) [003] .... 1.000000: f2fs_direct_IO_enter: "+start+"\n"+
			"io-40 (40) [003] .... 1.001000: f2fs_direct_IO_exit: "+done+"\n"+
			"io-40 (40) [003] .... 1.002000: f2fs_direct_IO_enter: "+start+"\n"+
			"io-40 (40) [003] .... 1.003000: f2fs_direct_IO_exit: "+done+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	storage := storageLatencyRow(stats.StorageLatencyByLayer, "f2fs", "f2fs_direct_io")
	if storage == nil || storage.PairedCount != 2 || storage.Bytes != int64(^uint64(0)>>1) {
		t.Fatalf("storage byte account overflowed instead of saturating: %+v", storage)
	}
	if len(stats.FileIOByInode) != 1 || stats.FileIOByInode[0].Bytes != int64(^uint64(0)>>1) || stats.FileIOByInode[0].Bytes < 0 {
		t.Fatalf("file IO byte account overflowed: %+v", stats.FileIOByInode)
	}
	if len(stats.FilesystemResources) != 1 || stats.FilesystemResources[0].Bytes != int64(^uint64(0)>>1) || stats.FilesystemResources[0].Bytes < 0 {
		t.Fatalf("filesystem resource byte account overflowed: %+v", stats.FilesystemResources)
	}
	if stats.IOPressureSummary == nil || stats.IOPressureSummary.FileIOBytes != int64(^uint64(0)>>1) || stats.IOPressureSummary.FileIOBytes < 0 {
		t.Fatalf("IO pressure byte account overflowed: %+v", stats.IOPressureSummary)
	}
	if len(stats.BlockIOByInode) != 1 || stats.BlockIOByInode[0].FileIOBytes != int64(^uint64(0)>>1) ||
		stats.BlockIOByInode[0].FileIOBytes < 0 || stats.BlockIOByInode[0].StorageMaxLatencyMs <= 0 {
		t.Fatalf("block/inode projection byte account overflowed: %+v", stats.BlockIOByInode)
	}
	for _, item := range BuildRootCauseRank(idx, Query{TimeStart: .9, TimeEnd: 1.1}).Items {
		if strings.Contains(item.Type, "io_") && (item.EffectiveImpactMs < 0 || item.CumulativeImpactMs < 0) {
			t.Fatalf("byte overflow propagated a negative IO rank impact: %+v", item)
		}
	}
}

func TestF2FSMalformedEndpointCannotBeDeletedOrPoisonOtherStorageProfiles(t *testing.T) {
	t.Parallel()
	// Zero inode destroys the request identity but leaves the exact event-name
	// scope known. Two witnesses pin that repeated scoped poison never widens
	// into a whole-source storage outage.
	malformed := strings.Replace(f2fsDIOEnter510Body, "ino=0x9", "ino=0x0", 1)
	idx := buildTraceIndex(t, "f2fs-anti-rescue.systrace",
		"io-40 (40) [003] .... 1.000000: f2fs_direct_IO_enter: "+f2fsDIOEnter510Body+"\n"+
			"io-40 (40) [003] .... 1.001000: f2fs_direct_IO_enter: "+malformed+"\n"+
			"io-40 (40) [003] .... 1.001100: f2fs_direct_IO_enter: "+malformed+"\n"+
			"io-40 (40) [003] .... 1.002000: f2fs_direct_IO_exit: "+f2fsDIOExitBody+"\n"+
			"io-40 (40) [003] .... 1.002100: f2fs_sync_file_enter: "+f2fsSyncEnterBody+"\n"+
			"io-40 (40) [003] .... 1.002200: f2fs_sync_file_exit: "+f2fsSyncExitBody+"\n"+
			"io-40 (40) [003] .... 1.003000: scsi_dispatch_cmd_start: tag=-1 dev=8:0 lba=2 len=8 opcode=READ_10\n"+
			"io-40 (40) [003] .... 1.004000: scsi_dispatch_cmd_done: tag=-1 dev=8:0 lba=2 len=8 opcode=READ_10 ret=0\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if row := storageLatencyRow(stats.StorageLatencyByLayer, "f2fs", "f2fs_direct_io"); row != nil && row.PairedCount != 0 {
		t.Fatalf("malformed F2FS endpoint was deleted and surrounding rows bridged: row=%+v caveats=%v", row, stats.Caveats)
	}
	if row := storageLatencyRow(stats.StorageLatencyByLayer, "f2fs", "f2fs_sync_file"); row != nil && row.PairedCount != 0 {
		t.Fatalf("unknown F2FS hard key did not quarantine the whole source-local F2FS family: row=%+v caveats=%v", row, stats.Caveats)
	}
	scsi := storageLatencyRow(stats.StorageLatencyByLayer, "scsi", "scsi_dispatch_cmd")
	if scsi == nil || scsi.PairedCount != 1 {
		t.Fatalf("F2FS source-scope poison leaked into independent SCSI profile: rows=%+v caveats=%v", stats.StorageLatencyByLayer, stats.Caveats)
	}
	if !containsSubstring(stats.Caveats, "duration_pairing_source_fail_closed=true") ||
		!containsSubstring(stats.Caveats, "sources=0 source_scopes=1") {
		t.Fatalf("F2FS scoped anti-rescue was not disclosed precisely: %v", stats.Caveats)
	}
}

func TestF2FSRetainedEventReplayCannotBridgeMalformedEndpoint(t *testing.T) {
	t.Parallel()
	malformed := strings.Replace(f2fsDIOEnter510Body, "len=4096", "len=9223372036854775808", 1)
	events := []Event{
		{Line: 1, Ts: 1, PID: 40, Type: EventFilesystem, Name: "f2fs_direct_IO_enter", FieldText: f2fsDIOEnter510Body},
		{Line: 2, Ts: 1.001, PID: 40, Type: EventFilesystem, Name: "f2fs_direct_IO_enter", FieldText: malformed},
		{Line: 3, Ts: 1.002, PID: 40, Type: EventFilesystem, Name: "f2fs_direct_IO_exit", FieldText: f2fsDIOExitBody},
	}
	decoded := decodeGenericStoragePairingEvent(&Index{Path: "/trace/f2fs-retained.systrace"}, events[1])
	if !decoded.endpoint || !decoded.verdict.Recognized || !decoded.verdict.KeyKnown || decoded.verdict.PayloadAdmitted || decoded.keyAdmitted {
		t.Fatalf("retained malformed exact F2FS endpoint escaped hard replay: %+v", decoded)
	}
	idx := &Index{Path: "/trace/f2fs-retained.systrace", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 3, Events: events}
	rows, caveats := computeStorageLatencyByLayer(idx, Query{TimeStart: .9, TimeEnd: 1.1}, nil, 8)
	if row := storageLatencyRow(rows, "f2fs", "f2fs_direct_io"); row != nil && row.PairedCount != 0 {
		t.Fatalf("consumer pre-audit deleted malformed retained endpoint and bridged: row=%+v caveats=%v", row, caveats)
	}
	if !containsSubstring(caveats, "duration_pairing_exact_lane_quarantined=true") {
		t.Fatalf("retained-event exact-lane quarantine was not disclosed: rows=%+v caveats=%v", rows, caveats)
	}
}

func TestF2FSWindowDiscoveryCannotGenerateProbeAcrossMalformedEndpoint(t *testing.T) {
	path := writeWindowDiscoveryTrace(t,
		"io-40 (40) [003] .... 1.000000: f2fs_direct_IO_enter: "+f2fsDIOEnter510Body+"\n"+
			"io-40 (40) [003] .... 1.001000: f2fs_direct_IO_enter: "+strings.Replace(f2fsDIOEnter510Body, "ino=0x9", "ino=0x0", 1)+"\n"+
			"io-40 (40) [003] .... 1.002000: f2fs_direct_IO_exit: "+f2fsDIOExitBody+"\n"+
			"io-40 (40) [003] .... 1.003000: f2fs_sync_file_enter: "+f2fsSyncEnterBody+"\n"+
			"io-40 (40) [003] .... 1.004000: f2fs_sync_file_exit: "+f2fsSyncExitBody+"\n"+
			"io-40 (40) [003] .... 1.005000: scsi_dispatch_cmd_start: tag=-1 dev=8:0 lba=2 len=8 opcode=READ_10\n"+
			"io-40 (40) [003] .... 1.006000: scsi_dispatch_cmd_done: tag=-1 dev=8:0 lba=2 len=8 opcode=READ_10 ret=0")
	req := pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IdentityComplete {
		t.Fatalf("malformed F2FS identity was hidden from discovery completeness: %+v", result)
	}
	if len(result.Windows) != 1 || result.Windows[0].Kind != "schema_probe" {
		t.Fatalf("independent clean storage profile did not remain collectable: %+v", result)
	}
	for _, candidate := range result.Candidates {
		if strings.Contains(strings.ToLower(candidate.Identity), "f2fs") {
			t.Fatalf("poisoned F2FS family generated a discovery candidate: %+v", result.Candidates)
		}
	}
}

func TestF2FSRightEdgeAdmissionSurvivesWarmIndexCacheAndJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f2fs-cache.systrace")
	body := "io-40 (40) [003] .... 1.000000: f2fs_direct_IO_enter: " + f2fsDIOEnter66Body + "\n" +
		"io-40 (40) [003] .... 1.002000: f2fs_direct_IO_exit: " + f2fsDIOExitBody + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		idx, err := BuildIndex(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		if len(idx.Events) != 2 || idx.Events[0].ResourceFields == nil || idx.Events[0].ResourceFields.f2fsPairing == nil {
			t.Fatalf("cache pass %d lost private F2FS admission side table: %+v", pass, idx.Events)
		}
		stats := ComputeWindowStats(idx, Query{})
		if row := storageLatencyRow(stats.StorageLatencyByLayer, "f2fs", "f2fs_direct_io"); row == nil || row.PairedCount != 1 {
			t.Fatalf("cache pass %d lost full-right-edge F2FS admission: rows=%+v caveats=%v", pass, stats.StorageLatencyByLayer, stats.Caveats)
		}
		encoded, err := json.Marshal(idx.Events[0])
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "f2fsPairing") || strings.Contains(string(encoded), "identityKnown") {
			t.Fatalf("private hard-admission side table leaked into public JSON: %s", encoded)
		}
	}
}
