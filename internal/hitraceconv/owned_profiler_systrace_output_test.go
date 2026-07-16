package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

type profilerWriterTestRow struct {
	tsNS      uint64
	seq       int
	body      string
	publisher profilerPairPublisherSlot
	text      bool
}

// profilerWriterCheckpointCancelContext counts only direct context polls made
// by the owned Profiler publication throat. Nested sorter, validator and
// platform-publication polls are deliberately excluded, so delay 0/1/2 binds
// exactly to post-publish/post-private-cleanup/post-receipt respectively.
// atomic.Int32 keeps the Context implementation safe if a future validator
// starts polling from a helper goroutine.
type profilerWriterCheckpointCancelContext struct {
	context.Context
	delay       int32
	directPolls atomic.Int32
}

func (ctx *profilerWriterCheckpointCancelContext) Err() error {
	if ctx == nil {
		return context.Canceled
	}
	var callers [1]uintptr
	if runtime.Callers(2, callers[:]) == 1 {
		frame, _ := runtime.CallersFrames(callers[:]).Next()
		if strings.HasSuffix(frame.Function, ".writeValidatedOwnedProfilerSystraceWithLedger") {
			poll := ctx.directPolls.Add(1)
			if poll >= 2+ctx.delay {
				return context.Canceled
			}
		}
	}
	if ctx.Context != nil {
		return ctx.Context.Err()
	}
	return nil
}

func profilerWriterKnownBody(pid int) string {
	return "sched_wakeup: comm=app pid=" + strconv.Itoa(pid) + " prio=20 target_cpu=001"
}

func profilerWriterUnknownBody(label string) string {
	return "vendor_profiler_opaque: label=" + label
}

func newProfilerWriterTestSink(
	t testing.TB,
	threshold int,
	rows []profilerWriterTestRow,
) *traceDBRowSink {
	t.Helper()
	source := profilerSourceLifecycleFile(t)
	sink := newProfilerSourceLifecycleCapture(t, source, threshold, traceDBRowSinkOptions{})
	if err := sink.enableProfilerTraceClassification(); err != nil {
		_ = sink.cleanup()
		t.Fatal(err)
	}
	for _, row := range rows {
		if !sink.beginPairRowCensusForPublisher(row.publisher) {
			_ = sink.cleanup()
			t.Fatalf("begin publisher %d", row.publisher)
		}
		if row.text && !sink.beginProfilerTextMessage() {
			_ = sink.cleanup()
			t.Fatal("begin profiler text message")
		}
		line := traceDBFormatLine("worker", 7, 7, 1, int64(row.tsNS), 0, 0, row.body)
		if err := sink.add(renderedRow{tsNS: row.tsNS, seq: row.seq, line: line}); err != nil {
			_ = sink.cleanup()
			t.Fatalf("add profiler writer row: %v", err)
		}
		if row.text {
			if err := sink.endProfilerTextMessage(1); err != nil {
				_ = sink.cleanup()
				t.Fatal(err)
			}
		}
		_ = sink.endPairRowCensus()
	}
	if err := sink.sealProfilerCaptureContext(context.Background()); err != nil {
		_ = sink.cleanup()
		t.Fatalf("seal profiler writer sink: %v", err)
	}
	return sink
}

func profilerWriterTerminal(rows int) profilerTerminalPublicationLedger {
	return profilerTerminalPublicationLedger{rows: profilerTerminalPublicationCounts{
		staged: uint64(rows), published: uint64(rows),
	}}
}

func publishProfilerWriterTestRows(
	t testing.TB,
	threshold int,
	path string,
	rows []profilerWriterTestRow,
) (profilerSystracePublication, *conversionFileLedger) {
	t.Helper()
	sink := newProfilerWriterTestSink(t, threshold, rows)
	ledger, err := newConversionFileLedger()
	if err != nil {
		_ = sink.cleanup()
		t.Fatal(err)
	}
	publication, err := writeValidatedOwnedProfilerSystraceWithLedger(
		context.Background(), path, sink, profilerContainerExtraction{}, profilerWriterTerminal(len(rows)), ledger,
	)
	if err != nil {
		_ = ledger.cleanup()
		t.Fatalf("publish profiler writer rows: %v", err)
	}
	return publication, ledger
}

