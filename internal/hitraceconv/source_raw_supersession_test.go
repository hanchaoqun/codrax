package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// traceDBMixedBlockStorageSourcePrecedenceFixture is the §40.12 mixed fixture:
// two governed block_rq_* rows plus four DB-only MMC/SCSI endpoint rows that
// share the SQL raw class "block_storage" but are never source-lane targets.
func traceDBMixedBlockStorageSourcePrecedenceFixture(t *testing.T) string {
	t.Helper()
	return createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 10, 'app')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 10, 1, 'io', 0, 1, 1)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'dev')",
		"INSERT INTO data_dict VALUES (2, 'sector')",
		"INSERT INTO data_dict VALUES (3, 'nr_sector')",
		"INSERT INTO data_dict VALUES (4, 'bytes')",
		"INSERT INTO data_dict VALUES (5, 'rwbs')",
		"INSERT INTO data_dict VALUES (6, 'cmd')",
		"INSERT INTO data_dict VALUES (7, 'comm')",
		"INSERT INTO data_dict VALUES (8, 'error')",
		"INSERT INTO data_dict VALUES (10, '8,0')",
		"INSERT INTO data_dict VALUES (11, 'R')",
		"INSERT INTO data_dict VALUES (12, 'READ')",
		"INSERT INTO data_dict VALUES (13, 'io')",
		"INSERT INTO data_dict VALUES (20, 'name')",
		"INSERT INTO data_dict VALUES (21, 'tag')",
		"INSERT INTO data_dict VALUES (22, 'cmd_opcode')",
		"INSERT INTO data_dict VALUES (23, 'blocks')",
		"INSERT INTO data_dict VALUES (24, 'block_size')",
		"INSERT INTO data_dict VALUES (25, 'blk_addr')",
		"INSERT INTO data_dict VALUES (26, 'bytes_xfered')",
		"INSERT INTO data_dict VALUES (27, 'ret')",
		"INSERT INTO data_dict VALUES (28, 'cmd_err')",
		"INSERT INTO data_dict VALUES (29, 'data_err')",
		"INSERT INTO data_dict VALUES (30, 'lba')",
		"INSERT INTO data_dict VALUES (31, 'len')",
		"INSERT INTO data_dict VALUES (32, 'opcode')",
		"INSERT INTO data_dict VALUES (40, 'mmc0')",
		"INSERT INTO data_dict VALUES (41, '8:0')",
		"INSERT INTO data_dict VALUES (42, 'READ_10')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 1, 10)",
		"INSERT INTO args VALUES (1, 2, 0, 100)",
		"INSERT INTO args VALUES (1, 3, 0, 8)",
		"INSERT INTO args VALUES (1, 4, 0, 4096)",
		"INSERT INTO args VALUES (1, 5, 1, 11)",
		"INSERT INTO args VALUES (1, 6, 1, 12)",
		"INSERT INTO args VALUES (1, 7, 1, 13)",
		"INSERT INTO args VALUES (2, 1, 1, 10)",
		"INSERT INTO args VALUES (2, 2, 0, 100)",
		"INSERT INTO args VALUES (2, 3, 0, 8)",
		"INSERT INTO args VALUES (2, 5, 1, 11)",
		"INSERT INTO args VALUES (2, 6, 1, 12)",
		"INSERT INTO args VALUES (2, 8, 0, 0)",
		"INSERT INTO args VALUES (3, 20, 1, 40)",
		"INSERT INTO args VALUES (3, 21, 0, -1)",
		"INSERT INTO args VALUES (3, 22, 0, 17)",
		"INSERT INTO args VALUES (3, 23, 0, 8)",
		"INSERT INTO args VALUES (3, 24, 0, 512)",
		"INSERT INTO args VALUES (3, 25, 0, 100)",
		"INSERT INTO args VALUES (4, 20, 1, 40)",
		"INSERT INTO args VALUES (4, 21, 0, -1)",
		"INSERT INTO args VALUES (4, 22, 0, 17)",
		"INSERT INTO args VALUES (4, 26, 0, 4096)",
		"INSERT INTO args VALUES (4, 27, 0, -5)",
		"INSERT INTO args VALUES (4, 28, 0, -6)",
		"INSERT INTO args VALUES (4, 29, 0, -7)",
		"INSERT INTO args VALUES (5, 21, 0, -1)",
		"INSERT INTO args VALUES (5, 1, 1, 41)",
		"INSERT INTO args VALUES (5, 30, 0, 200)",
		"INSERT INTO args VALUES (5, 31, 0, 8)",
		"INSERT INTO args VALUES (5, 32, 1, 42)",
		"INSERT INTO args VALUES (6, 21, 0, -1)",
		"INSERT INTO args VALUES (6, 1, 1, 41)",
		"INSERT INTO args VALUES (6, 30, 0, 200)",
		"INSERT INTO args VALUES (6, 31, 0, 8)",
		"INSERT INTO args VALUES (6, 32, 1, 42)",
		"INSERT INTO args VALUES (6, 27, 0, 0)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000000, 'block_rq_issue', 1, 1, 1)",
		"INSERT INTO raw VALUES (2, 2000000, 'block_rq_complete', 2, 1, 2)",
		"INSERT INTO raw VALUES (3, 3000000, 'mmc_request_start', 1, 1, 3)",
		"INSERT INTO raw VALUES (4, 4000000, 'mmc_request_done', 1, 1, 4)",
		"INSERT INTO raw VALUES (5, 5000000, 'scsi_dispatch_cmd_start', 1, 1, 5)",
		"INSERT INTO raw VALUES (6, 6000000, 'scsi_dispatch_cmd_done', 1, 1, 6)",
	})
}

