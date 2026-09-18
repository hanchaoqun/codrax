package repl

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestScriptInputOwnerStopWaitsForAdmittedCancelAndCannotStopNextLease(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	o := newScriptInputOwner(reader, 1024)
	defer o.close()
	entered, release := make(chan struct{}), make(chan struct{})
	a, err := o.beginRuntime(func(string) { close(entered); <-release }, nil)
	if err != nil {
		t.Fatal(err)
	}
	go io.WriteString(writer, "/cancel\n")
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("cancel not admitted")
	}
	if o.mu.TryLock() {
		o.mu.Unlock()
		t.Fatal("callback admission not protected by stop barrier")
	}
	stopped := make(chan scriptInputReceipt, 1)
	go func() { stopped <- a.stopAndDrain() }()
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop blocked after callback completion")
	}
	scanner, finish := o.borrowLines()
	defer finish()
	a.stopAndDrain()
	go io.WriteString(writer, "next operation\n")
	if !scanner.Scan() || scanner.Text() != "next operation" {
		t.Fatalf("old stop revoked new lease: %v", scanner.Err())
	}
}

func TestScriptInputOwnerAtomicQueueHandoff(t *testing.T) {
	for i := 0; i < 100; i++ {
		o := newScriptInputOwner(strings.NewReader("exactly once\n"), 1024)
		a, err := o.beginRuntime(func(string) { t.Error("unexpected cancel") }, nil)
		if err != nil {
			t.Fatal(err)
		}
		receipt := a.stopAndDrain()
		scanner, release := o.borrowLines()
		got := receipt.lines
		for scanner.Scan() {
			got = append(got, scanner.Text())
		}
		if scanner.Err() != nil {
			t.Fatal(scanner.Err())
		}
		release()
		o.close()
		if len(got) != 1 || got[0] != "exactly once" {
			t.Fatalf("handoff lost/duplicated input: %q", got)
		}
		if len(a.stopAndDrain().lines) != 0 {
			t.Fatal("repeated stop replayed queue")
		}
	}
}

func TestScriptInputOwnerByteBoundStillAdmitsCancel(t *testing.T) {
	o := newScriptInputOwner(strings.NewReader("aa\nbb\ncc\n/cancel\n"), 1024)
	defer o.close()
	o.byteCap = 4 // Small budget seam; line-limit behavior is covered through Loop.
	canceled := make(chan struct{})
	l, err := o.beginRuntime(func(string) { close(canceled) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("byte cap hid cancel")
	}
	receipt := l.stopAndDrain()
	if len(receipt.lines) != 2 || receipt.dropped != 1 || receipt.byteCap != 4 {
		t.Fatalf("wrong receipt: %+v", receipt)
	}
}

type scriptScopedRealRunner struct {
	*orchestrator.Orchestrator
	canceled chan struct{}
}

func (r *scriptScopedRealRunner) PrepareRunCancellation() (func(string), func(), error) {
	cancel, release, err := r.Orchestrator.PrepareRunCancellation()
	return func(reason string) { cancel(reason); close(r.canceled) }, release, err
}

func TestScriptInputOwnerRealRunCancelBeforeRunEntry(t *testing.T) {
	runner := &scriptScopedRealRunner{Orchestrator: &orchestrator.Orchestrator{}, canceled: make(chan struct{})}
	r := &REPL{runner: runner, in: strings.NewReader("/cancel\n"), out: io.Discard, attachedLogMaxBytes: 1024}
	r.cancelSigOnce.Do(func() {})
	defer func() { r.scriptInput.close() }()
	_, err := r.runInFlightWrap(func() (*types.BusContext, error) {
		select {
		case <-runner.canceled:
		case <-time.After(time.Second):
			t.Fatal("cancel not delivered before Run")
		}
		// Real public Run, no fake first-operation cancellation latch.
		return runner.Run("must not call model", t.TempDir(), "main")
	})
	if !errors.Is(err, orchestrator.ErrCanceled) {
		t.Fatalf("real Run lost prefetched cancel: %v", err)
	}
}
