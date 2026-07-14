package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type profilerSourceOrderDirectPollContext struct {
	context.Context
	cancelAt int
	polls    int
	err      error
}

func (ctx *profilerSourceOrderDirectPollContext) Err() error {
	if ctx == nil {
		return context.Canceled
	}
	var callers [8]uintptr
	frames := runtime.CallersFrames(callers[:runtime.Callers(2, callers[:])])
	caller, _ := frames.Next()
	if strings.HasSuffix(caller.Function, ".(*traceDBRowSink).addContext") {
		ctx.polls++
		if ctx.err != nil && ctx.cancelAt > 0 && ctx.polls >= ctx.cancelAt {
			return ctx.err
		}
	}
	if ctx.Context != nil {
		return ctx.Context.Err()
	}
	return nil
}

func profilerSourceLifecycleFile(t testing.TB) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.htrace")
	if err := os.WriteFile(path, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newProfilerSourceLifecycleCapture(t testing.TB, source string, threshold int,
	options traceDBRowSinkOptions,
) *traceDBRowSink {
	t.Helper()
	sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), threshold, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.openProfilerCapture(source); err != nil {
		_ = sink.cleanup()
		t.Fatal(err)
	}
	return sink
}

func requireProfilerSourceLifecycleReason(t testing.TB, err error, want string) {
	t.Helper()
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != want {
		t.Fatalf("invariant reason=%q ok=%t want=%q err=%T %v", reason, ok, want, err, err)
	}
}

func TestProfilerSourceOrderInactiveSorterKeepsZeroProof(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	if !sink.profilerSourceProof.pristine() {
		t.Fatalf("new inactive sink allocated source proof: %+v", sink.profilerSourceProof)
	}
	if err := sink.add(renderedRow{tsNS: 4, seq: 7, line: "inactive-row"}); err != nil {
		t.Fatal(err)
	}
	if !sink.profilerSourceProof.pristine() {
		t.Fatalf("inactive add hashed source proof: %+v", sink.profilerSourceProof)
	}
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RowsAccepted != 1 || stats.RowsWritten != 1 ||
		!bytes.Contains(output.Bytes(), []byte("inactive-row\n")) {
		t.Fatalf("inactive sorter output drifted: stats=%+v output=%q", stats, output.String())
	}
	if !sink.profilerSourceProof.pristine() {
		t.Fatalf("inactive prepare/write/cleanup allocated or retired proof: %+v", sink.profilerSourceProof)
	}
	if sink.sourceOrderSidecar.present() || stats.SourceSidecarLogicalBytes != 0 ||
		stats.SourceSidecarPhysicalBytes != 0 {
		t.Fatalf("inactive sorter allocated source-order sidecar: manifest=%+v stats=%+v",
			sink.sourceOrderSidecar, stats)
	}
	if _, _, err := sink.expectedProfilerSourceOrderProof(); err == nil {
		t.Fatal("inactive sorter exposed a Profiler source proof")
	} else {
		requireProfilerSourceLifecycleReason(t, err, "profiler_source_order_proof_inactive")
	}
}

func TestProfilerSourceOrderOpenFailureIsRetryableAndAtomic(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()

	source := filepath.Join(t.TempDir(), "late-capture.htrace")
	if err := sink.openProfilerCapture(source); err == nil {
		t.Fatal("missing source unexpectedly opened")
	}
	if sink.captureLifecycle != profilerCaptureInactive || sink.captureSource != "" ||
		!sink.profilerSourceProof.pristine() {
		t.Fatalf("failed open left capture/proof residue: lifecycle=%d source=%q proof=%+v",
			sink.captureLifecycle, sink.captureSource, sink.profilerSourceProof)
	}
	if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatalf("retry after source creation failed: %v", err)
	}
	if sink.captureLifecycle != profilerCaptureOpen || !sink.profilerSourceProof.active ||
		sink.profilerSourceProof.frozen || sink.profilerSourceProof.retired ||
		sink.profilerSourceProof.count != 0 || sink.profilerSourceProof.hasher == nil ||
		len(sink.profilerSourceProof.scratch) != profilerContextByteCheckpointBytes ||
		sink.profilerSourceProof.state != profilerSourceOrderInitialState() {
		t.Fatalf("successful retry did not atomically activate proof: lifecycle=%d proof=%+v",
			sink.captureLifecycle, sink.profilerSourceProof)
	}
}

