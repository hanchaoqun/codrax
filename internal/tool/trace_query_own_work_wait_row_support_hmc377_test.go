package tool

import (
	"fmt"
	"strings"
	"testing"
)

// HMC377 E1E6-R1: a split own-work family publishes two rows of the same
// spans — the own-work row (time the thread ran its own work) and the wait
// row (instants it waited for its chain inside those spans). The direction
// conservation audit sums each seat's support union; the wait row's support
// must be its priced share (member ∩ the waits), never the whole member
// extent, or the same instants are billed twice. Synthetic public entry:
// main-100 verifies two classes of 16 ms and sleeps 4 ms inside each, woken
// by waker-150; window 40 ms, member union 32 ms (own 24 + wait 8).
func hmc377SplitOwnWorkCapture(t *testing.T) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.000000, "main", 100, 100, 1)
	c.run(10.000050, "waker", 150, 100, 5)
	for _, base := range []float64{10.002, 10.020} {
		c.span(base, base+0.016, "main", 100, 100, 1, fmt.Sprintf("VerifyClass com.example.C%.0f", base*1000))
		c.sleep(base+0.006, "main", 100, 100, 1)
		c.add(base+0.010, "waker", 150, 100, 5, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
		c.run(base+0.010100, "main", 100, 100, 1)
	}
	c.sleep(10.039000, "main", 100, 100, 1)
	c.sleep(10.039500, "waker", 150, 100, 5)
	return c.write(t)
}

func TestHMC377SplitOwnWorkFamilyIsNotDoubleBilledByDirection(t *testing.T) {
	_, native := hmc376QueryTarget(t, hmc377SplitOwnWorkCapture(t), "root_cause_rank", map[string]any{"pid": 100, "target_scope": "thread"}, 10.0, 10.04)
	rank := native.RootCauseRank
	if rank == nil {
		t.Fatal("no rank board")
	}
	var own, wait bool
	for _, row := range rank.Items {
		if row.Thread.PID != 100 || row.Type != "class_verification" {
			continue
		}
		switch {
		case hmc376Near(row.EffectiveImpactMs, 24.0):
			own = true
		case hmc376Near(row.EffectiveImpactMs, 8.0):
			wait = true
		}
		if row.DirectionConservationExcess != nil {
			t.Fatalf("the split family's two rows bill the same instants twice: %s excess=%+v", hmc376RowLabel(row), row.DirectionConservationExcess)
		}
	}
	if !own || !wait {
		t.Fatalf("fixture: the split own-work (24) and wait (8) rows must both publish: %+v", rank.Items)
	}
	for _, caveat := range rank.Caveats {
		if strings.Contains(caveat, "direction_conservation") && strings.Contains(caveat, "main-100") {
			t.Fatalf("false double-billing disclosure: %s", caveat)
		}
	}
}