func TestOwnedProfilerSystracePublicationReceiptMatrix(t *testing.T) {
	tests := []struct {
		name           string
		rows           []profilerWriterTestRow
		known, unknown int
		ready          bool
	}{
		{name: "structured known", rows: []profilerWriterTestRow{
			{tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(10), publisher: profilerPairPublisherExactFtrace},
		}, known: 1, ready: true},
		{name: "strict text known", rows: []profilerWriterTestRow{
			{tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(11), publisher: profilerPairPublisherExactFtrace, text: true},
		}, known: 1, ready: true},
		{name: "bytrace intentional unknown", rows: []profilerWriterTestRow{
			{tsNS: 1_000_000, seq: 1, body: profilerWriterUnknownBody("bytrace"), publisher: profilerPairPublisherBytrace, text: true},
		}, unknown: 1},
		{name: "session intentional unknown", rows: []profilerWriterTestRow{
			{tsNS: 1_000_000, seq: 1, body: profilerWriterUnknownBody("session"), publisher: profilerPairPublisherSession},
		}, unknown: 1},
		{name: "mixed", rows: []profilerWriterTestRow{
			{tsNS: 2_000_000, seq: 2, body: profilerWriterUnknownBody("mixed"), publisher: profilerPairPublisherOtherText, text: true},
			{tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(12), publisher: profilerPairPublisherOtherText, text: true},
		}, known: 1, unknown: 1, ready: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "profiler.systrace")
			publication, ledger := publishProfilerWriterTestRows(t, 32, path, test.rows)
			t.Cleanup(func() { _ = ledger.cleanup() })
			artifact := publication.Artifact
			capability := artifact.Trace
			if artifact.Type != ArtifactSystrace || artifact.Path != path ||
				artifact.Converter != converterVersion+"+openharmony-profiler" || capability == nil ||
				capability.ProviderKind != traceProviderKindBuiltin || capability.ProviderName != traceProviderNameBuiltinModern ||
				capability.OutputFormat != ownedSystraceOutputFormat ||
				capability.ValidationProfile != string(ownedTraceValidationProfiler) ||
				capability.Rows != len(test.rows) || capability.Known != test.known ||
				capability.AuthoritativeKnown != test.known || capability.AdvisoryRows != 0 ||
				capability.IntentionalUnknown != test.unknown || capability.IntentionalHeaderOnly != 0 ||
				capability.TraceQueryReady != test.ready {
				t.Fatalf("Profiler receipt capability drifted: artifact=%+v capability=%+v", artifact, capability)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(body)
			if artifact.Bytes != int64(len(body)) || artifact.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatalf("Profiler byte receipt drifted: artifact=%+v bytes=%d", artifact, len(body))
			}
			coverage := publication.TraceCoverage
			headerLines := strings.Count(systraceHeader, "\n")
			if coverage.Family != tracebundle.SystraceReceiptFamily ||
				coverage.Table != tracebundle.SystraceReceiptTableProfiler ||
				coverage.Role != tracebundle.SystraceReceiptRole || coverage.ArtifactPath != path ||
				!coverage.Found || coverage.Error != "" || coverage.RowsRead != headerLines+len(test.rows) ||
				coverage.RowsEmitted != test.known+test.unknown {
				t.Fatalf("Profiler receipt coverage drifted: %+v", coverage)
			}
			published, ok := ledger.ownedTraceValidation(artifact.traceReceiptBindingPath)
			if !ok || published.artifactPath != path || published.receipt.kind != ownedTraceValidationProfiler ||
				published.receipt.rows != len(test.rows) || published.receipt.known != test.known ||
				published.receipt.authoritativeKnown != test.known || published.receipt.unknown != test.unknown ||
				published.receipt.advisory != 0 || published.receipt.unparsed != 0 ||
				published.receipt.queryReady != test.ready {
				t.Fatalf("Profiler ledger receipt drifted: published=%+v ok=%t", published, ok)
			}
			decision, err := traceProviderPublished(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinModern),
					Options{TraceEngine: traceEngineBuiltin}, "input.htrace", artifact.Path), artifact, ledger,
			)
			if err != nil || !decision.Selected || !decision.Attempted || !decision.Succeeded ||
				decision.ArtifactPath != artifact.Path || decision.TraceQueryReady != test.ready {
				t.Fatalf("Profiler provider/receipt parity drifted: decision=%+v err=%v", decision, err)
			}
		})
	}
}

