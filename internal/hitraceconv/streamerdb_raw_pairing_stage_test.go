package hitraceconv

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func traceDBRawStageWorkObservation(t *testing.T, stage *traceDBRawPairingStage, stable, ts, tid, itid int64, work uint64, name string) traceDBRawPairingObservation {
	t.Helper()
	verdict := tracequery.FingerprintPairingEndpoint(tracequery.PairingEndpointTypedInput{
		Name: name, HeaderTID: tid, WorkAddress: work, WorkAddressKnown: true,
	})
	lane, ok := verdict.LaneKey(stage.artifactSource)
	if !ok || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
		t.Fatalf("invalid test endpoint verdict: %+v", verdict)
	}
	body := name + ": work=0x" + strings.ToLower(strconvFormatUintHex(work))
	row, err := prepareTraceDBRenderedRow(ts, 0, "worker", tid, tid, 0, body)
	if err != nil {
		t.Fatal(err)
	}
	return traceDBRawPairingObservation{
		StableID: stable, StableKnown: true, Timestamp: ts, TimestampKnown: true,
		Class: "workqueue", Line: row.line, Publishable: true,
		Verdict: verdict, LaneKey: lane, HeaderOwnerKnown: true,
		CanonicalITID: itid, CanonicalITIDKnown: true, EndpointAdmitted: true,
	}
}

func strconvFormatUintHex(value uint64) string {
	const digits = "0123456789abcdef"
	if value == 0 {
		return "0"
	}
	var buf [16]byte
	position := len(buf)
	for value > 0 {
		position--
		buf[position] = digits[value&15]
		value >>= 4
	}
	return string(buf[position:])
}