func traceDBCompleteSourceBlockPairInventory() *traceDBSourceNameInventory {
	return &traceDBSourceNameInventory{
		Names: map[int64]string{10: "io"},
		RawDecode: TraceDBCoverage{
			Found: true,
			Metadata: map[string]string{
				"decode_state":                  "strict_target_ledger_complete",
				"retention_block_storage_state": "complete",
			},
			Metrics: map[string]int64{
				"target_block_rq_issue_records":              1,
				"target_block_rq_issue_body_admitted":        1,
				"target_block_rq_complete_records":           1,
				"target_block_rq_complete_body_admitted":     1,
				"target_block_storage_records_retained":      2,
				"target_block_storage_record_capture_failed": 0,
			},
		},
		RawBlock: []traceDBRawBlockRecord{
			{PhysicalOrdinal: 1, TimestampNS: 1_000_000, CPU: 1, HeaderPID: 10,
				Name: "block_rq_issue", Body: "8,0 R 4096 (READ) 100 + 8 [io]"},
			{PhysicalOrdinal: 2, TimestampNS: 2_000_000, CPU: 2, HeaderPID: 10,
				Name: "block_rq_complete", Body: "8,0 R (READ) 100 + 8 [0]"},
		},
	}
}

// §40.12 V6-1: a complete source block family supersedes exactly the DB rows
// whose event names the source lane governs (block_rq_*/block_bio_*). Other
// members of the SQL raw class "block_storage" (MMC/UFS/SCSI endpoints) are
// never source targets, so they stay published by the DB lane, and the source
// lane's overlap twin must not read those published rows as a block overlap.
func TestCompleteSourceRawBlockFamilySupersedesSQLiteKeepsUngovernedBlockStorageNames(t *testing.T) {
	tdb, err := openTraceDB(context.Background(), traceDBMixedBlockStorageSourcePrecedenceFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	tdb.sourceNameInventory = traceDBCompleteSourceBlockPairInventory()
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 10, Name: "app"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 10, IPID: 1, Name: "io"}
	buildTraceDBThreadSecondaryIndexes(&index)
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	rawCoverage, err := exportTraceDBRawFtraceFamilies(
		context.Background(), tdb, sink, traceDBTestCompleteSchedulerAuthority(index),
		traceDBSchedulerRunningIndex{}, filepath.Join(t.TempDir(), "mixed-precedence.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	var block TraceDBCoverage
	for _, item := range rawCoverage {
		if item.Family == "raw_ftrace" && item.Table == "block_storage" {
			block = item
		}
	}
	if block.RowsRead != 6 || block.RowsEmitted != 4 ||
		block.Skipped != "superseded_complete_source_raw_block_family=2" ||
		block.Metrics["source_governed_block_storage_rows_publishable"] != 0 ||
		len(sink.rows) != 4 {
		t.Fatalf("source block precedence suppressed by class instead of governed name: block=%+v rows=%+v",
			block, sink.rows)
	}
	body := ""
	for _, row := range sink.rows {
		body += row.line + "\n"
	}
	for _, want := range []string{
		"mmc_request_start: mmc0 tag=-1",
		"mmc_request_done: mmc0 tag=-1 opcode=17 bytes_xfered=4096 ret=-5 cmd_err=-6 data_err=-7",
		"scsi_dispatch_cmd_start: tag=-1 dev=8:0",
		"scsi_dispatch_cmd_done: tag=-1 dev=8:0",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("ungoverned block_storage endpoint missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "block_rq_") {
		t.Fatalf("superseded governed DB block rows leaked:\n%s", body)
	}
	recovered, err := publishTraceDBRawBlockRecovery(
		context.Background(), tdb.sourceNameInventory, sink, rawCoverage)
	if err != nil || recovered.RowsEmitted != 2 || len(sink.rows) != 6 ||
		recovered.Metadata["publication_state"] != "published_complete_exact_source_family" {
		t.Fatalf("overlap twin read ungoverned DB rows as a block overlap: coverage=%+v rows=%d err=%v",
			recovered, len(sink.rows), err)
	}
}

// §40.12 V6-1 end-to-end: an official RMQ capture with a complete block family
// converted through trace_streamer whose DB carries both governed block_rq_*
// rows and DB-only SCSI endpoints publishes exactly one source block pair,
// zero DB block_rq_* rows, and both SCSI endpoints.
func TestTraceStreamerConversionPublishesSourceBlockFamilyAlongsideDBStorageEndpoints(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses a POSIX shell")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "official-block-mixed.sys")
	output := filepath.Join(dir, "official-block-mixed.systrace")
	if err := os.WriteFile(input, traceDBRawBlockRecoveryCapture(t), 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, rawFtraceRootCauseFixtureStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output,
		TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatal(err)
	}
	bodyBytes, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Count(body, "block_rq_issue: 12,80 RCVHS 32768 () 923339752 + 64 [com.tencent.mm]") != 1 ||
		strings.Count(body, "block_rq_complete: 12,80 RCVHS () 923339752 + 64 [0]") != 1 ||
		strings.Contains(body, "block_rq_issue: 8,0") || strings.Contains(body, "block_rq_complete: 8,0") {
		t.Fatalf("source block family did not supersede exactly the governed DB block rows:\n%s", body)
	}
	for _, want := range []string{"scsi_dispatch_cmd_start: tag=7 dev=8:0", "scsi_dispatch_cmd_done: tag=7 dev=8:0"} {
		if strings.Count(body, want) != 1 {
			t.Fatalf("DB-only SCSI endpoint %q missing from mixed conversion:\n%s", want, body)
		}
	}
	found := false
	for _, coverage := range result.TraceDBCoverage {
		if coverage.Family == "source_rawtrace_block" {
			found = coverage.RowsEmitted == 2 &&
				coverage.Metadata["publication_state"] == "published_complete_exact_source_family"
		}
	}
	if !found {
		t.Fatalf("source block recovery coverage missing or withheld: %+v", result.TraceDBCoverage)
	}
}

// §40.12 structural tripwire: for every registered supersession the set of
// names it can suppress (Governed) equals the family's ledger target set, the
// registry covers every exact recovery family exactly once with unique wire
// tokens, governed sets are pairwise disjoint, and names that share a SQL raw
// class without being source targets are never governed.
func TestTraceDBRawSourceSupersessionGovernedSetsMatchLedger(t *testing.T) {
	vocabulary := dedupeStrings(append(traceDBRawDecodeTargetNames(),
		"mmc_request_start", "mmc_request_done",
		"ufshcd_command_start", "ufshcd_command_done",
		"scsi_dispatch_cmd_start", "scsi_dispatch_cmd_done",
		"android_fs_dataread_start", "binder_transaction", "dma_fence_signaled",
		"dma_fence_wait_start", "dma_fence_wait_end", "dma_fence_init", "dma_fence_destroy", "dma_fence_enable_signal", "dma_fence_emit",
		"MMC_request_start", "block_rq_issue_vendor", "Block_rq_issue", " block_rq_issue", ""))
	ledgerSet := func(entry traceDBRawSourceSupersession) map[string]bool {
		set := map[string]bool{}
		if entry.Family == traceDBRawRetentionBlock {
			for _, name := range traceDBRawBlockTargetNames() {
				set[name] = true
			}
			return set
		}
		if entry.DBSupersedesSource {
			// §40.42 ④a: the DMA families' governed sets are the decode ledger's
			// retention families (inverse policy, never a superseding entry).
			for _, name := range vocabulary {
				if name != "" && traceDBRawRetentionFamily(name) == entry.Family {
					set[name] = true
				}
			}
			return set
		}
		for _, name := range traceDBRawExactRecoveryTargetNames() {
			if traceDBRawExactRecoveryFamily(name) == entry.Family {
				set[name] = true
			}
		}
		return set
	}
	if len(traceDBRawSourceSupersessions) != 1+len(traceDBRawExactRecoveryFamilies)+2 {
		t.Fatalf("supersession registry size %d != block + %d exact families + 2 DB-superseding DMA families",
			len(traceDBRawSourceSupersessions), len(traceDBRawExactRecoveryFamilies))
	}
	dbSuperseding := 0
	for _, entry := range traceDBRawSourceSupersessions {
		if entry.DBSupersedesSource {
			dbSuperseding++
			if entry.Eligible(&traceDBSourceNameInventory{}) {
				t.Fatalf("DB-superseding family %s must never become a superseding entry", entry.Family)
			}
		}
	}
	if dbSuperseding != 2 {
		t.Fatalf("exactly the two DMA families carry the inverse policy, got %d", dbSuperseding)
	}
	families, reasons := map[string]bool{}, map[string]bool{}
	owners := map[string]string{}
	for _, entry := range traceDBRawSourceSupersessions {
		if entry.Family == "" || entry.Reason == "" || entry.Governed == nil || entry.Eligible == nil ||
			families[entry.Family] || reasons[entry.Reason] {
			t.Fatalf("supersession entry incomplete or duplicated: family=%q reason=%q", entry.Family, entry.Reason)
		}
		families[entry.Family], reasons[entry.Reason] = true, true
		want := ledgerSet(entry)
		if len(want) == 0 {
			t.Fatalf("family %s has an empty ledger target set", entry.Family)
		}
		for _, name := range vocabulary {
			if entry.Governed(name) != want[name] {
				t.Fatalf("family %s governed(%q)=%v diverged from ledger target set %v",
					entry.Family, name, entry.Governed(name), want)
			}
			if entry.Governed(name) {
				if owner, taken := owners[name]; taken {
					t.Fatalf("name %q governed by both %s and %s", name, owner, entry.Family)
				}
				owners[name] = entry.Family
				if got, ok := traceDBRawSourceSupersessionFor(name); !ok || got.Family != entry.Family {
					t.Fatalf("supersession lookup for %q returned %+v ok=%v want family %s", name, got, ok, entry.Family)
				}
			}
		}
	}
	for _, family := range traceDBRawExactRecoveryFamilies {
		if !families[family] {
			t.Fatalf("exact recovery family %s missing from supersession registry", family)
		}
	}
	block, ok := traceDBRawSourceSupersessionForFamily(traceDBRawRetentionBlock)
	if !ok || block.Reason != "block" || block.RowReason() != "superseded_complete_source_raw_block_family" ||
		block.PublishableMetric() != "source_governed_block_storage_rows_publishable" ||
		block.PrecedenceField() != "block_source_precedence" {
		t.Fatalf("block supersession wire tokens drifted: %+v ok=%v", block, ok)
	}
	for _, name := range vocabulary {
		if traceDBRawBlockNameGoverned(name) != (traceDBRawRetentionFamily(name) == traceDBRawRetentionBlock) ||
			traceDBRawBlockNameGoverned(name) != directBlockNameGoverned(name) {
			t.Fatalf("block governed vocabularies diverged at %q: target=%v retention=%v direct=%v",
				name, traceDBRawBlockNameGoverned(name),
				traceDBRawRetentionFamily(name) == traceDBRawRetentionBlock, directBlockNameGoverned(name))
		}
		if _, governed := traceDBRawSourceSupersessionFor(name); governed != (owners[name] != "") {
			t.Fatalf("supersession lookup for %q disagrees with the registry census", name)
		}
		if traceDBRawFtraceClass(name) == "block_storage" && !traceDBRawBlockNameGoverned(name) {
			if _, governed := traceDBRawSourceSupersessionFor(name); governed {
				t.Fatalf("ungoverned block_storage class member %q would be superseded", name)
			}
		}
	}
	if _, governed := traceDBRawSourceSupersessionFor("mmc_request_start"); governed {
		t.Fatal("mmc_request_start is not a source target and must never be superseded")
	}
}

// §40.12 source-text tripwire: DB-lane source precedence and both source-lane
// overlap checks must be keyed by governed event name through the registry,
// never by SQL raw class or by a class-wide RowsEmitted comparison.
func TestTraceDBRawExportSupersessionNeverKeysOnClass(t *testing.T) {
	export := mustReadRendererSource(t, "streamerdb_export_raw_ftrace.go")
	body := sourceBetween(t, export, "func exportTraceDBRawFtraceFamilies(", "func traceDBRawArgsetsReady(")
	for _, forbidden := range []string{
		`class == "block_storage"`, `SupersedesDB[class]`, `sourceBlockSupersedesDB`, `sourceExactSupersedesDB`,
		`traceDBRawExactRecoveryDBClass(`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("DB raw exporter keys source precedence on class again: %q", forbidden)
		}
	}
	if !strings.Contains(body, "traceDBRawSourceSupersessionFor(") ||
		!strings.Contains(body, "range traceDBRawSourceSupersessions") {
		t.Fatal("DB raw exporter no longer reads the source supersession registry")
	}
	for _, file := range []string{"source_raw_block_recovery.go", "source_raw_exact_recovery.go", "source_raw_dma_wait_recovery.go", "source_raw_dma_lifecycle_recovery.go"} {
		source := mustReadRendererSource(t, file)
		if strings.Contains(source, "RowsEmitted > 0") || strings.Contains(source, `item.Table == "`) ||
			strings.Contains(source, "traceDBRawExactRecoveryDBClass") {
			t.Fatalf("%s overlap check keys on SQL raw class or class-wide RowsEmitted again", file)
		}
		if !strings.Contains(source, "traceDBRawSourceGovernedRowsPublishable(") {
			t.Fatalf("%s overlap check no longer reads the governed-name publishable census", file)
		}
	}
}