func TestOwnedProfilerSystraceForcedSpillReceiptParity(t *testing.T) {
	rows := []profilerWriterTestRow{
		{tsNS: 3_000_000, seq: 3, body: profilerWriterUnknownBody("late"), publisher: profilerPairPublisherBytrace, text: true},
		{tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(21), publisher: profilerPairPublisherExactFtrace},
		{tsNS: 2_000_000, seq: 2, body: profilerWriterKnownBody(22), publisher: profilerPairPublisherSession},
	}
	dir := t.TempDir()
	memory, memoryLedger := publishProfilerWriterTestRows(t, 1024, filepath.Join(dir, "memory.systrace"), rows)
	t.Cleanup(func() { _ = memoryLedger.cleanup() })
	spill, spillLedger := publishProfilerWriterTestRows(t, 1, filepath.Join(dir, "spill.systrace"), rows)
	t.Cleanup(func() { _ = spillLedger.cleanup() })
	memoryBody, err := os.ReadFile(memory.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	spillBody, err := os.ReadFile(spill.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(memoryBody, spillBody) || memory.Artifact.SHA256 != spill.Artifact.SHA256 ||
		memory.Artifact.Bytes != spill.Artifact.Bytes || !reflect.DeepEqual(memory.Artifact.Trace, spill.Artifact.Trace) ||
		memory.Stats.RowsWritten != spill.Stats.RowsWritten || memory.Stats.RowsWithheld != spill.Stats.RowsWithheld ||
		spill.Stats.SpillChunks == 0 {
		t.Fatalf("Profiler memory/spill receipt parity drifted:\nmemory=%+v\nspill=%+v", memory, spill)
	}
	wantOrder := []string{"pid=21", "pid=22", "label=late"}
	last := -1
	for _, token := range wantOrder {
		at := bytes.Index(memoryBody, []byte(token))
		if at <= last {
			t.Fatalf("Profiler final row order drifted at %q:\n%s", token, memoryBody)
		}
		last = at
	}
}

func TestProfilerOwnedRowProfileRejectsOpenOrContradictoryProvenance(t *testing.T) {
	knownLine := traceDBFormatLine("worker", 7, 7, 1, 1_000_000, 0, 0, profilerWriterKnownBody(30))
	tests := []struct {
		name       string
		provenance profilerPairRowProvenance
	}{
		{name: "source neutral", provenance: profilerPairRowProvenance{}},
		{name: "active class none", provenance: profilerPairRowProvenance{PublisherSlot: profilerPairPublisherSession}},
		{name: "structured claims unknown", provenance: profilerPairRowProvenance{
			PublisherSlot: profilerPairPublisherExactFtrace, TraceClass: profilerTraceClassTextIntentionalUnknown,
		}},
		{name: "text claims structured", provenance: profilerPairRowProvenance{
			TextMessageOrdinal: 1, PublisherSlot: profilerPairPublisherBytrace,
			Flags: profilerPairRowProvenanceText, TraceClass: profilerTraceClassStructuredKnown,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := &profilerOwnedRowProfileBuilder{}
			err := builder.observe(traceDBFinalRowObservation{LineNo: strings.Count(systraceHeader, "\n") + 1,
				Row: traceDBStoredRow{line: knownLine, provenance: test.provenance}})
			reason, _, typed := ownedTraceOutputInvariantReason(err)
			if !typed || reason != profilerOwnedRowProvenanceInvalid || builder.rows != 0 || builder.known != 0 {
				t.Fatalf("open/contradictory provenance was not rejected atomically: reason=%q err=%v builder=%+v", reason, err, builder)
			}
		})
	}
}

func TestOwnedProfilerSystraceCollisionAndPreCancelLeaveNoPublication(t *testing.T) {
	t.Run("collision", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "profiler.systrace")
		original := []byte("customer-owned-generation\n")
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatal(err)
		}
		sink := newProfilerWriterTestSink(t, 8, []profilerWriterTestRow{{
			tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(40), publisher: profilerPairPublisherExactFtrace,
		}})
		defer sink.cleanup()
		ledger, err := newConversionFileLedger()
		if err != nil {
			t.Fatal(err)
		}
		defer ledger.cleanup()
		_, err = writeValidatedOwnedProfilerSystraceWithLedger(
			context.Background(), path, sink, profilerContainerExtraction{}, profilerWriterTerminal(1), ledger,
		)
		var publication *ownedTracePublicationError
		got, readErr := os.ReadFile(path)
		if err == nil || !errors.As(err, &publication) || !bytes.Equal(got, original) || readErr != nil || len(ledger.created) != 0 {
			t.Fatalf("Profiler collision boundary drifted: err=%v got=%q read=%v ledger=%+v", err, got, readErr, ledger.created)
		}
		assertNoProfilerStagingResidue(t, dir)
	})

	t.Run("pre canceled", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "profiler.systrace")
		sink := newProfilerWriterTestSink(t, 8, []profilerWriterTestRow{{
			tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(41), publisher: profilerPairPublisherExactFtrace,
		}})
		defer sink.cleanup()
		ledger, err := newConversionFileLedger()
		if err != nil {
			t.Fatal(err)
		}
		defer ledger.cleanup()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		publication, err := writeValidatedOwnedProfilerSystraceWithLedger(
			ctx, path, sink, profilerContainerExtraction{}, profilerWriterTerminal(1), ledger,
		)
		if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(publication, profilerSystracePublication{}) ||
			len(ledger.created) != 0 {
			t.Fatalf("Profiler pre-cancel boundary drifted: publication=%+v err=%v ledger=%+v", publication, err, ledger.created)
		}
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("pre-canceled Profiler writer left public output: %v", statErr)
		}
		assertNoProfilerStagingResidue(t, dir)
	})
}

