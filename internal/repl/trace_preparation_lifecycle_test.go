package repl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceLifecyclePendingText(t *testing.T, ctx context.Context) *traceinput.Preparation {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.sys")
	if err := os.WriteFile(path, []byte("worker-12 (12) [000] .... 9.000000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pending, err := traceinput.Begin(ctx, traceinput.Options{InputPath: path, PreviewBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pending.Discard() })
	return pending
}

func traceLifecycleAwait(t *testing.T, done <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal(message)
	}
}

func TestTracePreparationLifecycleCancelBeforeCommitRollsBackBinary(t *testing.T) {
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(t.TempDir(), "missing-trace-streamer"))
	op := newTracePreparationOperation()
	t.Cleanup(op.finish)
	dir := t.TempDir()
	source := filepath.Join(dir, "raw capture.sys")
	original := hmc17BinaryTraceTailFixture()
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	var outputDir string
	pending, err := traceinput.Begin(op.ctx, traceinput.Options{
		InputPath: source, RuntimeAnchor: filepath.Join(dir, ".codrax"), PreviewBytes: 1024,
		Progress: func(event hitraceconv.ProgressEvent) {
			if event.Stage == "trace_prepare" && event.Status == hitraceconv.ProgressStatusStarted {
				outputDir = filepath.Dir(event.OutputPath)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pending.Discard() })
	if outputDir == "" {
		t.Fatal("real binary preparation never acquired output ownership")
	}
	op.Cancel("/cancel")
	published := false
	err = op.commit(pending, func(*attachment.TraceMaterial) { published = true })
	if !errors.Is(err, context.Canceled) || published {
		t.Fatalf("canceled operation published material: published=%t err=%v", published, err)
	}
	if _, err := os.Stat(outputDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled unpublished binary output survived: %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("rollback changed the raw source: %v", err)
	}
	if op.cancelActive("late cancel") {
		t.Fatal("failed commit still accepted cancellation as an active operation")
	}
}

func TestTracePreparationLifecycleCommitPublicationWinsLateCancel(t *testing.T) {
	op := newTracePreparationOperation()
	t.Cleanup(op.finish)
	pending := traceLifecyclePendingText(t, op.ctx)
	publishing, release := make(chan struct{}), make(chan struct{})
	committed := make(chan error, 1)
	var material *attachment.TraceMaterial
	go func() {
		committed <- op.commit(pending, func(next *attachment.TraceMaterial) {
			material = next
			close(publishing)
			<-release
		})
	}()
	traceLifecycleAwait(t, publishing, "commit did not enter publication")
	if op.mu.TryLock() {
		op.mu.Unlock()
		close(release)
		t.Fatal("tuple publication was not serialized with cancellation")
	}
	canceled := make(chan bool, 1)
	go func() { canceled <- op.cancelActive("late cancel") }()
	close(release)
	if err := <-committed; err != nil {
		t.Fatal(err)
	}
	if <-canceled || op.ctx.Err() != nil {
		t.Fatal("late cancellation revoked a committed attachment")
	}
	if material == nil || material.Validate(context.Background(), material.Preview()) != nil {
		t.Fatal("committed material became unavailable")
	}
	select {
	case <-op.done:
		t.Fatal("publication signaled shutdown completion before lifecycle cleanup")
	default:
	}
	op.finish()
	op.finish() // shutdown/defer cleanup must remain idempotent
	traceLifecycleAwait(t, op.done, "finished lifecycle did not release shutdown")
	if err := material.Validate(context.Background(), material.Preview()); err != nil {
		t.Fatalf("finishing operation revoked published material: %v", err)
	}
}

func TestTracePreparationLifecycleConcurrentCancelCommitHasOneOutcome(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		op := newTracePreparationOperation()
		pending := traceLifecyclePendingText(t, op.ctx)
		start := make(chan struct{})
		committed, canceled := make(chan error, 1), make(chan bool, 1)
		var material *attachment.TraceMaterial
		go func() {
			<-start
			committed <- op.commit(pending, func(next *attachment.TraceMaterial) { material = next })
		}()
		go func() {
			<-start
			canceled <- op.cancelActive("racing cancel")
		}()
		close(start)
		err, cancellationAccepted := <-committed, <-canceled
		if err == nil {
			if cancellationAccepted || material == nil || op.ctx.Err() != nil {
				t.Fatalf("iteration %d: success and cancellation both won", iteration)
			}
		} else if !errors.Is(err, context.Canceled) || !cancellationAccepted || material != nil {
			t.Fatalf("iteration %d: inconsistent cancellation: err=%v accepted=%t material=%v", iteration, err, cancellationAccepted, material)
		}
		op.finish()
	}
}

func TestTracePreparationLifecycleTerminalOperationCannotRepublish(t *testing.T) {
	op := newTracePreparationOperation()
	t.Cleanup(op.finish)
	first := traceLifecyclePendingText(t, op.ctx)
	second := traceLifecyclePendingText(t, op.ctx)
	publications := 0
	publish := func(*attachment.TraceMaterial) { publications++ }
	if err := op.commit(first, publish); err != nil {
		t.Fatal(err)
	}
	if err := op.commit(second, publish); err == nil || publications != 1 {
		t.Fatalf("terminal operation republished: count=%d err=%v", publications, err)
	}
}

func TestTracePreparationLifecycleShutdownAcceptedBeforeFinishHoldsREPL(t *testing.T) {
	for _, committed := range []bool{false, true} {
		name := "unpublished"
		if committed {
			name = "published but still draining"
		}
		t.Run(name, func(t *testing.T) {
			op := newTracePreparationOperation()
			defer close(op.shutdownExit)
			var material *attachment.TraceMaterial
			if committed {
				pending := traceLifecyclePendingText(t, op.ctx)
				if err := op.commit(pending, func(next *attachment.TraceMaterial) { material = next }); err != nil {
					t.Fatal(err)
				}
			}
			if !op.requestShutdown() {
				t.Fatal("unfinished preparation rejected process shutdown")
			}
			if committed && op.ctx.Err() != nil {
				t.Fatal("shutdown retroactively canceled the published result")
			}
			if !committed && !errors.Is(op.ctx.Err(), context.Canceled) {
				t.Fatal("shutdown did not cancel unpublished preparation")
			}
			returned := make(chan struct{})
			go func() { op.finish(); close(returned) }()
			traceLifecycleAwait(t, op.done, "finish did not acknowledge completed rollback/input drain")
			select {
			case <-returned:
				t.Fatal("REPL continued before the signal owner exited")
			default:
			}
			if op.requestShutdown() {
				t.Fatal("finished operation accepted a new shutdown request")
			}
			if material != nil {
				if err := material.Validate(context.Background(), material.Preview()); err != nil {
					t.Fatalf("shutdown deleted published material: %v", err)
				}
			}
		})
	}
}

func TestTracePreparationLifecycleFinishRejectsLaterShutdown(t *testing.T) {
	op := newTracePreparationOperation()
	op.finish()
	if op.requestShutdown() {
		t.Fatal("late shutdown acquired a completed operation")
	}
	returned := make(chan struct{})
	go func() { op.finish(); close(returned) }()
	traceLifecycleAwait(t, returned, "late shutdown made an already completed finish block")
	select {
	case <-op.shutdownExit:
		t.Fatal("normal completion consumed the process-exit barrier")
	default:
	}
}

func TestTracePreparationLifecycleConcurrentShutdownFinishHasOneOwner(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		op := newTracePreparationOperation()
		start, returned := make(chan struct{}), make(chan struct{})
		accepted := make(chan bool, 1)
		go func() { <-start; op.finish(); close(returned) }()
		go func() { <-start; accepted <- op.requestShutdown() }()
		close(start)
		shutdownOwnsExit := <-accepted
		traceLifecycleAwait(t, op.done, "racing finish did not close done")
		if shutdownOwnsExit {
			select {
			case <-returned:
				t.Fatalf("iteration %d: accepted shutdown let the REPL return", iteration)
			default:
			}
		} else {
			traceLifecycleAwait(t, returned, "finish won but shutdown still blocked the REPL")
		}
		close(op.shutdownExit)
		traceLifecycleAwait(t, returned, "simulated process exit did not release the test lifecycle")
	}
}

