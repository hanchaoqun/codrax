package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TTY-3 pins (design ledger docs/design/tty_single_stdin_owner_20260729.md
// §3): mid-run steering notes — accepted only while a run is live,
// consumed at explore boundaries (@pins → forced reads, text → window
// hint), unconsumed notes handed back exactly once.

func TestSteeringIntake_Lifecycle(t *testing.T) {
	o := &Orchestrator{}
	if o.PushSteeringNote("before open") {
		t.Fatal("closed intake must reject")
	}
	o.steering.openIntake()
	if !o.PushSteeringNote("note one") || !o.PushSteeringNote("  note two  ") {
		t.Fatal("open intake must accept")
	}
	if o.PushSteeringNote("   ") {
		t.Fatal("blank notes never accepted")
	}
	o.steering.closeIntake()
	if o.PushSteeringNote("after close") {
		t.Fatal("closed intake must reject")
	}
	rem := o.TakeUnconsumedSteeringNotes()
	if len(rem) != 2 || rem[0] != "note one" || rem[1] != "note two" {
		t.Fatalf("unconsumed = %v", rem)
	}
	if len(o.TakeUnconsumedSteeringNotes()) != 0 {
		t.Fatal("take must clear (exactly-once handback)")
	}
}

func TestApplySteeringNotes_PinsAndHint(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "dock.go"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:  types.NewMutableState(""),
		RepoRoot: root,
	}}
	o.steering.openIntake()
	o.PushSteeringNote(`也看看 @dock.go 的重绘计数, 别漏 "同步输出"`)

	if consumed := o.applySteeringNotesToExploreWindow(); consumed != 1 {
		t.Fatalf("consumed = %d, want 1", consumed)
	}
	// The pin joined the forced-read queue with the steering origin.
	var found bool
	for _, p := range o.busCtx.Mutable.EvidenceClosure().PendingReads() {
		if p.File == "dock.go" && p.Origin == "explore.user_steering_pin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("steering pin missing from pending reads: %+v", o.busCtx.Mutable.EvidenceClosure().PendingReads())
	}
	// The note rides the window hint verbatim, quoted as data.
	hint := o.busCtx.TaskState.RetryHint
	if !strings.Contains(hint, "Operator steering note") || !strings.Contains(hint, "dock.go 的重绘计数") {
		t.Fatalf("hint missing verbatim note: %q", hint)
	}
	// Consumption cleared the intake: nothing left to hand back.
	if len(o.TakeUnconsumedSteeringNotes()) != 0 {
		t.Fatal("consumed notes must not also hand back")
	}
	// Second call with nothing pending appends nothing.
	before := o.busCtx.TaskState.RetryHint
	if o.applySteeringNotesToExploreWindow() != 0 || o.busCtx.TaskState.RetryHint != before {
		t.Fatal("empty drain must be a no-op")
	}
}