func TestOwnedProfilerSystraceTerminalCountMismatchLeavesNoClaim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "terminal-mismatch.systrace")
	sink := newProfilerWriterTestSink(t, 8, []profilerWriterTestRow{{
		tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(42), publisher: profilerPairPublisherExactFtrace,
	}})
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ledger.cleanup() })

	publication, err := writeValidatedOwnedProfilerSystraceWithLedger(
		context.Background(), path, sink, profilerContainerExtraction{}, profilerWriterTerminal(2), ledger,
	)
	var publicationErr *ownedTracePublicationError
	reason, typed := traceDBOutputInvariantReason(err)
	if !errors.As(err, &publicationErr) || publicationErr.Stage != "terminal_projection" ||
		!typed || reason != "profiler_terminal_publication_written_account_mismatch" ||
		!reflect.DeepEqual(publication.Artifact, Artifact{}) || publication.Stats.RowsWritten != 1 {
		t.Fatalf("Profiler terminal mismatch lost its hard identity: publication=%+v stage=%q reason=%q typed=%t err=%v",
			publication, publicationErrorStage(publicationErr), reason, typed, err)
	}
	assertProfilerFailedPublicationState(t, dir, path, sink, ledger, 0)
}

func TestOwnedProfilerSystracePostPublicationCancellationRollsBack(t *testing.T) {
	tests := []struct {
		name  string
		delay int32
		stage string
	}{
		{name: "post publish", delay: 0, stage: "post_publish_context"},
		{name: "post private cleanup", delay: 1, stage: "post_cleanup_context"},
		{name: "post receipt", delay: 2, stage: "post_receipt_context"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "canceled-after-publication.systrace")
			sink := newProfilerWriterTestSink(t, 8, []profilerWriterTestRow{{
				tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(43), publisher: profilerPairPublisherExactFtrace,
			}})
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ledger.cleanup() })
			ctx := &profilerWriterCheckpointCancelContext{
				Context: context.Background(), delay: test.delay,
			}

			publication, err := writeValidatedOwnedProfilerSystraceWithLedger(
				ctx, path, sink, profilerContainerExtraction{}, profilerWriterTerminal(1), ledger,
			)
			var publicationErr *ownedTracePublicationError
			if !errors.Is(err, context.Canceled) || !errors.As(err, &publicationErr) ||
				publicationErr.Stage != test.stage || !reflect.DeepEqual(publication.Artifact, Artifact{}) ||
				ctx.directPolls.Load() != 2+test.delay {
				t.Fatalf("Profiler %s cancellation boundary drifted: publication=%+v stage=%q polls=%d want=%d err=%v",
					test.name, publication, publicationErrorStage(publicationErr), ctx.directPolls.Load(), 2+test.delay, err)
			}
			// The published generation is retained only as one removed audit
			// tombstone; no live receipt or path claim may survive rollback.
			assertProfilerFailedPublicationState(t, dir, path, sink, ledger, 1)
		})
	}
}