func traceDBRawStageDMAObservation(t *testing.T, stage *traceDBRawPairingStage, stable, ts int64, name string) traceDBRawPairingObservation {
	t.Helper()
	verdict := tracequery.FingerprintPairingEndpoint(tracequery.PairingEndpointTypedInput{
		Name: name, HeaderTID: 20, Driver: "gpu", Timeline: "render",
		ContextNumber: 7, ContextNumberKnown: true, SeqnoNumber: 9, SeqnoNumberKnown: true,
	})
	lane, ok := verdict.LaneKey(stage.artifactSource)
	if !ok {
		t.Fatalf("invalid DMA test verdict: %+v", verdict)
	}
	body := name + ": driver=gpu timeline=render context=7 seqno=9"
	row, err := prepareTraceDBRenderedRow(ts, 0, "gpu", 20, 20, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	return traceDBRawPairingObservation{
		StableID: stable, StableKnown: true, Timestamp: ts, TimestampKnown: true,
		Class: "dma_fence", Line: row.line, Publishable: true,
		Verdict: verdict, LaneKey: lane, HeaderOwnerKnown: true,
		CanonicalITID: 2, CanonicalITIDKnown: true, EndpointAdmitted: true,
	}
}

func TestTraceDBRawPairingStageFreezeQuarantinesCompleteExactLane(t *testing.T) {
	ctx := context.Background()
	stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer stage.cleanup()
	badStart := traceDBRawStageWorkObservation(t, stage, 1, 1000, 10, 1, 0xaa, "workqueue_execute_start")
	badHole := traceDBRawStageWorkObservation(t, stage, 2, 1100, 10, 1, 0xaa, "workqueue_execute_end")
	badHole.Publishable, badHole.EndpointAdmitted, badHole.Line = false, false, ""
	badDone := traceDBRawStageWorkObservation(t, stage, 3, 1200, 10, 1, 0xaa, "workqueue_execute_end")
	goodStart := traceDBRawStageWorkObservation(t, stage, 4, 1000, 10, 1, 0xbb, "workqueue_execute_start")
	goodDone := traceDBRawStageWorkObservation(t, stage, 5, 1300, 10, 1, 0xbb, "workqueue_execute_end")
	for _, observation := range []traceDBRawPairingObservation{badStart, badHole, badDone, goodStart, goodDone} {
		if err := stage.add(ctx, observation); err != nil {
			t.Fatal(err)
		}
	}
	report, err := stage.seal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.PoisonedLanes != 1 || report.PoisonedFamilies != 0 {
		t.Fatalf("unexpected quarantine scope: %+v", report)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	publish, err := stage.publish(ctx, sink, map[string]int{})
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, row := range sink.rows {
		joined += row.line + "\n"
	}
	if strings.Contains(joined, "work=0xaa") || strings.Count(joined, "work=0xbb") != 2 || publish.PublishedRows != 2 || publish.SuppressedRows != 2 {
		t.Fatalf("exact lane was rescued or sibling lost: report=%+v\n%s", publish, joined)
	}
	if err := stage.add(ctx, goodStart); err == nil {
		t.Fatal("sealed stage accepted a new record")
	}
	if _, err := stage.publish(ctx, sink, map[string]int{}); err == nil {
		t.Fatal("stage allowed a second publication")
	}
}

func TestTraceDBRawPairingStageFamilyPoisonAndSameTimestampStableOrder(t *testing.T) {
	ctx := context.Background()
	stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer stage.cleanup()
	work := traceDBRawStageWorkObservation(t, stage, 4, 1000, 10, 1, 0xaa, "workqueue_execute_start")
	unknown := traceDBRawPairingObservation{
		StableID: 5, StableKnown: true, Timestamp: 1100, TimestampKnown: true,
		Verdict:          tracequery.FingerprintPairingEndpoint(tracequery.PairingEndpointTypedInput{Name: "workqueue_execute_end", HeaderTID: 10}),
		HeaderOwnerKnown: true,
	}
	dmaDone := traceDBRawStageDMAObservation(t, stage, 3, 1000, "dma_fence_wait_end")
	dmaStart := traceDBRawStageDMAObservation(t, stage, 1, 1000, "dma_fence_wait_start")
	middle, err := prepareTraceDBRenderedRow(1000, 0, "worker", 10, 10, 0, "mm_filemap_add_to_page_cache: dev=8,0 ino=1")
	if err != nil {
		t.Fatal(err)
	}
	nonEndpoint := traceDBRawPairingObservation{
		StableID: 2, StableKnown: true, Timestamp: 1000, TimestampKnown: true,
		Class: "page_cache", Line: middle.line, Publishable: true,
	}
	for _, observation := range []traceDBRawPairingObservation{dmaStart, nonEndpoint, dmaDone, work, unknown} {
		if err := stage.add(ctx, observation); err != nil {
			t.Fatal(err)
		}
	}
	report, err := stage.seal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.PoisonedFamilies != 1 {
		t.Fatalf("unknown key did not poison exactly its family: %+v", report)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if _, err := stage.publish(ctx, sink, map[string]int{}); err != nil {
		t.Fatal(err)
	}
	if len(sink.rows) != 3 || !strings.Contains(sink.rows[0].line, "dma_fence_wait_start") ||
		!strings.Contains(sink.rows[1].line, "mm_filemap_add_to_page_cache") || !strings.Contains(sink.rows[2].line, "dma_fence_wait_end") {
		t.Fatalf("same-timestamp stable order changed or sibling family lost: %+v", sink.rows)
	}
}

func TestTraceDBRawPairingStagePhysicalRollbackAndGenerationBoundary(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name       string
		rows       func(*testing.T, *traceDBRawPairingStage) []traceDBRawPairingObservation
		want       int
		wantPoison int64
	}{
		{name: "timestamp rollback", rows: func(t *testing.T, stage *traceDBRawPairingStage) []traceDBRawPairingObservation {
			return []traceDBRawPairingObservation{
				traceDBRawStageWorkObservation(t, stage, 1, 2000, 10, 1, 0xaa, "workqueue_execute_start"),
				traceDBRawStageWorkObservation(t, stage, 2, 1000, 10, 1, 0xaa, "workqueue_execute_end"),
				traceDBRawStageWorkObservation(t, stage, 3, 3000, 10, 1, 0xbb, "workqueue_execute_start"),
				traceDBRawStageWorkObservation(t, stage, 4, 4000, 10, 1, 0xbb, "workqueue_execute_end"),
			}
		}, want: 2, wantPoison: 1},
		{name: "open cohort crosses generation", rows: func(t *testing.T, stage *traceDBRawPairingStage) []traceDBRawPairingObservation {
			return []traceDBRawPairingObservation{
				traceDBRawStageWorkObservation(t, stage, 1, 1000, 10, 1, 0xaa, "workqueue_execute_start"),
				traceDBRawStageWorkObservation(t, stage, 2, 2000, 10, 2, 0xaa, "workqueue_execute_end"),
				traceDBRawStageWorkObservation(t, stage, 3, 3000, 10, 2, 0xbb, "workqueue_execute_start"),
				traceDBRawStageWorkObservation(t, stage, 4, 4000, 10, 2, 0xbb, "workqueue_execute_end"),
			}
		}, want: 2, wantPoison: 1},
		{name: "closed cohort permits sequential reuse", rows: func(t *testing.T, stage *traceDBRawPairingStage) []traceDBRawPairingObservation {
			return []traceDBRawPairingObservation{
				traceDBRawStageWorkObservation(t, stage, 1, 1000, 10, 1, 0xaa, "workqueue_execute_start"),
				traceDBRawStageWorkObservation(t, stage, 2, 1500, 10, 1, 0xaa, "workqueue_execute_end"),
				traceDBRawStageWorkObservation(t, stage, 3, 2000, 10, 2, 0xaa, "workqueue_execute_start"),
				traceDBRawStageWorkObservation(t, stage, 4, 2500, 10, 2, 0xaa, "workqueue_execute_end"),
			}
		}, want: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{TempRoot: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer stage.cleanup()
			for _, observation := range test.rows(t, stage) {
				if err := stage.add(ctx, observation); err != nil {
					t.Fatal(err)
				}
			}
			sealed, err := stage.seal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if sealed.PoisonedLanes != test.wantPoison || sealed.PoisonedFamilies != 0 {
				t.Fatalf("quarantine scope=%+v want exact lanes=%d", sealed, test.wantPoison)
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 32)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			report, err := stage.publish(ctx, sink, map[string]int{})
			if err != nil || int(report.PublishedRows) != test.want {
				t.Fatalf("publish=%+v err=%v want=%d", report, err, test.want)
			}
		})
	}
}

func TestTraceDBRawPairingStageBinderAndBlockAllowCrossEmitterEndpoints(t *testing.T) {
	ctx := context.Background()
	stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer stage.cleanup()
	makeObservation := func(stable, ts, tid, itid int64, input tracequery.PairingEndpointTypedInput, body, class string) traceDBRawPairingObservation {
		input.HeaderTID = tid
		verdict := tracequery.FingerprintPairingEndpoint(input)
		lane, ok := verdict.LaneKey(stage.artifactSource)
		if !ok || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
			t.Fatalf("invalid cross-emitter verdict: %+v", verdict)
		}
		row, err := prepareTraceDBRenderedRow(ts, 0, "worker", tid, tid, 0, body)
		if err != nil {
			t.Fatal(err)
		}
		return traceDBRawPairingObservation{
			StableID: stable, StableKnown: true, Timestamp: ts, TimestampKnown: true,
			Class: class, Line: row.line, Publishable: true,
			Verdict: verdict, LaneKey: lane, HeaderOwnerKnown: true,
			CanonicalITID: itid, CanonicalITIDKnown: true, EndpointAdmitted: true,
		}
	}
	rows := []traceDBRawPairingObservation{
		makeObservation(1, 1000, 10, 1,
			tracequery.PairingEndpointTypedInput{Name: "binder_transaction", TransactionNumber: 7, TransactionNumberKnown: true},
			"binder_transaction: transaction=7 dest_node=1 dest_proc=20 dest_thread=20 reply=0 flags=0x0 code=0x1", "binder"),
		makeObservation(2, 1100, 20, 2,
			tracequery.PairingEndpointTypedInput{Name: "binder_transaction_received", TransactionNumber: 7, TransactionNumberKnown: true},
			"binder_transaction_received: transaction=7", "binder"),
		makeObservation(3, 2000, 10, 1,
			tracequery.PairingEndpointTypedInput{Name: "block_rq_issue", BlockIdentityKnown: true, BlockPayloadAdmissionKnown: true, BlockPayloadAdmitted: true, BlockDevice: "8,0", BlockOperation: "R", BlockSector: 100, BlockLength: 8},
			"block_rq_issue: 8,0 R 4096 () 100 + 8 []", "block_storage"),
		makeObservation(4, 2100, 20, 2,
			tracequery.PairingEndpointTypedInput{Name: "block_rq_complete", BlockIdentityKnown: true, BlockPayloadAdmissionKnown: true, BlockPayloadAdmitted: true, BlockDevice: "8,0", BlockOperation: "R", BlockSector: 100, BlockLength: 8},
			"block_rq_complete: 8,0 R () 100 + 8 [0]", "block_storage"),
	}
	for _, row := range rows {
		if err := stage.add(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	report, err := stage.seal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.PoisonedLanes != 0 || report.PoisonedFamilies != 0 {
		t.Fatalf("legal cross-emitter Binder/Block endpoints were poisoned: %+v", report)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	published, err := stage.publish(ctx, sink, map[string]int{})
	if err != nil || published.PublishedRows != 4 {
		t.Fatalf("cross-emitter publication=%+v err=%v", published, err)
	}
}

func TestTraceDBRawPairingStageRecordBudgetPublishesNothing(t *testing.T) {
	ctx := context.Background()
	stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{
		TempRoot: t.TempDir(), MaxRecords: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stage.cleanup()
	row := traceDBRawStageWorkObservation(t, stage, 1, 1000, 10, 1, 0xaa, "workqueue_execute_start")
	if err := stage.add(ctx, row); err != nil {
		t.Fatal(err)
	}
	err = stage.add(ctx, row)
	if reason, ok := traceDBRawPairingStageBudgetReason(err); !ok || reason != traceDBRawPairingBudgetRecordCap {
		t.Fatalf("record cap err=%v reason=%q", err, reason)
	}
	if _, err := stage.seal(ctx); err == nil {
		t.Fatal("budget-exhausted stage sealed for publication")
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if _, err := stage.publish(ctx, sink, map[string]int{}); err == nil || sink.stats.RowsAccepted != 0 {
		t.Fatalf("budget failure partially published: err=%v rows=%d", err, sink.stats.RowsAccepted)
	}
	var budget *traceDBRawPairingStageBudgetError
	if !errors.As(stage.failBudget(traceDBRawPairingBudgetRecordCap), &budget) {
		t.Fatal("budget error lost typed identity")
	}
}

func TestTraceDBRawPairingStageSequenceCapIsCheckedBeforePublication(t *testing.T) {
	ctx := context.Background()
	stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer stage.cleanup()
	if err := stage.add(ctx, traceDBRawStageWorkObservation(t, stage, 1, 1000, 10, 1, 0xaa, "workqueue_execute_start")); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.seal(ctx); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	sink.stats.RowsAccepted = math.MaxInt
	report, err := stage.publish(ctx, sink, map[string]int{})
	reason, budget := traceDBRawPairingStageBudgetReason(err)
	if !budget || reason != traceDBRawPairingBudgetSequenceCap || report.PublishedRows != 0 || len(sink.rows) != 0 {
		t.Fatalf("sequence cap published partially: report=%+v err=%v reason=%q rows=%d", report, err, reason, len(sink.rows))
	}
}

func TestTraceDBRawPairingStageLaneAndPageBudgetsFailClosedBeforePublish(t *testing.T) {
	ctx := context.Background()
	t.Run("lane key cap is family local", func(t *testing.T) {
		stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{
			TempRoot: t.TempDir(), MaxLaneKeys: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer stage.cleanup()
		first := traceDBRawStageWorkObservation(t, stage, 1, 1000, 10, 1, 0xaa, "workqueue_execute_start")
		second := traceDBRawStageWorkObservation(t, stage, 2, 1100, 10, 1, 0xbb, "workqueue_execute_start")
		if err := stage.add(ctx, first); err != nil {
			t.Fatal(err)
		}
		if err := stage.add(ctx, second); err == nil {
			t.Fatal("per-family lane key cap did not fire")
		} else if reason, ok := traceDBRawPairingStageBudgetReason(err); !ok || reason != traceDBRawPairingBudgetLaneKeyCap {
			t.Fatalf("lane key cap err=%v reason=%q", err, reason)
		}
		for _, row := range []traceDBRawPairingObservation{
			traceDBRawStageDMAObservation(t, stage, 3, 1200, "dma_fence_wait_start"),
			traceDBRawStageDMAObservation(t, stage, 4, 1300, "dma_fence_wait_end"),
		} {
			if err := stage.add(ctx, row); err != nil {
				t.Fatal(err)
			}
		}
		report, err := stage.seal(ctx)
		if err != nil || report.PoisonedFamilies != 1 {
			t.Fatalf("lane cap seal=%+v err=%v", report, err)
		}
		sink, err := newTraceDBRowSink(t.TempDir(), 32)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		published, err := stage.publish(ctx, sink, map[string]int{})
		if err != nil || published.PublishedRows != 2 {
			t.Fatalf("lane cap leaked WQ or erased DMA: %+v err=%v", published, err)
		}
	})

	t.Run("SQLite page cap is whole-stage zero publication", func(t *testing.T) {
		stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{
			TempRoot: t.TempDir(), MaxTempBytes: 512 << 10, MaxRecords: 10_000,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer stage.cleanup()
		budgetHit := false
		for stable := int64(1); stable <= 1000; stable++ {
			body := "print: " + strings.Repeat("x", 8192)
			row, err := prepareTraceDBRenderedRow(stable, 0, "worker", 10, 10, 0, body)
			if err != nil {
				t.Fatal(err)
			}
			err = stage.add(ctx, traceDBRawPairingObservation{
				StableID: stable, StableKnown: true, Timestamp: stable, TimestampKnown: true,
				Class: "page_cache", Line: row.line, Publishable: true,
			})
			if err == nil {
				continue
			}
			reason, ok := traceDBRawPairingStageBudgetReason(err)
			if !ok || reason != traceDBRawPairingBudgetSQLitePageCap {
				t.Fatalf("page cap err=%v reason=%q", err, reason)
			}
			budgetHit = true
			break
		}
		if !budgetHit {
			t.Fatal("configured page cap did not trigger")
		}
		if _, err := stage.seal(ctx); err == nil {
			t.Fatal("page-capped stage sealed for publication")
		}
		sink, err := newTraceDBRowSink(t.TempDir(), 32)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if _, err := stage.publish(ctx, sink, map[string]int{}); err == nil || sink.stats.RowsAccepted != 0 {
			t.Fatalf("page cap partially published: err=%v rows=%d", err, sink.stats.RowsAccepted)
		}
	})
}

func TestTraceDBRawPairingStageSecureCleanupIsIdempotent(t *testing.T) {
	ctx := context.Background()
	stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	workspace, dbPath := stage.workspace, stage.dbPath
	workspaceInfo, err := os.Stat(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if workspaceInfo.Mode().Perm() != 0o700 {
		t.Fatalf("workspace permissions=%v", workspaceInfo.Mode().Perm())
	}
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if dbInfo.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions=%v", dbInfo.Mode().Perm())
	}
	if err := stage.cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := stage.cleanup(); err != nil {
		t.Fatalf("second cleanup changed result: %v", err)
	}
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private workspace survived cleanup: %v", err)
	}
}

func TestTraceDBRawPairingAuditJournalIsBoundedAndCorruptionFailsHard(t *testing.T) {
	ctx := context.Background()
	t.Run("exact combined cap blocks SQLite page split", func(t *testing.T) {
		stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{
			TempRoot: t.TempDir(), MaxTempBytes: 4 << 20, MaxRecords: 10_000,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer stage.cleanup()
		journal, err := newTraceDBRawPairingPoisonJournal(ctx, stage)
		if err != nil {
			t.Fatal(err)
		}
		defer journal.remove()
		if err := journal.add(ctx, tracequery.PairingEndpointWorkqueue, []byte(strings.Repeat("x", 64<<10))); err != nil {
			t.Fatal(err)
		}
		if err := journal.seal(); err != nil {
			t.Fatal(err)
		}
		journalInfo, err := os.Stat(journal.path)
		if err != nil {
			t.Fatal(err)
		}
		if journalInfo.Mode().Perm() != 0o600 {
			t.Fatalf("audit journal permissions=%v", journalInfo.Mode().Perm())
		}
		var pagesBefore int64
		if err := stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pagesBefore); err != nil {
			t.Fatal(err)
		}
		// Leave exactly the existing DB pages plus the sealed journal. The
		// poison-row payload requires overflow pages, so SQLite's replay-time
		// max_page_count must reject the split without crossing the byte cap.
		stage.options.MaxTempBytes = pagesBefore*traceDBRawPairingSQLitePageBytes + journalInfo.Size()
		err = journal.replay(ctx)
		reason, budget := traceDBRawPairingStageBudgetReason(err)
		if !budget || reason != traceDBRawPairingBudgetTempByteCap {
			t.Fatalf("audit replay cap err=%v reason=%q", err, reason)
		}
		var pagesAfter int64
		if err := stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pagesAfter); err != nil {
			t.Fatal(err)
		}
		if combined := pagesAfter*traceDBRawPairingSQLitePageBytes + journalInfo.Size(); combined > stage.options.MaxTempBytes {
			t.Fatalf("audit replay crossed hard cap: combined=%d cap=%d", combined, stage.options.MaxTempBytes)
		}
		if _, err := stage.seal(ctx); err == nil {
			t.Fatal("audit-budgeted stage sealed for publication")
		}
		sink, err := newTraceDBRowSink(t.TempDir(), 32)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if _, err := stage.publish(ctx, sink, map[string]int{}); err == nil || len(sink.rows) != 0 {
			t.Fatalf("audit replay budget partially published: err=%v rows=%d", err, len(sink.rows))
		}
	})

	t.Run("CRC corruption", func(t *testing.T) {
		stage, err := newTraceDBRawPairingStage(ctx, filepath.Join(t.TempDir(), "out.ftrace"), traceDBRawPairingStageOptions{TempRoot: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		defer stage.cleanup()
		journal, err := newTraceDBRawPairingPoisonJournal(ctx, stage)
		if err != nil {
			t.Fatal(err)
		}
		defer journal.remove()
		if err := journal.add(ctx, tracequery.PairingEndpointWorkqueue, []byte("exact-lane")); err != nil {
			t.Fatal(err)
		}
		if err := journal.seal(); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(journal.path, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteAt([]byte{'X'}, 0); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if err := journal.replay(ctx); err == nil {
			t.Fatal("corrupted audit journal was replayed")
		} else if _, budget := traceDBRawPairingStageBudgetReason(err); budget {
			t.Fatalf("journal corruption was misclassified as budget: %v", err)
		}
	})
}

func TestTraceDBRawStandardEndpointParityRoundTrip(t *testing.T) {
	statements := []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 42, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 42, 1, 'worker', 1, 1)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'name')", "INSERT INTO data_dict VALUES (2, 'tag')",
		"INSERT INTO data_dict VALUES (3, 'cmd_opcode')", "INSERT INTO data_dict VALUES (4, 'blocks')",
		"INSERT INTO data_dict VALUES (5, 'block_size')", "INSERT INTO data_dict VALUES (6, 'blk_addr')",
		"INSERT INTO data_dict VALUES (7, 'bytes_xfered')", "INSERT INTO data_dict VALUES (8, 'ret')",
		"INSERT INTO data_dict VALUES (9, 'cmd_err')", "INSERT INTO data_dict VALUES (10, 'data_err')",
		"INSERT INTO data_dict VALUES (11, 'dev')", "INSERT INTO data_dict VALUES (12, 'lba')",
		"INSERT INTO data_dict VALUES (13, 'len')", "INSERT INTO data_dict VALUES (14, 'opcode')",
		"INSERT INTO data_dict VALUES (15, 'work')", "INSERT INTO data_dict VALUES (16, 'fs_dev')",
		"INSERT INTO data_dict VALUES (17, 'ino')", "INSERT INTO data_dict VALUES (18, 'bytes')",
		"INSERT INTO data_dict VALUES (19, 'rw')",
		"INSERT INTO data_dict VALUES (100, 'mmc0')", "INSERT INTO data_dict VALUES (101, '8:0')",
		"INSERT INTO data_dict VALUES (102, 'READ_10')", "INSERT INTO data_dict VALUES (103, '8,0')",
		"INSERT INTO data_dict VALUES (104, 'R')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1,1,1,100)", "INSERT INTO args VALUES (1,2,0,-1)",
		"INSERT INTO args VALUES (1,3,0,17)", "INSERT INTO args VALUES (1,4,0,8)",
		"INSERT INTO args VALUES (1,5,0,512)", "INSERT INTO args VALUES (1,6,0,100)",
		"INSERT INTO args VALUES (2,1,1,100)", "INSERT INTO args VALUES (2,2,0,-1)",
		"INSERT INTO args VALUES (2,3,0,17)", "INSERT INTO args VALUES (2,7,0,4096)",
		"INSERT INTO args VALUES (2,8,0,-5)", "INSERT INTO args VALUES (2,9,0,-6)",
		"INSERT INTO args VALUES (2,10,0,-7)",
		"INSERT INTO args VALUES (3,2,0,-1)", "INSERT INTO args VALUES (3,11,1,101)",
		"INSERT INTO args VALUES (3,12,0,200)", "INSERT INTO args VALUES (3,13,0,8)",
		"INSERT INTO args VALUES (3,14,1,102)",
		"INSERT INTO args VALUES (4,2,0,-1)", "INSERT INTO args VALUES (4,11,1,101)",
		"INSERT INTO args VALUES (4,12,0,200)", "INSERT INTO args VALUES (4,13,0,8)",
		"INSERT INTO args VALUES (4,14,1,102)", "INSERT INTO args VALUES (4,8,0,0)",
		"INSERT INTO args VALUES (5,15,0,2748)",
		"INSERT INTO args VALUES (6,16,1,103)", "INSERT INTO args VALUES (6,17,0,7)",
		"INSERT INTO args VALUES (6,18,0,4096)",
		"INSERT INTO args VALUES (7,16,1,103)", "INSERT INTO args VALUES (7,17,0,9)",
		"INSERT INTO args VALUES (7,13,0,4096)", "INSERT INTO args VALUES (7,19,1,104)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1,1000,'mmc_request_start',0,1,1)",
		"INSERT INTO raw VALUES (2,2000,'mmc_request_done',0,1,2)",
		"INSERT INTO raw VALUES (3,3000,'scsi_dispatch_cmd_start',0,1,3)",
		"INSERT INTO raw VALUES (4,4000,'scsi_dispatch_cmd_done',0,1,4)",
		"INSERT INTO raw VALUES (5,5000,'workqueue_execute_start',0,1,5)",
		"INSERT INTO raw VALUES (6,6000,'workqueue_execute_end',0,1,5)",
		"INSERT INTO raw VALUES (7,7000,'android_fs_dataread_start',0,1,6)",
		"INSERT INTO raw VALUES (8,8000,'android_fs_dataread_done',0,1,6)",
		"INSERT INTO raw VALUES (9,9000,'f2fs_direct_io_enter',0,1,7)",
		"INSERT INTO raw VALUES (10,10000,'f2fs_direct_io_exit',0,1,7)",
	}
	path := createTraceDBRawAuthorityFixture(t, statements)
	outPath := filepath.Join(t.TempDir(), "raw-standard-parity.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export standard endpoint parity fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"mmc_request_start: mmc0 tag=-1", "mmc_request_done: mmc0 tag=-1 opcode=17 bytes_xfered=4096 ret=-5 cmd_err=-6 data_err=-7",
		"scsi_dispatch_cmd_start: tag=-1 dev=8:0", "scsi_dispatch_cmd_done: tag=-1 dev=8:0",
		"workqueue_execute_start: work struct 0xabc", "workqueue_execute_end: work struct 0xabc",
		"android_fs_dataread_start: dev=8,0 ino=7", "f2fs_direct_io_enter: dev=8,0 ino=9",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("standard endpoint output missing %q:\n%s", want, body)
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "workqueue_execute_") && strings.Contains(line, "function ") {
			t.Fatalf("work-only WQ endpoint fabricated function metadata: %s", line)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatal(err)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 {
		t.Fatalf("work-only WQ endpoints did not pair: %+v", stats.WorkqueueActivity)
	}
	pairedByLayer := map[string]int{}
	for _, item := range stats.StorageLatencyByLayer {
		pairedByLayer[item.Layer] += item.PairedCount
	}
	for _, layer := range []string{"mmc", "scsi", "android_fs", "f2fs"} {
		if pairedByLayer[layer] != 1 {
			t.Fatalf("storage parity layer %s pairs=%d all=%+v coverage=%+v", layer, pairedByLayer[layer], stats.StorageLatencyByLayer, result.Coverage)
		}
	}
}

func TestTraceDBRawMMCDeviceClosedScalarTypedWireParity(t *testing.T) {
	text := func(value string) traceDBValue { return traceDBValue{Valid: true, Text: value, Datatype: 1} }
	integer := func(value string) traceDBValue { return traceDBValue{Valid: true, Text: value, Datatype: 0} }
	for _, tc := range []struct {
		name string
		args map[string]traceDBValue
	}{
		{name: "start", args: map[string]traceDBValue{
			"name": text("mmc0"), "tag": integer("-1"), "cmd_opcode": integer("17"),
			"blocks": integer("8"), "block_size": integer("512"), "blk_addr": integer("100"),
		}},
		{name: "done", args: map[string]traceDBValue{
			"name": text("mmc0"), "tag": integer("-1"), "cmd_opcode": integer("17"),
			"bytes_xfered": integer("4096"), "ret": integer("-5"),
		}},
	} {
		eventName := "mmc_request_" + tc.name
		if !traceDBRawRequiredArgs(eventName, tc.args, nil) {
			t.Fatalf("valid compact MMC %s args rejected", tc.name)
		}
		verdict := traceDBRawPairingVerdict(eventName, 42, tc.args, nil, true)
		body, rendered := traceDBRenderRawFtrace(eventName, tc.args, nil)
		if !rendered || !verdict.Recognized || !verdict.KeyKnown || !verdict.PayloadAdmitted ||
			!traceDBRawPairingWireParity(eventName, body, 42, verdict) {
			t.Fatalf("valid compact MMC typed/wire parity failed: verdict=%+v body=%q rendered=%t", verdict, body, rendered)
		}

		for _, bad := range []string{`"mmc0"`, "mmc0,"} {
			badArgs := make(map[string]traceDBValue, len(tc.args))
			for key, value := range tc.args {
				badArgs[key] = value
			}
			badArgs["name"] = text(bad)
			if traceDBRawRequiredArgs(eventName, badArgs, nil) {
				t.Fatalf("noncanonical MMC device passed SQL required-arg gate: event=%s device=%q", eventName, bad)
			}
			badVerdict := traceDBRawPairingVerdict(eventName, 42, badArgs, nil, true)
			if !badVerdict.Recognized || badVerdict.KeyKnown || badVerdict.PayloadAdmitted {
				t.Fatalf("noncanonical MMC device gained typed authority: event=%s device=%q verdict=%+v", eventName, bad, badVerdict)
			}
		}
	}
}

func TestTraceDBRawBadMMCDeviceStaysCoverageOnlyAndPreservesSibling(t *testing.T) {
	for _, badDevice := range []string{`"mmc0"`, "mmc0,"} {
		t.Run(badDevice, func(t *testing.T) {
			statements := []string{
				"CREATE TABLE trace_range (start_ts INT)", "INSERT INTO trace_range VALUES (0)",
				"CREATE TABLE process (ipid INT, pid INT, name TEXT)", "INSERT INTO process VALUES (1,42,'demo')",
				"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT)",
				"INSERT INTO thread VALUES (1,42,1,'worker',1,1)",
				"CREATE TABLE data_dict (id, data)",
				"INSERT INTO data_dict VALUES (1,'name')", "INSERT INTO data_dict VALUES (2,'tag')",
				"INSERT INTO data_dict VALUES (3,'cmd_opcode')", "INSERT INTO data_dict VALUES (4,'blocks')",
				"INSERT INTO data_dict VALUES (5,'block_size')", "INSERT INTO data_dict VALUES (6,'blk_addr')",
				"INSERT INTO data_dict VALUES (7,'work')", "INSERT INTO data_dict VALUES (100,'" + strings.ReplaceAll(badDevice, "'", "''") + "')",
				"CREATE TABLE args (argset, key, datatype, value)",
				"INSERT INTO args VALUES (1,1,1,100)", "INSERT INTO args VALUES (1,2,0,-1)",
				"INSERT INTO args VALUES (1,3,0,17)", "INSERT INTO args VALUES (1,4,0,8)",
				"INSERT INTO args VALUES (1,5,0,512)", "INSERT INTO args VALUES (1,6,0,100)",
				"INSERT INTO args VALUES (2,7,0,2748)",
				"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
				"INSERT INTO raw VALUES (1,1000,'mmc_request_start',0,1,1)",
				"INSERT INTO raw VALUES (2,2000,'workqueue_execute_start',0,1,2)",
				"INSERT INTO raw VALUES (3,3000,'workqueue_execute_end',0,1,2)",
			}
			path := createTraceDBRawAuthorityFixture(t, statements)
			outPath := filepath.Join(t.TempDir(), "bad-mmc-device.systrace")
			if _, err := exportTraceDBToSystrace(context.Background(), path, outPath); err != nil {
				t.Fatalf("bad MMC device escalated to conversion failure: %v", err)
			}
			body, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "mmc_request_start:") ||
				!strings.Contains(string(body), "workqueue_execute_start:") || !strings.Contains(string(body), "workqueue_execute_end:") {
				t.Fatalf("bad MMC device leaked or damaged valid sibling:\n%s", body)
			}
		})
	}
}

func TestTraceDBRawPairingFreezeFiveFamiliesRejectsRescueAndPreservesSibling(t *testing.T) {
	statements := []string{
		"CREATE TABLE trace_range (start_ts INT)", "INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)", "INSERT INTO process VALUES (1,42,'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1,42,1,'worker',1,1)",
		"INSERT INTO thread VALUES (2,43,1,'worker-rx',0,1)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1,'transaction')", "INSERT INTO data_dict VALUES (2,'dest_node')",
		"INSERT INTO data_dict VALUES (3,'dest_proc')", "INSERT INTO data_dict VALUES (4,'dest_thread')",
		"INSERT INTO data_dict VALUES (5,'reply')", "INSERT INTO data_dict VALUES (6,'flags')",
		"INSERT INTO data_dict VALUES (7,'code')", "INSERT INTO data_dict VALUES (8,'work')",
		"INSERT INTO data_dict VALUES (9,'driver')", "INSERT INTO data_dict VALUES (10,'timeline')",
		"INSERT INTO data_dict VALUES (11,'context')", "INSERT INTO data_dict VALUES (12,'seqno')",
		"INSERT INTO data_dict VALUES (13,'dev')", "INSERT INTO data_dict VALUES (14,'sector')",
		"INSERT INTO data_dict VALUES (15,'nr_sector')", "INSERT INTO data_dict VALUES (16,'bytes')",
		"INSERT INTO data_dict VALUES (17,'rwbs')", "INSERT INTO data_dict VALUES (18,'error')",
		"INSERT INTO data_dict VALUES (19,'tag')", "INSERT INTO data_dict VALUES (20,'lba')",
		"INSERT INTO data_dict VALUES (21,'len')", "INSERT INTO data_dict VALUES (22,'opcode')",
		"INSERT INTO data_dict VALUES (100,'gpu')", "INSERT INTO data_dict VALUES (101,'render')",
		"INSERT INTO data_dict VALUES (102,'8,0')", "INSERT INTO data_dict VALUES (103,'8,1')",
		"INSERT INTO data_dict VALUES (104,'R')", "INSERT INTO data_dict VALUES (105,'READ_10')",
		"INSERT INTO data_dict VALUES (106,'-1')",
		"CREATE TABLE args (argset, key, datatype, value)",
		// Binder bad lane 100 and clean sibling 200.
		"INSERT INTO args VALUES (1,1,0,100)", "INSERT INTO args VALUES (1,2,0,9)", "INSERT INTO args VALUES (1,3,0,500)",
		"INSERT INTO args VALUES (1,4,0,700)", "INSERT INTO args VALUES (1,5,0,0)", "INSERT INTO args VALUES (1,6,0,18)", "INSERT INTO args VALUES (1,7,0,4)",
		"INSERT INTO args VALUES (2,1,0,100)",
		"INSERT INTO args VALUES (3,1,0,200)", "INSERT INTO args VALUES (3,2,0,9)", "INSERT INTO args VALUES (3,3,0,500)",
		"INSERT INTO args VALUES (3,4,0,701)", "INSERT INTO args VALUES (3,5,0,0)", "INSERT INTO args VALUES (3,6,0,18)", "INSERT INTO args VALUES (3,7,0,4)",
		"INSERT INTO args VALUES (4,1,0,200)",
		// Workqueue and DMA keys.
		"INSERT INTO args VALUES (5,8,0,10)", "INSERT INTO args VALUES (6,8,0,20)",
		"INSERT INTO args VALUES (7,9,1,100)", "INSERT INTO args VALUES (7,10,1,101)", "INSERT INTO args VALUES (7,11,0,1)", "INSERT INTO args VALUES (7,12,0,1)",
		"INSERT INTO args VALUES (8,9,1,100)", "INSERT INTO args VALUES (8,10,1,101)", "INSERT INTO args VALUES (8,11,0,2)", "INSERT INTO args VALUES (8,12,0,2)",
		// Block issue/complete: argset 11 has a recoverable key but invalid int32 error.
		"INSERT INTO args VALUES (9,13,1,102)", "INSERT INTO args VALUES (9,14,0,100)", "INSERT INTO args VALUES (9,15,0,8)", "INSERT INTO args VALUES (9,16,0,4096)", "INSERT INTO args VALUES (9,17,1,104)",
		"INSERT INTO args VALUES (10,13,1,102)", "INSERT INTO args VALUES (10,14,0,100)", "INSERT INTO args VALUES (10,15,0,8)", "INSERT INTO args VALUES (10,17,1,104)", "INSERT INTO args VALUES (10,18,0,0)",
		"INSERT INTO args VALUES (11,13,1,102)", "INSERT INTO args VALUES (11,14,0,100)", "INSERT INTO args VALUES (11,15,0,8)", "INSERT INTO args VALUES (11,17,1,104)", "INSERT INTO args VALUES (11,18,0,2147483648)",
		"INSERT INTO args VALUES (12,13,1,102)", "INSERT INTO args VALUES (12,14,0,200)", "INSERT INTO args VALUES (12,15,0,8)", "INSERT INTO args VALUES (12,16,0,4096)", "INSERT INTO args VALUES (12,17,1,104)",
		"INSERT INTO args VALUES (13,13,1,102)", "INSERT INTO args VALUES (13,14,0,200)", "INSERT INTO args VALUES (13,15,0,8)", "INSERT INTO args VALUES (13,17,1,104)", "INSERT INTO args VALUES (13,18,0,0)",
		// SCSI: argset 15 has a TEXT tag, while dev=8,0 still localizes the coarse lane.
		"INSERT INTO args VALUES (14,19,0,-1)", "INSERT INTO args VALUES (14,13,1,102)", "INSERT INTO args VALUES (14,20,0,100)", "INSERT INTO args VALUES (14,21,0,8)", "INSERT INTO args VALUES (14,22,1,105)",
		"INSERT INTO args VALUES (15,19,1,106)", "INSERT INTO args VALUES (15,13,1,102)", "INSERT INTO args VALUES (15,20,0,100)", "INSERT INTO args VALUES (15,21,0,8)", "INSERT INTO args VALUES (15,22,1,105)",
		"INSERT INTO args VALUES (16,19,0,-1)", "INSERT INTO args VALUES (16,13,1,103)", "INSERT INTO args VALUES (16,20,0,200)", "INSERT INTO args VALUES (16,21,0,8)", "INSERT INTO args VALUES (16,22,1,105)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1,1000,'binder_transaction',0,1,1)", "INSERT INTO raw VALUES (2,1100,'binder_transaction_received',4096,1,2)", "INSERT INTO raw VALUES (3,1200,'binder_transaction_received',0,1,2)",
		"INSERT INTO raw VALUES (4,1300,'binder_transaction',0,1,3)", "INSERT INTO raw VALUES (5,1400,'binder_transaction_received',0,2,4)",
		"INSERT INTO raw VALUES (6,2000,'workqueue_execute_start',0,1,5)", "INSERT INTO raw VALUES (7,2100,'workqueue_execute_end',4096,1,5)", "INSERT INTO raw VALUES (8,2200,'workqueue_execute_end',0,1,5)",
		"INSERT INTO raw VALUES (9,2300,'workqueue_execute_start',0,1,6)", "INSERT INTO raw VALUES (10,2400,'workqueue_execute_end',0,1,6)",
		"INSERT INTO raw VALUES (11,3000,'dma_fence_wait_start',0,1,7)", "INSERT INTO raw VALUES (12,3100,'dma_fence_wait_end',4096,1,7)", "INSERT INTO raw VALUES (13,3200,'dma_fence_wait_end',0,1,7)",
		"INSERT INTO raw VALUES (14,3300,'dma_fence_wait_start',0,1,8)", "INSERT INTO raw VALUES (15,3400,'dma_fence_wait_end',0,1,8)",
		"INSERT INTO raw VALUES (16,4000,'block_rq_issue',0,1,9)", "INSERT INTO raw VALUES (17,4100,'block_rq_complete',0,1,11)", "INSERT INTO raw VALUES (18,4200,'block_rq_complete',0,1,10)",
		"INSERT INTO raw VALUES (19,4300,'block_rq_issue',0,1,12)", "INSERT INTO raw VALUES (20,4400,'block_rq_complete',0,2,13)",
		"INSERT INTO raw VALUES (21,5000,'scsi_dispatch_cmd_start',0,1,14)", "INSERT INTO raw VALUES (22,5100,'scsi_dispatch_cmd_done',0,1,15)", "INSERT INTO raw VALUES (23,5200,'scsi_dispatch_cmd_done',0,1,14)",
		"INSERT INTO raw VALUES (24,5300,'scsi_dispatch_cmd_start',0,1,16)", "INSERT INTO raw VALUES (25,5400,'scsi_dispatch_cmd_done',0,1,16)",
	}
	path := createTraceDBRawAuthorityFixture(t, statements)
	outPath := filepath.Join(t.TempDir(), "raw-five-family-freeze.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export five-family freeze fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, forbidden := range []string{"transaction=100", "work struct 0xa", "context=1", " 100 + 8", "dev=8,0"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("poisoned lane %q survived complete-set freeze:\n%s", forbidden, body)
		}
	}
	for _, want := range []string{"transaction=200", "work struct 0x14", "context=2", " 200 + 8", "dev=8,1"} {
		if strings.Count(body, want) != 2 {
			t.Fatalf("clean sibling %q did not retain both endpoints:\n%s", want, body)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatal(err)
	}
	if ipc := tracequery.BuildIPCGraph(idx, tracequery.Query{}); len(ipc.Edges) != 1 || ipc.Edges[0].TransactionID != 200 {
		t.Fatalf("binder anti-rescue/sibling parity failed: %+v", ipc)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{})
	workPairs, dmaPairs, blockPairs, storagePairs := 0, 0, 0, 0
	for _, item := range stats.WorkqueueActivity {
		workPairs += item.PairedCount
	}
	for _, item := range stats.DMAFenceActivity {
		dmaPairs += item.PairedCount
	}
	blockPairs = len(stats.IOLatencies)
	for _, item := range stats.StorageLatencyByLayer {
		if item.Layer == "scsi" {
			storagePairs += item.PairedCount
		}
	}
	if workPairs != 1 || dmaPairs != 1 || blockPairs != 1 || storagePairs != 1 {
		t.Fatalf("five-family tracequery parity work=%d dma=%d block=%d storage=%d stats=%+v coverage=%+v",
			workPairs, dmaPairs, blockPairs, storagePairs, stats, result.Coverage)
	}
}

func TestTraceDBRawGenericStorageUnsupportedKnownKeyPoisonsExactLaneOnly(t *testing.T) {
	path := createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)", "INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)", "INSERT INTO process VALUES (1,42,'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1,42,1,'worker',1,1)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1,'dev')", "INSERT INTO data_dict VALUES (2,'ino')",
		"INSERT INTO data_dict VALUES (3,'pos')", "INSERT INTO data_dict VALUES (4,'len')",
		"INSERT INTO data_dict VALUES (5,'flags')", "INSERT INTO data_dict VALUES (6,'tag')",
		"INSERT INTO data_dict VALUES (7,'lba')", "INSERT INTO data_dict VALUES (8,'opcode')",
		"INSERT INTO data_dict VALUES (100,'8,0')", "INSERT INTO data_dict VALUES (101,'8,1')",
		"INSERT INTO data_dict VALUES (102,'READ_10')", "INSERT INTO data_dict VALUES (103,'0x7')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1,1,1,100)", "INSERT INTO args VALUES (1,2,1,103)",
		"INSERT INTO args VALUES (1,3,0,0)", "INSERT INTO args VALUES (1,4,0,4096)", "INSERT INTO args VALUES (1,5,0,0)",
		"INSERT INTO args VALUES (2,6,0,-1)", "INSERT INTO args VALUES (2,1,1,101)",
		"INSERT INTO args VALUES (2,7,0,200)", "INSERT INTO args VALUES (2,4,0,8)", "INSERT INTO args VALUES (2,8,1,102)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1,1000,'ext4_da_write_begin',0,1,1)",
		"INSERT INTO raw VALUES (2,2000,'scsi_dispatch_cmd_start',0,1,2)",
		"INSERT INTO raw VALUES (3,3000,'scsi_dispatch_cmd_done',0,1,2)",
	})
	outPath := filepath.Join(t.TempDir(), "raw-generic-storage-locality.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatal(err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Contains(body, "ext4_da_write_begin") || strings.Count(body, "scsi_dispatch_cmd_") != 2 {
		t.Fatalf("unsupported exact ext4 lane over-poisoned storage sibling:\n%s\ncoverage=%+v", body, result.Coverage)
	}
	var rawCoverage TraceDBCoverage
	for _, item := range result.Coverage {
		if item.Family == "raw_ftrace" && item.Table == "raw" {
			rawCoverage = item
		}
	}
	if backend := rawCoverage.FieldSources["pairing_stage_backend"]; !strings.Contains(backend, "poisoned_lanes=1") || !strings.Contains(backend, "poisoned_families=0") {
		t.Fatalf("generic storage poison scope not exact: %+v", rawCoverage)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatal(err)
	}
	paired := 0
	for _, item := range tracequery.ComputeWindowStats(idx, tracequery.Query{}).StorageLatencyByLayer {
		if item.Layer == "scsi" {
			paired += item.PairedCount
		}
	}
	if paired != 1 {
		t.Fatalf("clean SCSI sibling did not survive exact ext4 quarantine: %d", paired)
	}
}