func TestProfilerSourceOrderOpenRejectsRetiredAndPrePoisonedSinks(t *testing.T) {
	source := profilerSourceLifecycleFile(t)

	t.Run("pre-poisoned", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 32)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		sink.poisonPairKind(pairRenderF2FS)
		err = sink.openProfilerCapture(source)
		requireProfilerSourceLifecycleReason(t, err, "profiler_capture_open_state_invalid")
		if sink.captureLifecycle != profilerCaptureInactive || sink.captureSource != "" ||
			!sink.profilerSourceProof.pristine() {
			t.Fatalf("pre-poisoned open left capture proof: lifecycle=%d source=%q proof=%+v",
				sink.captureLifecycle, sink.captureSource, sink.profilerSourceProof)
		}
	})

	t.Run("cleaned-before-open", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 32)
		if err != nil {
			t.Fatal(err)
		}
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		err = sink.openProfilerCapture(source)
		requireProfilerSourceLifecycleReason(t, err, "profiler_capture_open_state_invalid")
		if sink.captureLifecycle != profilerCaptureInactive || sink.captureSource != "" ||
			!sink.profilerSourceProof.pristine() || sink.artifacts != nil {
			t.Fatalf("cleaned sink was revived: lifecycle=%d source=%q artifacts=%v proof=%+v",
				sink.captureLifecycle, sink.captureSource, sink.artifacts, sink.profilerSourceProof)
		}
	})
}

func TestProfilerSourceOrderZeroRowSealFreezesDeterministicRoot(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32, traceDBRowSinkOptions{})
	if _, _, err := sink.expectedProfilerSourceOrderProof(); err == nil {
		t.Fatal("open zero-row capture exposed an unfrozen proof")
	} else {
		requireProfilerSourceLifecycleReason(t, err, "profiler_source_order_proof_not_frozen")
	}
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	count, root, err := sink.expectedProfilerSourceOrderProof()
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || root == ([sha256.Size]byte{}) || !sink.profilerSourceProof.frozen ||
		sink.profilerSourceProof.retired || sink.profilerSourceProof.state != profilerSourceOrderInitialState() ||
		sink.captureLifecycle != profilerCaptureSealed || !sink.prepared || len(sink.runs) != 0 {
		t.Fatalf("zero-row seal state drifted: count=%d root=%x lifecycle=%d prepared=%t runs=%d proof=%+v",
			count, root, sink.captureLifecycle, sink.prepared, len(sink.runs), sink.profilerSourceProof)
	}
	if err := sink.cleanup(); err != nil {
		t.Fatal(err)
	}
	if !sink.profilerSourceProof.retired || !sink.profilerSourceProof.frozen ||
		sink.profilerSourceProof.expectedCount != count || sink.profilerSourceProof.expectedRoot != root ||
		sink.profilerSourceProof.hasher != nil || sink.profilerSourceProof.scratch != nil {
		t.Fatalf("zero-row cleanup lost frozen snapshot or retained scratch: %+v", sink.profilerSourceProof)
	}
}

func TestProfilerSourceOrderNoRowDeltaDoesNotAdvanceProof(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32, traceDBRowSinkOptions{})
		defer sink.cleanup()
		beforeState := sink.profilerSourceProof.state
		beforeCount := sink.profilerSourceProof.count
		var delta traceDBProfilerEventDelta
		delta.poisonKind(pairRenderF2FS)
		if err := sink.commitProfilerEventDeltaContext(context.Background(), delta); err != nil {
			t.Fatal(err)
		}
		if !sink.poisoned[pairRenderF2FS] || sink.profilerSourceProof.count != beforeCount ||
			sink.profilerSourceProof.state != beforeState || sink.profilerSourceProof.prepared {
			t.Fatalf("no-row delta drifted row proof: poisoned=%t before=%d/%x after=%d/%x prepared=%t",
				sink.poisoned[pairRenderF2FS], beforeCount, beforeState, sink.profilerSourceProof.count,
				sink.profilerSourceProof.state, sink.profilerSourceProof.prepared)
		}
	})

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(want.Error(), func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32, traceDBRowSinkOptions{})
			defer sink.cleanup()
			beforeState := sink.profilerSourceProof.state
			ctx := &profilerByteCancelAfterPollContext{
				Context: context.Background(), cancelAt: 1, err: want,
			}
			var delta traceDBProfilerEventDelta
			delta.poisonKind(pairRenderF2FS)
			err := sink.commitProfilerEventDeltaContext(ctx, delta)
			if err != want || sink.poisoned[pairRenderF2FS] ||
				sink.profilerSourceProof.count != 0 || sink.profilerSourceProof.state != beforeState ||
				sink.profilerSourceProof.prepared {
				t.Fatalf("canceled no-row delta drifted state: err=%v want_cause=%v poisoned=%t proof=%+v",
					err, want, sink.poisoned[pairRenderF2FS], sink.profilerSourceProof)
			}
		})
	}
}