type traceLifecycleRunner struct {
	runs, cancellations, steering, steeringDrains atomic.Int32
}

func (r *traceLifecycleRunner) Run(string, string, string) (*types.BusContext, error) {
	r.runs.Add(1)
	return nil, nil
}

func (r *traceLifecycleRunner) Cancel(string)    { r.cancellations.Add(1) }
func (r *traceLifecycleRunner) IsCanceled() bool { return r.cancellations.Load() != 0 }
func (r *traceLifecycleRunner) PushSteeringNote(string) bool {
	r.steering.Add(1)
	return true
}
func (r *traceLifecycleRunner) TakeUnconsumedSteeringNotes() []string {
	r.steeringDrains.Add(1)
	return []string{"old pipeline note must stay there"}
}

func TestTracePreparationLifecycleTTYTypedCancelDoesNotSteer(t *testing.T) {
	for _, command := range []string{"/cancel", "\\cancel", " /CaNcEl reason "} {
		t.Run(command, func(t *testing.T) {
			op := newTracePreparationOperation()
			t.Cleanup(op.finish)
			runner := &traceLifecycleRunner{}
			r := &REPL{runner: runner, out: io.Discard}
			w, _ := windowFromScript(t, "question for this capture\n"+command+"\n", r.tracePreparationCallbacks(op))
			traceLifecycleAwait(t, op.ctx.Done(), "typed cancel did not reach preparation")
			r.drainTracePreparationWindow(w)
			want := []pendingFollowUp{{Text: "question for this capture"}}
			if !reflect.DeepEqual(r.pendingFollowUps, want) {
				t.Fatalf("typed cancel leaked into replay or lost input: %+v", r.pendingFollowUps)
			}
			if runner.runs.Load() != 0 || runner.cancellations.Load() != 0 || runner.steering.Load() != 0 || runner.steeringDrains.Load() != 0 {
				t.Fatal("preparation input reached pipeline execution/cancellation/steering")
			}
		})
	}
}