func TestOwnedProfilerSystraceFinalRunReadFaultLeavesNoClaim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "final-run-read-fault.systrace")
	sink := newProfilerWriterTestSink(t, 1, []profilerWriterTestRow{{
		tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(44), publisher: profilerPairPublisherExactFtrace,
	}})
	want := errors.New("final-run-read-fault")
	var fired atomic.Bool
	sink.options.ops.fault = func(point, _ string) error {
		if point == "read" && fired.CompareAndSwap(false, true) {
			return want
		}
		return nil
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ledger.cleanup() })

	publication, err := writeValidatedOwnedProfilerSystraceWithLedger(
		context.Background(), path, sink, profilerContainerExtraction{}, profilerWriterTerminal(1), ledger,
	)
	var publicationErr *ownedTracePublicationError
	reason, typed := traceDBOutputInvariantReason(err)
	if !fired.Load() || !errors.Is(err, want) || !errors.As(err, &publicationErr) ||
		publicationErr.Stage != "finish_private" || !typed || reason != "trace_row_sort_run_read_failed" ||
		!reflect.DeepEqual(publication.Artifact, Artifact{}) || publication.Stats.RowsWritten != 0 {
		t.Fatalf("Profiler final-run read fault drifted: fired=%t publication=%+v stage=%q reason=%q typed=%t err=%v",
			fired.Load(), publication, publicationErrorStage(publicationErr), reason, typed, err)
	}
	assertProfilerFailedPublicationState(t, dir, path, sink, ledger, 0)
}

func TestTraceDBFinalRowObserverRejectsInvalidAuthorityShapes(t *testing.T) {
	noop := traceDBFinalRowObserver(func(traceDBFinalRowObservation) error { return nil })
	tests := []struct {
		name      string
		ordinary  bool
		observers []traceDBFinalRowObserver
	}{
		{name: "nil observer", observers: []traceDBFinalRowObserver{nil}},
		{name: "multiple observers", observers: []traceDBFinalRowObserver{noop, noop}},
		{name: "unclassified sink", ordinary: true, observers: []traceDBFinalRowObserver{noop}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sink *traceDBRowSink
			if test.ordinary {
				var err error
				sink, err = newTraceDBRowSink(t.TempDir(), 8)
				if err != nil {
					t.Fatal(err)
				}
				line := traceDBFormatLine("worker", 7, 7, 1, 1_000_000, 0, 0, profilerWriterKnownBody(45))
				if err := sink.add(renderedRow{tsNS: 1_000_000, seq: 1, line: line}); err != nil {
					t.Fatal(err)
				}
				if err := sink.prepareForPublication(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				sink = newProfilerWriterTestSink(t, 8, []profilerWriterTestRow{{
					tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(45), publisher: profilerPairPublisherExactFtrace,
				}})
			}
			t.Cleanup(func() { _ = sink.cleanup() })
			var output bytes.Buffer
			stats, err := sink.writeTo(context.Background(), &output, test.observers...)
			reason, typed := traceDBOutputInvariantReason(err)
			if !typed || reason != "trace_row_sort_final_observer_invalid" || output.Len() != 0 || stats.RowsWritten != 0 {
				t.Fatalf("%s observer authority drifted: stats=%+v reason=%q typed=%t bytes=%d err=%v",
					test.name, stats, reason, typed, output.Len(), err)
			}
		})
	}
}

func publicationErrorStage(err *ownedTracePublicationError) string {
	if err == nil {
		return ""
	}
	return err.Stage
}