func TestProfilerSourceOrderNegativeSequencePreservesCommittedPrefix(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32, traceDBRowSinkOptions{})
	defer sink.cleanup()
	if err := sink.addProfilerEventContext(context.Background(),
		renderedRow{tsNS: 10, seq: 7, line: "committed-prefix"}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	beforeState := sink.profilerSourceProof.state
	beforeCount := sink.profilerSourceProof.count
	var delta traceDBProfilerEventDelta
	delta.poisonKind(pairRenderF2FS)
	err := sink.addProfilerEventContext(context.Background(),
		renderedRow{tsNS: 11, seq: -1, line: "rejected-current"}, delta)
	requireProfilerSourceLifecycleReason(t, err, "profiler_source_order_proof_sequence_invalid")
	if sink.stats.RowsAccepted != 1 || sink.nextIngestOrdinal != 1 || len(sink.rows) != 1 ||
		sink.rows[0].line != "committed-prefix" || sink.poisoned[pairRenderF2FS] ||
		sink.profilerSourceProof.count != beforeCount || sink.profilerSourceProof.state != beforeState ||
		sink.profilerSourceProof.prepared {
		t.Fatalf("negative sequence damaged prefix/current transaction: stats=%+v next=%d rows=%+v poisoned=%t proof=%+v",
			sink.stats, sink.nextIngestOrdinal, sink.rows, sink.poisoned[pairRenderF2FS],
			sink.profilerSourceProof)
	}
}

func TestProfilerSourceOrderHashAndFinalPollCancellationIsAtomic(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for _, cancelAt := range []int{2, 3, 4} {
			t.Run(want.Error()+"/hash-poll-"+string(rune('0'+cancelAt)), func(t *testing.T) {
				sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32, traceDBRowSinkOptions{})
				defer sink.cleanup()
				beforeState := sink.profilerSourceProof.state
				ctx := &profilerGenericTransactionContext{
					Context: context.Background(), targetContains: ".prepareRowContext",
					cancelAt: cancelAt, err: want,
				}
				var delta traceDBProfilerEventDelta
				delta.poisonKind(pairRenderF2FS)
				err := sink.addProfilerEventContext(ctx, renderedRow{
					tsNS: 1, seq: 1,
					line: strings.Repeat("x", 2*profilerContextByteCheckpointBytes+1),
				}, delta)
				if err != want || ctx.polls != cancelAt || sink.stats.RowsAccepted != 0 ||
					sink.nextIngestOrdinal != 0 || len(sink.rows) != 0 || sink.poisoned[pairRenderF2FS] ||
					sink.profilerSourceProof.count != 0 || sink.profilerSourceProof.state != beforeState ||
					sink.profilerSourceProof.prepared {
					t.Fatalf("hash cancellation leaked current transaction: err=%T %v want=%v polls=%d/%d stats=%+v next=%d rows=%d poisoned=%t proof=%+v",
						err, err, want, ctx.polls, cancelAt, sink.stats, sink.nextIngestOrdinal,
						len(sink.rows), sink.poisoned[pairRenderF2FS], sink.profilerSourceProof)
				}
			})
		}

		t.Run(want.Error()+"/add-final-poll", func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32, traceDBRowSinkOptions{})
			defer sink.cleanup()
			beforeState := sink.profilerSourceProof.state
			ctx := &profilerSourceOrderDirectPollContext{
				Context: context.Background(), cancelAt: 2, err: want,
			}
			var delta traceDBProfilerEventDelta
			delta.poisonKind(pairRenderF2FS)
			err := sink.addProfilerEventContext(ctx,
				renderedRow{tsNS: 1, seq: 1, line: "prepared-before-final-poll"}, delta)
			if err != want || ctx.polls != 2 || sink.stats.RowsAccepted != 0 ||
				sink.nextIngestOrdinal != 0 || len(sink.rows) != 0 || sink.poisoned[pairRenderF2FS] ||
				sink.profilerSourceProof.count != 0 || sink.profilerSourceProof.state != beforeState ||
				sink.profilerSourceProof.prepared {
				t.Fatalf("final poll cancellation leaked current transaction: err=%T %v want=%v polls=%d stats=%+v next=%d rows=%d poisoned=%t proof=%+v",
					err, err, want, ctx.polls, sink.stats, sink.nextIngestOrdinal, len(sink.rows),
					sink.poisoned[pairRenderF2FS], sink.profilerSourceProof)
			}
		})
	}
}