func TestTracePreparationLifecycleTTYPasteCancelRemainsVerbatim(t *testing.T) {
	op := newTracePreparationOperation()
	t.Cleanup(op.finish)
	runner := &traceLifecycleRunner{}
	r := &REPL{runner: runner, out: io.Discard}
	const blob = "/cancel\n!this is pasted data\n/htrace clear"
	w, _ := windowFromScript(t, "\x1b[200~"+blob+"\x1b[201~", r.tracePreparationCallbacks(op))
	waitFor(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.pastes) == 1
	})
	r.drainTracePreparationWindow(w)
	if op.ctx.Err() != nil || runner.cancellations.Load() != 0 || runner.steering.Load() != 0 || runner.steeringDrains.Load() != 0 {
		t.Fatal("bracketed-paste data canceled preparation or reached old pipeline")
	}
	if want := []pendingFollowUp{{Text: blob, Verbatim: true}}; !reflect.DeepEqual(r.pendingFollowUps, want) {
		t.Fatalf("paste lost its verbatim replay authority: %+v", r.pendingFollowUps)
	}
}

func TestTracePreparationLifecycleStaleCallbacksCannotReachNextOperation(t *testing.T) {
	first, next := newTracePreparationOperation(), newTracePreparationOperation()
	t.Cleanup(first.finish)
	t.Cleanup(next.finish)
	runner := &traceLifecycleRunner{}
	r := &REPL{runner: runner, out: io.Discard}
	callbacks := r.tracePreparationCallbacks(first)
	first.finish()
	r.tracePreparation.Store(next)
	if !callbacks.consumeCommand("/cancel") {
		t.Fatal("typed cancel was not recognized")
	}
	callbacks.onCtrlC()
	callbacks.onEsc()
	if next.ctx.Err() != nil || runner.cancellations.Load() != 0 {
		t.Fatal("stale callback canceled a later operation or pipeline")
	}
	if callbacks.trySteer != nil || callbacks.onSteered != nil {
		t.Fatal("local operation unexpectedly installed pipeline steering callbacks")
	}
	if callbacks.consumeCommand("/cancelled") || callbacks.consumeCommand("ordinary question") {
		t.Fatal("cancellation classifier swallowed ordinary input")
	}
	// A stale input window must also leave an unsubmitted command intact.
	w := &runInputWindow{partial: []byte("/cancel")}
	w.dead.Store(true)
	w.commitLine(r.tracePreparationCallbacks(next))
	if next.ctx.Err() != nil || strings.TrimSpace(string(w.partial)) != "/cancel" {
		t.Fatal("dead input window canceled the next operation or discarded input")
	}
}
