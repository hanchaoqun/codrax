package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// app-100 blocks on a monitor whose payload names owner-4242, a thread with its
// own scheduler row in the capture, and is woken by worker-200.
const hmc378LockTwinCapture = `# tracer: nop
         swapper-0     (      0) [001] .... 4.990000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
             app-100   (    100) [001] .... 5.000000: tracing_mark_write: B|100|monitor contention with owner owner (4242) at void com.example.Store.list()(Store.java:12) waiters=1 blocking from void com.example.Store.read()(Store.java:34)
             app-100   (    100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120
         swapper-0     (      0) [002] .... 5.000200: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=120
          worker-200   (    100) [002] .... 5.112000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001
         swapper-0     (      0) [001] .... 5.112223: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
             app-100   (    100) [001] .... 5.112300: tracing_mark_write: E|100
             app-100   (    100) [001] .... 5.114000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120
         swapper-0     (      0) [003] .... 5.200000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=owner next_pid=4242 next_prio=120
           owner-4242  (    100) [003] .... 5.210000: sched_switch: prev_comm=owner prev_pid=4242 prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120
`

// The evidence record that mirrors a typed lock rank row is worded like that
// row: the resolved holder's row says the holder blocked the waiter, never that
// the holder itself waited. The mirror stays off the supplement rows.
func TestHMC378EvidenceFactTwinSpeaksItsRankRowsBlockingWord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lock.ftrace")
	if err := os.WriteFile(path, []byte(hmc378LockTwinCapture), 0o644); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100, "target_scope": "thread", "time_start": 4.99, "time_end": 5.115})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	var rank, mirror *types.ObservationRecord
	for i := range result.Observations {
		record := &result.Observations[i]
		if record.Object != "blocking_span" || !strings.HasPrefix(record.Predicate, "root_cause_") {
			continue
		}
		switch record.ClaimKey {
		case record.Predicate:
			rank = record
		case "evidence_fact:" + record.Predicate:
			mirror = record
		}
	}
	if rank == nil || mirror == nil || rank.Subject != mirror.Subject || rank.Subject != "owner-4242" {
		t.Fatalf("fixture: want the resolved holder's lock rank record and its mirror: rank=%+v mirror=%+v", rank, mirror)
	}
	for _, c := range []struct {
		zh         bool
		want, deny string
	}{
		{true, "owner-4242 -> 持锁阻塞", "阻塞等待"},
		{false, "owner-4242 -> lock-holder blocking", "blocking wait"},
	} {
		rankText, mirrorText := traceQueryObservationSupplementText(*rank, c.zh), traceQueryObservationSupplementText(*mirror, c.zh)
		if !strings.Contains(rankText, c.want) {
			t.Fatalf("fixture: the rank row does not speak the holder word: %s", rankText)
		}
		if !strings.Contains(mirrorText, c.want) || strings.Contains(mirrorText, c.deny) {
			t.Fatalf("the mirror of the holder's row must say %q, got: %s", c.want, mirrorText)
		}
	}
	if order := traceQueryObservationSupplementOrder(*mirror); order != 0 {
		t.Fatalf("the mirror must stay off the supplement rows, got order %d", order)
	}
}