func TestProfilerSourceOrderProofSurvivesPublicationVerdicts(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*traceDBRowSink)
		check  func(*traceDBRowSink) bool
	}{
		{name: "family-withheld", mutate: func(sink *traceDBRowSink) { sink.poisonPairKind(pairRenderF2FS) },
			check: func(sink *traceDBRowSink) bool { return sink.poisoned[pairRenderF2FS] }},
		{name: "source-fail-closed", mutate: func(sink *traceDBRowSink) { sink.failCloseAllRows() },
			check: func(sink *traceDBRowSink) bool { return sink.allRowsFailClosed }},
	} {
		t.Run(test.name, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32, traceDBRowSinkOptions{})
			defer sink.cleanup()
			if err := sink.addProfilerEventContext(context.Background(),
				renderedRow{
					tsNS: 1, seq: 1, line: "accepted-before-verdict", pairKind: pairRenderF2FS,
					pairLane: "f2fs-lane-1", pairTable: "f2fs_write_begin",
					profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
				}, traceDBProfilerEventDelta{}); err != nil {
				t.Fatal(err)
			}
			beforeCount := sink.profilerSourceProof.count
			beforeState := sink.profilerSourceProof.state
			beforeRoot, ok := sink.profilerSourceProof.terminalDigest()
			if !ok {
				t.Fatal("accepted prefix lacks terminal proof")
			}
			test.mutate(sink)
			afterRoot, afterOK := sink.profilerSourceProof.terminalDigest()
			if !test.check(sink) || !afterOK || sink.profilerSourceProof.count != beforeCount ||
				sink.profilerSourceProof.state != beforeState || afterRoot != beforeRoot {
				t.Fatalf("%s verdict changed accepted-source proof: before=%d/%x/%x after=%d/%x/%x ok=%t",
					test.name, beforeCount, beforeState, beforeRoot, sink.profilerSourceProof.count,
					sink.profilerSourceProof.state, afterRoot, afterOK)
			}
		})
	}
}

func TestProfilerSourceOrderPrepareIOFailureFreezesAndRejectsLaterAdd(t *testing.T) {
	fault := errors.New("source-proof-create")
	sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32,
		traceDBRowSinkOptions{ops: traceDBRowSinkOps{fault: func(point, _ string) error {
			if point == "create" {
				return fault
			}
			return nil
		}}})
	defer sink.cleanup()
	if err := sink.addProfilerEventContext(context.Background(),
		renderedRow{tsNS: 1, seq: 1, line: "accepted-before-prepare"}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	if err := sink.prepareForPublication(context.Background()); !errors.Is(err, fault) {
		t.Fatalf("prepare error=%v want=%v", err, fault)
	}
	count, root, err := sink.expectedProfilerSourceOrderProof()
	if err != nil {
		t.Fatalf("I/O-failed prepare lost frozen expected proof: %v", err)
	}
	if count != 1 || !sink.profilerSourceProof.frozen || sink.prepared || sink.prepareFailure == nil {
		t.Fatalf("I/O-failed prepare freeze state drifted: count=%d root=%x prepared=%t failure=%v proof=%+v",
			count, root, sink.prepared, sink.prepareFailure, sink.profilerSourceProof)
	}
	err = sink.addProfilerEventContext(context.Background(),
		renderedRow{tsNS: 2, seq: 2, line: "must-not-append"}, traceDBProfilerEventDelta{})
	requireProfilerSourceLifecycleReason(t, err, "profiler_capture_mutation_after_source_freeze")
	countAfter, rootAfter, proofErr := sink.expectedProfilerSourceOrderProof()
	if proofErr != nil || countAfter != count || rootAfter != root || sink.stats.RowsAccepted != 1 ||
		sink.nextIngestOrdinal != 1 || sink.profilerSourceProof.prepared {
		t.Fatalf("post-failure add changed frozen prefix: err=%v proof=%d/%x want=%d/%x stats=%+v",
			proofErr, countAfter, rootAfter, count, root, sink.stats)
	}
}

