package repl

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc17RecoveryREPL(t *testing.T, input io.Reader, output io.Writer) (*REPL, *materialAwareRunner) {
	t.Helper()
	old := replPreparedMaterial(t)
	runner := &materialAwareRunner{material: old}
	runner.curTrace, runner.curTraceSource = old.Preview(), "android_atrace"
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatal(err)
	}
	r := New(Config{Runner: runner, Store: store, In: input, Out: output, Language: "en",
		Render: renderNothing, RepoRoot: t.TempDir(), RuntimeAnchor: filepath.Join(t.TempDir(), "runtime")})
	return r, runner
}

func TestHMC17TraceFailureRequiresExplicitRecoveryThroughLoop(t *testing.T) {
	for _, recovery := range []string{"none", "keep", "clear", "retry"} {
		t.Run(recovery, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "new trace.sys")
			if err := os.WriteFile(path, []byte("# new trace content\n"), 0600); err != nil {
				t.Fatal(err)
			}
			input := "/htrace /missing/new.sys\ninspect the new capture\n"
			switch recovery {
			case "keep", "clear":
				input += "/htrace " + recovery + "\ninspect once more\n"
			case "retry":
				input += "/htrace " + path + "\ninspect once more\n"
			}
			input += "/exit\n"
			var out bytes.Buffer
			r, runner := hmc17RecoveryREPL(t, strings.NewReader(input), &out)
			old := r.attachedTraceMaterial
			if err := r.Loop(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Following input is held") {
				t.Fatal(out.String())
			}
			if recovery == "none" {
				if len(runner.seenMaterials) != 0 || len(r.heldTraceInputs) != 1 || !r.traceAttachmentFailed || r.attachedTraceMaterial != old {
					t.Fatalf("failed attachment silently used old material: runs=%d held=%d failed=%v", len(runner.seenMaterials), len(r.heldTraceInputs), r.traceAttachmentFailed)
				}
				return
			}
			if len(runner.seenMaterials) != 2 || r.traceAttachmentFailed || len(r.heldTraceInputs) != 0 {
				t.Fatalf("recovery lost/duplicated questions: runs=%d out=%s", len(runner.seenMaterials), out.String())
			}
			for i, material := range runner.seenMaterials {
				switch recovery {
				case "keep":
					if material != old {
						t.Fatal("confirmation did not keep old receipt")
					}
				case "clear":
					if material != nil || runner.seenTraces[i] != "" {
						t.Fatal("clear retained receipt")
					}
				case "retry":
					if material == nil || material == old || material.SourcePath() != path {
						t.Fatal("retry did not replace receipt")
					}
					if err := material.Validate(context.Background(), runner.seenTraces[i]); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestHMC17TraceFailureHoldsUnreadPipeQuestion(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	out := &hmc17ScriptOutput{changed: make(chan struct{}, 1)}
	r, runner := hmc17RecoveryREPL(t, reader, out)
	done := make(chan error, 1)
	go func() { done <- r.Loop() }()
	if _, err := io.WriteString(writer, "/htrace /missing/not-prefetched.sys\n"); err != nil {
		t.Fatal(err)
	}
	waitOutput := func(needle string) {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		for !strings.Contains(out.String(), needle) {
			select {
			case <-out.changed:
			case <-deadline.C:
				t.Fatalf("missing %q: %s", needle, out.String())
			}
		}
	}
	waitOutput("Following input is held")
	// This question arrives only AFTER failure, never on the preparation lease.
	if _, err := io.WriteString(writer, "inspect the late capture question\n/exit\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not return")
	}
	if len(runner.seenMaterials) != 0 || len(r.heldTraceInputs) != 1 {
		t.Fatalf("unread question bypassed recovery: runs=%d held=%d", len(runner.seenMaterials), len(r.heldTraceInputs))
	}
}

func TestHMC17TraceFailureDirectRunCannotBypassRecovery(t *testing.T) {
	var out bytes.Buffer
	r, _ := hmc17RecoveryREPL(t, strings.NewReader(""), &out)
	r.handleHitraceCmd("/htrace /missing/new.sys")
	called := false
	_, err := r.runInFlightWrap(func() (*types.BusContext, error) { called = true; return nil, nil })
	if err == nil || called {
		t.Fatal("direct slash Run bypassed failed-attachment state")
	}
	r.dispatch("question from direct API", "question from direct API")
	if len(r.heldTraceInputs) != 1 {
		t.Fatal("direct dispatch bypassed recovery")
	}
}

func TestHMC17TraceFailureCommandLikePasteRemainsData(t *testing.T) {
	var out bytes.Buffer
	r, runner := hmc17RecoveryREPL(t, strings.NewReader("/htrace keep\n/exit\n"), &out)
	r.traceAttachmentFailed = true
	r.pendingFollowUps = []pendingFollowUp{{Text: "/htrace clear", Verbatim: true}}
	old := r.attachedTraceMaterial
	if err := r.Loop(); err != nil {
		t.Fatal(err)
	}
	if len(runner.seenMaterials) != 1 || runner.seenMaterials[0] != old || r.attachedTraceMaterial != old {
		t.Fatal("held pasted command executed instead of remaining data")
	}
}

func TestHMC17TracePreparationPreflightDoesNotConvertOrPublish(t *testing.T) {
	var out bytes.Buffer
	runner := &logAwareRunner{curTrace: "old", curTraceSource: "android_atrace"}
	anchor := filepath.Join(t.TempDir(), "not-created")
	r := New(Config{Runner: runner, In: strings.NewReader(""), Out: &out, Language: "en", RuntimeAnchor: anchor})
	path := filepath.Join(t.TempDir(), "binary.sys")
	if err := os.WriteFile(path, hmc17BinaryTraceTailFixture(), 0600); err != nil {
		t.Fatal(err)
	}
	r.handleHitraceCmd("/htrace " + path)
	if !r.traceAttachmentFailed || runner.curTrace != "old" || !strings.Contains(out.String(), "cannot retain complete") {
		t.Fatal(out.String())
	}
	if paths, err := filepath.Glob(filepath.Join(anchor, "trace-input-*")); err != nil || len(paths) != 0 {
		t.Fatalf("preflight started conversion: %v %v", paths, err)
	}
}

func TestHMC17TracePreparationIsNotAModelUsageBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.sys")
	if err := os.WriteFile(path, []byte("# source text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	r, runner := hmc17RecoveryREPL(t, strings.NewReader(""), &out)
	r.renderer = render.New(io.Discard, false)
	r.usageBoundaryPending = true
	previousTurnCanceled := false
	r.turnCancel = func() { previousTurnCanceled = true }
	r.handleHitraceCmd("/htrace " + path)
	if r.attachedTraceMaterial == nil || r.attachedTraceMaterial.SourcePath() != path {
		t.Fatal("preparation failed")
	}
	if !r.usageBoundaryPending || previousTurnCanceled || r.runInFlight.Load() || len(runner.seenMaterials) != 0 {
		t.Fatal("local preparation entered a model turn or reset its usage boundary")
	}
}

type hmc17LocalOnlyRunner struct {
	*materialAwareRunner
	cancels atomic.Int32
	steers  atomic.Int32
}

func (r *hmc17LocalOnlyRunner) Cancel(string)                         { r.cancels.Add(1) }
func (r *hmc17LocalOnlyRunner) IsCanceled() bool                      { return false }
func (r *hmc17LocalOnlyRunner) PushSteeringNote(string) bool          { r.steers.Add(1); return true }
func (r *hmc17LocalOnlyRunner) TakeUnconsumedSteeringNotes() []string { r.steers.Add(1); return nil }

func TestHMC17TracePreparationScriptCancelDoesNotCancelRunnerOrAnswerOldTrace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new capture.sys")
	original := hmc17BinaryTraceTailFixture()
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(t.TempDir(), "missing"))
	var out bytes.Buffer
	r, materialRunner := hmc17RecoveryREPL(t, strings.NewReader("/htrace "+path+"\nqueued before cancel\n/cancel\nquestion after cancel\n/exit\n"), &out)
	runner := &hmc17LocalOnlyRunner{materialAwareRunner: materialRunner}
	r.runner = runner
	old := r.attachedTraceMaterial
	if err := r.Loop(); err != nil {
		t.Fatal(err)
	}
	if !r.traceAttachmentFailed || len(r.heldTraceInputs) != 2 || len(runner.seenMaterials) != 0 || r.attachedTraceMaterial != old || runner.material != old {
		t.Fatalf("canceled preparation reused old trace or swallowed input: failed=%v held=%d runs=%d output=%s", r.traceAttachmentFailed, len(r.heldTraceInputs), len(runner.seenMaterials), out.String())
	}
	if runner.cancels.Load() != 0 || runner.steers.Load() != 0 {
		t.Fatal("local preparation called prior pipeline cancellation/steering")
	}
	if r.tracePreparation.Load() != nil || r.runInFlight.Load() {
		t.Fatal("local operation retained active state")
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original changed: %v", err)
	}
	outputs, err := filepath.Glob(filepath.Join(r.runtimeAnchor, "trace-input-*"))
	if err != nil || len(outputs) != 0 {
		t.Fatalf("unpublished preparation leaked outputs: %v %v", outputs, err)
	}
}
