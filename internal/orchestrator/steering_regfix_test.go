package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// REGFIX-B pins (regression sweep 2026-07-30, findings #9/#10/#11/#15;
// postmortem docs/design/tty_single_stdin_owner_20260729.md §6).

// TestSteering_ReturnsHintTextForParallelLane pins finding #9 (high):
// the drain must hand the composed hint text back to the caller so the
// PARALLEL lane can merge it into every worker hint — writing only into
// the parent bus destroyed the operator's text (workers overwrite their
// clone's hint with the pre-drain value).
func TestSteering_ReturnsHintTextForParallelLane(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState(""), RepoRoot: t.TempDir()}}
	o.steering.openIntake()
	o.PushSteeringNote("也看看 vsync 抖动")

	consumed, hint := o.takeSteeringNotesForExploreWindow()
	if consumed != 1 {
		t.Fatalf("consumed = %d, want 1", consumed)
	}
	if !strings.Contains(hint, "vsync 抖动") {
		t.Fatalf("returned hint must carry the note verbatim; got %q", hint)
	}
	// The parent bus still gets it (serial lane), AND the caller has the
	// text for the parallel workers.
	if !strings.Contains(o.busCtx.TaskState.RetryHint, "vsync 抖动") {
		t.Fatal("serial lane must still receive the hint on the parent bus")
	}

	// Merge helper keeps the pre-composed body intact.
	merged := appendSteeringHintToWindowHint("worker window body", hint)
	if !strings.HasPrefix(merged, "worker window body") || !strings.Contains(merged, "vsync 抖动") {
		t.Fatalf("merge lost content: %q", merged)
	}
	if got := appendSteeringHintToWindowHint("body", ""); got != "body" {
		t.Fatalf("empty steering must pass through: %q", got)
	}
}

// TestSteering_RefusesCommandLines pins findings #10/#15: a line the
// operator typed as a COMMAND must be refused by the intake so the
// window queues it for real dispatch instead of pasting "/cancel" into
// the explorer prompt.
func TestSteering_RefusesCommandLines(t *testing.T) {
	o := &Orchestrator{}
	o.steering.openIntake()
	for _, cmd := range []string{"/cancel", "/approve plan-1", "\\history", "!ls -la"} {
		if o.PushSteeringNote(cmd) {
			t.Errorf("command line %q must be refused by the steering intake", cmd)
		}
	}
	if !o.PushSteeringNote("这是普通补充说明") {
		t.Fatal("prose must still be accepted")
	}
	if len(o.TakeUnconsumedSteeringNotes()) != 1 {
		t.Fatal("only the prose note may be held")
	}
}

// TestSteering_PinsHonourSourceExclusionBoundary pins finding #11: an
// operator who declared "don't read source" keeps that boundary even
// when a steering note carries an @token; the note text still reaches
// the prompt.
func TestSteering_PinsHonourSourceExclusionBoundary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "dock.go"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	mkOrch := func(exclude bool) *Orchestrator {
		ir := &types.AnalysisIR{}
		if exclude {
			ir.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"只看日志，不要读代码"},
			}
		}
		return &Orchestrator{busCtx: &types.BusContext{
			Mutable:    types.NewMutableState(""),
			RepoRoot:   root,
			AnalysisIR: ir,
		}}
	}

	excluded := mkOrch(true)
	excluded.steering.openIntake()
	excluded.PushSteeringNote("顺带看看 @dock.go")
	if _, hint := excluded.takeSteeringNotesForExploreWindow(); !strings.Contains(hint, "dock.go") {
		t.Fatal("note text must still reach the prompt under the exclusion boundary")
	}
	if len(excluded.busCtx.Mutable.EvidenceClosure().PendingReads()) != 0 {
		t.Fatal("source-exclusion boundary must withhold the forced read")
	}

	allowed := mkOrch(false)
	allowed.steering.openIntake()
	allowed.PushSteeringNote("顺带看看 @dock.go")
	allowed.takeSteeringNotesForExploreWindow()
	var found bool
	for _, p := range allowed.busCtx.Mutable.EvidenceClosure().PendingReads() {
		if p.File == "dock.go" && p.Origin == "explore.user_steering_pin" {
			found = true
		}
	}
	if !found {
		t.Fatal("without an exclusion boundary the pin must still enqueue")
	}
}