func TestProfilerSourceOrderActiveSpillFaultsPreserveCompletedPrefix(t *testing.T) {
	for _, point := range []string{"create", "write", "stat"} {
		for _, phase := range []string{"next-row-preflush", "batch-tail"} {
			t.Run(point+"/"+phase, func(t *testing.T) {
				fault := errors.New("active-source-proof-" + point + "-" + phase)
				sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1,
					traceDBRowSinkOptions{ops: traceDBRowSinkOps{fault: func(got, _ string) error {
						if got == point {
							return fault
						}
						return nil
					}}})
				defer sink.cleanup()
				if err := sink.addProfilerEventContext(context.Background(),
					renderedRow{tsNS: 1, seq: 1, line: "completed-prefix"}, traceDBProfilerEventDelta{}); err != nil {
					t.Fatal(err)
				}
				beforeState := sink.profilerSourceProof.state
				beforeRoot, ok := sink.profilerSourceProof.terminalDigest()
				if !ok {
					t.Fatal("completed prefix lacks producer root")
				}

				var err error
				if phase == "batch-tail" {
					err = sink.flushTriggeredProfilerEventContext(context.Background())
				} else {
					var delta traceDBProfilerEventDelta
					delta.poisonKind(pairRenderF2FS)
					err = sink.addProfilerEventContext(context.Background(),
						renderedRow{tsNS: 2, seq: 2, line: "uncommitted-current"}, delta)
				}
				afterRoot, afterOK := sink.profilerSourceProof.terminalDigest()
				if !errors.Is(err, fault) || !afterOK || sink.stats.RowsAccepted != 1 ||
					sink.nextIngestOrdinal != 1 || sink.profilerSourceProof.count != 1 ||
					sink.profilerSourceProof.state != beforeState || afterRoot != beforeRoot ||
					sink.profilerSourceProof.prepared || sink.poisoned[pairRenderF2FS] {
					t.Fatalf("active %s %s fault split prefix/proof: err=%v want=%v ok=%t stats=%+v next=%d poisoned=%t before=%x/%x after=%+v/%x",
						point, phase, err, fault, afterOK, sink.stats, sink.nextIngestOrdinal,
						sink.poisoned[pairRenderF2FS], beforeState, beforeRoot,
						sink.profilerSourceProof, afterRoot)
				}
			})
		}
	}
}

