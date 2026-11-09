package tool

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Whole original, not an excerpt. The constants were read independently
// from sched_switch / sched_wakeup in the raw capture. A wake boundary and
// the next execution must keep their own original lines, even when only
// 13 microseconds apart. Neither line tells why the thread went to sleep.
func TestHMC365HoneybirdWholeNativeWaitSequence(t *testing.T) {
	path := os.Getenv("CODRAX_HMC365_HONEYBIRD")
	if path == "" {
		t.Skip("requires the pinned complete Honeybird ftrace")
	}
	const digest = "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e"
	if h, n := hmc176bRepresentativeDigest(t, path); h != digest || n != 106896120 {
		t.Fatal("not the pinned whole Honeybird capture")
	}
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	const start, end, tid = 11343.248, 11343.310, 64156
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "thread_timeline", "pid": tid, "time_start": start, "time_end": end})
	var value tracequery.Result
	for _, row := range r.Observations {
		if row.Predicate != TraceThreadTimelinePredicate {
			continue
		}
		data, err := os.ReadFile(row.SourceRef.PayloadRef)
		if err != nil || json.Unmarshal(data, &value) != nil {
			t.Fatalf("cannot read the current native payload: %v", err)
		}
		break
	}
	timeline := value.Timeline
	if timeline == nil || timeline.Thread.PID != tid || timeline.Window.StartTs != start || timeline.Window.EndTs != end || timeline.IntegrityFailure != "" {
		t.Fatalf("no current exact target timeline: %+v", timeline)
	}
	type expected struct {
		state              tracequery.ThreadState
		start, end, millis float64
		startLine          int
	}
	want := []expected{
		{tracequery.StateSSleep, 11343.250704, 11343.297396, 46.692, 20312},
		{tracequery.StateRunnable, 11343.297396, 11343.297409, .013, 25227},
	}
	near := func(a, b float64) bool { return math.Abs(a-b) <= 1e-7 }
	sleep, ready, running := false, false, false
	for _, row := range timeline.Intervals {
		for i, e := range want {
			if row.State == e.state && near(row.StartTs, e.start) && near(row.EndTs, e.end) && near(row.DurationMs, e.millis) && row.StartLine == e.startLine {
				if i == 0 {
					// The closing line belongs to the sleep interval;
					// the wake-start witness belongs to the ready interval.
					sleep = row.EndLine == 25227 && !row.CPUKnown
				} else {
					ready = row.WakeupLine == 25227 && row.EndLine == 25229 && !row.CPUKnown
				}
			}
		}
		running = running || (row.State == tracequery.StateRunning && near(row.StartTs, 11343.297409) && row.StartLine == 25229 && row.CPUKnown && row.CPU == 4)
	}
	if !sleep || !ready || !running {
		t.Fatalf("native states disagree with the raw boundaries (sleep=%t ready=%t running=%t): %+v", sleep, ready, running, timeline.Intervals)
	}
	table := nativeTimelinePublicTable(t, r, TraceThreadTimelinePredicate)
	if len(table.Rows) != len(timeline.Intervals) {
		t.Fatal("native public timeline changed its population in presentation")
	}
	if h, _ := hmc176bRepresentativeDigest(t, path); h != digest {
		t.Fatal("whole capture changed")
	}
}