func assertProfilerFailedPublicationState(
	t testing.TB,
	dir, path string,
	sink *traceDBRowSink,
	ledger *conversionFileLedger,
	wantLedgerRecords int,
) {
	t.Helper()
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("failed Profiler publication left a public output: %v", statErr)
	}
	if _, ok := ledger.ownedTraceValidation(path); ok {
		t.Fatalf("failed Profiler publication retained an active receipt: %+v", ledger.created)
	}
	live := 0
	for _, record := range ledger.created {
		if !record.removed {
			live++
		}
	}
	if len(ledger.created) != wantLedgerRecords || live != 0 {
		t.Fatalf("failed Profiler publication ledger drifted: records=%d want=%d live=%d ledger=%+v",
			len(ledger.created), wantLedgerRecords, live, ledger.created)
	}
	if err := ledger.validateOwnedPaths(); err != nil {
		t.Fatalf("failed Profiler publication left an invalid live ledger: %v", err)
	}
	if sink == nil {
		t.Fatal("failed Profiler publication lost its sorter fixture")
	}
	if sink.openRunFDs != 0 || sink.activeTempBytes != 0 || sink.liveTempBytes != 0 ||
		sink.stats.CurrentLiveTempBytes != 0 || len(sink.runs) != 0 || len(sink.artifacts) != 0 ||
		sink.sourceOrderSidecar.present() {
		t.Fatalf("failed Profiler publication retained sorter state: sink=%+v runs=%d artifacts=%d sidecar=%+v",
			sink, len(sink.runs), len(sink.artifacts), sink.sourceOrderSidecar)
	}
	assertNoProfilerStagingResidue(t, dir)
}

func assertNoProfilerStagingResidue(t testing.TB, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	prefix := strings.TrimSuffix(ownedProfilerSystraceStagingPattern, "*")
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			t.Fatalf("private Profiler staging residue survived: %s", entry.Name())
		}
	}
}

func TestConvertFileProfilerIntentionalUnknownReceiptParity(t *testing.T) {
	tests := []struct {
		name           string
		lines          []string
		known, unknown int
		ready          bool
	}{
		{name: "all unknown", lines: []string{
			"worker-7 (7) [001] .... 1.000000: vendor_profiler_opaque: value=one",
			"worker-7 (7) [001] .... 1.001000: vendor_profiler_opaque: value=two",
		}, unknown: 2},
		{name: "mixed", lines: []string{
			"worker-7 (7) [001] .... 1.000000: sched_wakeup: comm=app pid=7 prio=20 target_cpu=001",
			"worker-7 (7) [001] .... 1.001000: vendor_profiler_opaque: value=two",
		}, known: 1, unknown: 1, ready: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "profiler.htrace")
			output := filepath.Join(dir, "profiler.systrace")
			body := syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(strings.Join(test.lines, "\n"))))
			if err := os.WriteFile(input, body, 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(context.Background(), Options{
				InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
			})
			if err != nil {
				t.Fatalf("convert Profiler receipt fixture: %v", err)
			}
			var artifact *Artifact
			for index := range result.Artifacts {
				candidate := &result.Artifacts[index]
				if candidate.Type == ArtifactSystrace && candidate.Path == output {
					artifact = candidate
					break
				}
			}
			if artifact == nil || artifact.Trace == nil || artifact.Trace.Rows != len(test.lines) ||
				artifact.Trace.Known != test.known || artifact.Trace.AuthoritativeKnown != test.known ||
				artifact.Trace.IntentionalUnknown != test.unknown || artifact.Trace.TraceQueryReady != test.ready ||
				result.OutputPath != artifact.Path || result.OutputBytes != artifact.Bytes ||
				result.EventsWritten != artifact.Trace.Rows || result.UnknownEventCount != 0 {
				t.Fatalf("Profiler Result/receipt parity drifted: result=%+v artifact=%+v", result, artifact)
			}
			decisionFound := false
			coverageFound := false
			for _, decision := range result.TraceDecisions {
				if decision.ProviderName == traceProviderNameBuiltinModern {
					decisionFound = decision.Succeeded && decision.ArtifactPath == artifact.Path &&
						decision.TraceQueryReady == test.ready
				}
			}
			for _, coverage := range result.TraceCoverage {
				if coverage.Table == tracebundle.SystraceReceiptTableProfiler {
					coverageFound = coverage.Family == tracebundle.SystraceReceiptFamily &&
						coverage.Role == tracebundle.SystraceReceiptRole && coverage.ArtifactPath == artifact.Path &&
						coverage.RowsEmitted == test.known+test.unknown && coverage.Error == ""
				}
			}
			if !decisionFound || !coverageFound {
				t.Fatalf("Profiler receipt decision/coverage absent: decisions=%+v coverage=%+v", result.TraceDecisions, result.TraceCoverage)
			}
		})
	}
}

