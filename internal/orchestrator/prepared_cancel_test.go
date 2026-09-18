package orchestrator

import (
	"errors"
	"testing"
)

func TestPreparedRunCancellationConsumesTurnMetadata(t *testing.T) {
	o := &Orchestrator{presentationDirective: "canceled diagram request", presentationDiagramRequired: true, phaseContextPrefix: "old phase", nextPhaseHint: "old hint"}
	cancel, release, err := o.PrepareRunCancellation()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	cancel("early cancel")
	if _, err := o.Run("canceled request", t.TempDir(), "main"); !errors.Is(err, ErrCanceled) {
		t.Fatalf("expected cancel: %v", err)
	}
	if o.presentationDirective != "" || o.presentationDiagramRequired || o.phaseContextPrefix != "" || o.nextPhaseHint != "" {
		t.Fatal("early cancellation leaked canceled-turn metadata to the next request")
	}
}

func TestPreparedRunCancellationBeforePublicRun(t *testing.T) {
	// A zero-value orchestrator has no agents or tools: the cancellation must
	// exit through Run itself before any model/source/write work starts.
	o := &Orchestrator{}
	cancel, release, err := o.PrepareRunCancellation()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	cancel("early script cancel")
	bus, err := o.Run("never dispatched", t.TempDir(), "main")
	var canceled *CanceledError
	if bus != nil || !errors.As(err, &canceled) || canceled.Reason != "early script cancel" {
		t.Fatalf("early cancel lost: bus=%v err=%v", bus, err)
	}
	if o.cancelTokenLoad() != nil {
		t.Fatal("finished Run retained a token")
	}
}

func TestPreparedRunCancellationIsolationAndAbandonment(t *testing.T) {
	o := &Orchestrator{}
	cancelA, releaseA, err := o.PrepareRunCancellation()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.PrepareRunCancellation(); err == nil {
		t.Fatal("double reservation accepted")
	}
	releaseA() // No Run took A.
	cancelB, releaseB, err := o.PrepareRunCancellation()
	if err != nil {
		t.Fatal(err)
	}
	defer releaseB()
	tokenB := o.beginRunCancellation()
	defer o.endRunCancellation(tokenB)
	releaseA() // Must not clear or cancel B, including repeated stale releases.
	cancelA("late A")
	if tokenB.IsCanceled() || tokenB.Context().Err() != nil {
		t.Fatal("A affected B")
	}
	if _, _, err := o.PrepareRunCancellation(); err == nil {
		t.Fatal("reservation during active Run accepted")
	}
	cancelB("B only")
	if !tokenB.IsCanceled() || tokenB.Reason() != "B only" {
		t.Fatal("B lost bound cancellation")
	}
}

func TestPreparedRunCancellationDoesNotChangeUnreservedRun(t *testing.T) {
	o := &Orchestrator{}
	a := o.beginRunCancellation()
	o.Cancel("ordinary cancel")
	if !a.IsCanceled() {
		t.Fatal("legacy active cancellation lost")
	}
	o.endRunCancellation(a)
	b := o.beginRunCancellation()
	defer o.endRunCancellation(b)
	if b == a || b.IsCanceled() || b.Context().Err() != nil {
		t.Fatal("new Run inherited cancellation")
	}
	o.endRunCancellation(a)
	if o.cancelTokenLoad() != b {
		t.Fatal("stale cleanup cleared active token")
	}
}