func TestProfilerSourceOrderCleanupRetirementIsAtomicAndRetryable(t *testing.T) {
	t.Run("successful cleanup retires once", func(t *testing.T) {
		sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32, traceDBRowSinkOptions{})
		if err := sink.addProfilerEventContext(context.Background(),
			renderedRow{tsNS: 1, seq: 1, line: "cleanup-success"}, traceDBProfilerEventDelta{}); err != nil {
			t.Fatal(err)
		}
		root, ok := sink.profilerSourceProof.terminalDigest()
		if !ok {
			t.Fatal("live proof had no terminal digest")
		}
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		if !sink.profilerSourceProof.retired || !sink.profilerSourceProof.frozen ||
			sink.profilerSourceProof.expectedCount != 1 || sink.profilerSourceProof.expectedRoot != root ||
			sink.profilerSourceProof.hasher != nil || sink.profilerSourceProof.scratch != nil ||
			sink.artifacts != nil || sink.rows != nil || sink.rowIngestOrdinals != nil {
			t.Fatalf("successful cleanup did not retire/release proof atomically: %+v", sink.profilerSourceProof)
		}
		beforeCount := sink.profilerSourceProof.expectedCount
		beforeRoot := sink.profilerSourceProof.expectedRoot
		beforeState := sink.profilerSourceProof.state
		if err := sink.add(renderedRow{tsNS: 2, seq: 2, line: "after-retire"}); err == nil {
			t.Fatal("retired open capture accepted another row")
		} else {
			requireProfilerSourceLifecycleReason(t, err, "profiler_capture_mutation_after_source_freeze")
		}
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		if sink.profilerSourceProof.expectedCount != beforeCount ||
			sink.profilerSourceProof.expectedRoot != beforeRoot || sink.profilerSourceProof.state != beforeState ||
			!sink.profilerSourceProof.active || !sink.profilerSourceProof.frozen ||
			!sink.profilerSourceProof.retired || sink.profilerSourceProof.hasher != nil ||
			sink.profilerSourceProof.scratch != nil {
			t.Fatalf("idempotent cleanup changed retired snapshot: count=%d/%d root=%x/%x state=%x/%x proof=%+v",
				sink.profilerSourceProof.expectedCount, beforeCount, sink.profilerSourceProof.expectedRoot,
				beforeRoot, sink.profilerSourceProof.state, beforeState, sink.profilerSourceProof)
		}
	})

	t.Run("failed cleanup keeps live proof and run for retry", func(t *testing.T) {
		fault := errors.New("source-proof-remove")
		armed := false
		triggered := false
		sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1,
			traceDBRowSinkOptions{ops: traceDBRowSinkOps{fault: func(point, _ string) error {
				if armed && !triggered && point == "remove" {
					triggered = true
					return fault
				}
				return nil
			}}})
		if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "cleanup-retry"}); err != nil {
			t.Fatal(err)
		}
		if len(sink.runs) != 1 || len(sink.artifacts) != 1 {
			t.Fatalf("cleanup retry fixture did not spill: runs=%d artifacts=%d", len(sink.runs), len(sink.artifacts))
		}
		beforeState := sink.profilerSourceProof.state
		armed = true
		if err := sink.cleanup(); !errors.Is(err, fault) {
			t.Fatalf("cleanup error=%v want=%v", err, fault)
		}
		if sink.profilerSourceProof.retired || !sink.profilerSourceProof.frozen ||
			sink.profilerSourceProof.count != 1 || sink.profilerSourceProof.state != beforeState ||
			sink.profilerSourceProof.expectedCount != 1 ||
			sink.profilerSourceProof.hasher == nil ||
			len(sink.profilerSourceProof.scratch) != profilerContextByteCheckpointBytes ||
			len(sink.runs) != 1 || len(sink.artifacts) != 1 || sink.activeTempBytes == 0 || sink.liveTempBytes == 0 {
			t.Fatalf("failed cleanup half-cleared proof/run: runs=%d artifacts=%d active/live=%d/%d proof=%+v",
				len(sink.runs), len(sink.artifacts), sink.activeTempBytes, sink.liveTempBytes,
				sink.profilerSourceProof)
		}
		beforeRoot := sink.profilerSourceProof.expectedRoot
		if err := sink.add(renderedRow{tsNS: 2, seq: 2, line: "after-failed-cleanup"}); err == nil {
			t.Fatal("failed cleanup left the active capture mutable")
		} else {
			requireProfilerSourceLifecycleReason(t, err, "profiler_capture_mutation_after_source_freeze")
		}
		if sink.profilerSourceProof.expectedCount != 1 ||
			sink.profilerSourceProof.expectedRoot != beforeRoot || sink.profilerSourceProof.count != 1 ||
			sink.profilerSourceProof.state != beforeState {
			t.Fatalf("post-cleanup-failure add changed frozen proof: %+v", sink.profilerSourceProof)
		}
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		if !sink.profilerSourceProof.retired || !sink.profilerSourceProof.frozen ||
			sink.profilerSourceProof.expectedCount != 1 || sink.profilerSourceProof.hasher != nil ||
			sink.profilerSourceProof.scratch != nil || sink.runs != nil || sink.artifacts != nil ||
			sink.activeTempBytes != 0 || sink.liveTempBytes != 0 {
			t.Fatalf("cleanup retry did not finish retirement: runs=%d active/live=%d/%d proof=%+v",
				len(sink.runs), sink.activeTempBytes, sink.liveTempBytes, sink.profilerSourceProof)
		}
	})
}