func TestOwnedProfilerSystracePublicationStructurePinned(t *testing.T) {
	body := sourceGenerationFunctionBody(t, "owned_profiler_systrace_output.go", "writeValidatedOwnedProfilerSystraceWithLedger")
	assertSourceGenerationOrder(t, body,
		"prepareSealedConversionPublicationTarget(outputPath, ownedProfilerSystraceStagingPattern)",
		"os.OpenFile(target.StagingPath",
		"sink.writeTo(ctx, io.MultiWriter(out, wireHasher), profileBuilder.observe)",
		"out.Sync()",
		"out.Close()",
		"validateProfilerTerminalWrittenProjection(extraction, terminal, sink)",
		"wireHasher.finish()",
		"profileBuilder.finish(publication.Stats, expectedRows, wire)",
		"target.stagingDir.AdoptRegularChild(target.finalLeaf, true)",
		"validateOwnedTraceOutput(",
		"publishValidatedOwnedTraceOutputNoReplace(",
		"\n\tcleanupErr := targetCleanup()\n\ttargetCleanup = nil",
		"newValidatedSystraceArtifact(",
	)
	for _, forbidden := range []string{
		"os.OpenFile(outputPath", "ledger.recordOpenFile(", "os.Lstat(outputPath)",
		"ledger.sealOwnedPath(", "traceProviderInventoryPublished(", "storageValid()",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Profiler receipt writer regained forbidden publication %q:\n%s", forbidden, body)
		}
	}

	sorter := sourceGenerationFunctionBody(t, "streamerdb_sorter.go", "writeTo")
	verifyAt := strings.Index(sorter, "sourceOrderPublication.verifyRunRecord(ctx, record)")
	publishableAt := strings.Index(sorter, "if !publishable")
	observerAt := strings.Index(sorter, "observer(traceDBFinalRowObservation")
	writeAt := strings.Index(sorter, "bw.WriteString(record.row.line)")
	if verifyAt < 0 || publishableAt <= verifyAt || observerAt <= publishableAt || writeAt <= observerAt ||
		!strings.Contains(sorter, "provenance.valid()") || strings.Contains(sorter, "provenance.storageValid()") {
		t.Fatalf("final-row observer left the authenticated publication throat: verify=%d publishable=%d observer=%d write=%d\n%s",
			verifyAt, publishableAt, observerAt, writeAt, sorter)
	}
}

func TestProfilerFinalRowObserverProductionCallsiteIsUnique(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	type observerCallsite struct {
		file  string
		line  int
		exact bool
	}
	var calls []observerCallsite
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse production source %s: %v", name, parseErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "writeTo" || len(call.Args) < 3 {
				return true
			}
			position := fset.Position(call.Pos())
			receiver, receiverOK := selector.X.(*ast.Ident)
			observer, observerOK := call.Args[2].(*ast.SelectorExpr)
			builder, builderOK := func() (*ast.Ident, bool) {
				if !observerOK {
					return nil, false
				}
				ident, ok := observer.X.(*ast.Ident)
				return ident, ok
			}()
			calls = append(calls, observerCallsite{
				file: name,
				line: position.Line,
				exact: len(call.Args) == 3 && name == "owned_profiler_systrace_output.go" &&
					receiverOK && receiver.Name == "sink" && observerOK && observer.Sel.Name == "observe" &&
					builderOK && builder.Name == "profileBuilder",
			})
			return true
		})
	}
	if len(calls) != 1 || !calls[0].exact {
		t.Fatalf("production final-row observer authority is not unique: %+v", calls)
	}
}