func TestProfilerSourceOrderPostSealAddPreservesFrozenProof(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 32, traceDBRowSinkOptions{})
	defer sink.cleanup()
	if err := sink.addProfilerEventContext(context.Background(),
		renderedRow{tsNS: 1, seq: 1, line: "sealed-prefix"}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	count, root, err := sink.expectedProfilerSourceOrderProof()
	if err != nil {
		t.Fatal(err)
	}
	err = sink.addProfilerEventContext(context.Background(),
		renderedRow{tsNS: 2, seq: 2, line: "late-row"}, traceDBProfilerEventDelta{})
	requireProfilerSourceLifecycleReason(t, err, "profiler_capture_add_after_seal")
	countAfter, rootAfter, proofErr := sink.expectedProfilerSourceOrderProof()
	if proofErr != nil || countAfter != count || rootAfter != root || sink.stats.RowsAccepted != 1 ||
		sink.nextIngestOrdinal != 1 || !sink.allRowsFailClosed {
		t.Fatalf("post-seal add changed proof/prefix: err=%v proof=%d/%x want=%d/%x stats=%+v failclosed=%t",
			proofErr, countAfter, rootAfter, count, root, sink.stats, sink.allRowsFailClosed)
	}
}

func TestProfilerSourceOrderRootIgnoresSpillAndMergeTopology(t *testing.T) {
	source := profilerSourceLifecycleFile(t)
	rows := []renderedRow{
		{tsNS: 90, seq: 10, line: "ingest-0"},
		{tsNS: 10, seq: 11, line: "ingest-1"},
		{tsNS: 70, seq: 12, line: "ingest-2"},
		{tsNS: 20, seq: 13, line: "ingest-3"},
		{tsNS: 80, seq: 14, line: "ingest-4"},
		{tsNS: 30, seq: 15, line: "ingest-5"},
		{tsNS: 60, seq: 16, line: "ingest-6"},
		{tsNS: 40, seq: 17, line: "ingest-7"},
		{tsNS: 50, seq: 18, line: "ingest-8"},
	}
	cases := []struct {
		name          string
		threshold     int
		options       traceDBRowSinkOptions
		wantSpills    int
		minimumMerges int
	}{
		{name: "no-preseal-spill", threshold: 64, wantSpills: 1},
		{name: "tiny-one-level", threshold: 1, wantSpills: len(rows), minimumMerges: 1},
		{name: "tiny-multilevel", threshold: 1, options: traceDBRowSinkOptions{mergeFanIn: 2},
			wantSpills: len(rows), minimumMerges: 2},
	}
	var wantRoot [sha256.Size]byte
	for index, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(t, source, test.threshold, test.options)
			for _, row := range rows {
				if err := sink.addProfilerEventContext(context.Background(), row, traceDBProfilerEventDelta{}); err != nil {
					_ = sink.cleanup()
					t.Fatal(err)
				}
			}
			if err := sink.sealProfilerCapture(); err != nil {
				_ = sink.cleanup()
				t.Fatal(err)
			}
			count, root, err := sink.expectedProfilerSourceOrderProof()
			if err != nil {
				_ = sink.cleanup()
				t.Fatal(err)
			}
			if count != uint64(len(rows)) || sink.stats.RowsAccepted != len(rows) ||
				sink.stats.SpillChunks != test.wantSpills || sink.stats.MergePasses < test.minimumMerges {
				_ = sink.cleanup()
				t.Fatalf("topology fixture drifted: count=%d stats=%+v want_spills=%d min_merges=%d",
					count, sink.stats, test.wantSpills, test.minimumMerges)
			}
			if index == 0 {
				wantRoot = root
			} else if root != wantRoot {
				_ = sink.cleanup()
				t.Fatalf("spill/merge topology changed producer root: got=%x want=%x", root, wantRoot)
			}
			if err := sink.cleanup(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
