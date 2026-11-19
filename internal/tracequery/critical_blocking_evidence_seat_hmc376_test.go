package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The evidence pack reads the head of the blocking list, so vocabulary-only rows
// sit behind the other rows even when the list fits its own limit.
func TestHMC376EvidencePackKeepsRowsPublishedWithoutTheVocabulary(t *testing.T) {
	type span struct {
		comm       string
		tid        int
		name       string
		durationUs int
	}
	var spans []span
	for i := 0; i < 8; i++ {
		spans = append(spans, span{fmt.Sprintf("gen%d", i), 220 + i, fmt.Sprintf("worker lock wait %d", i), 300000 - i*1000})
	}
	for i := 0; i < 3; i++ {
		spans = append(spans, span{fmt.Sprintf("gcw%d", i), 210 + i, "GC: Wait For Completion Alloc", 200000 - i*1000})
	}
	for i := 0; i < 9; i++ {
		spans = append(spans, span{fmt.Sprintf("ver%d", i), 240 + i, fmt.Sprintf("VerifyClass com.example.Lock%d", i), 20000 - i*100})
	}
	type event struct {
		us, order int
		text      string
	}
	var events []event
	for i, s := range spans {
		const startUs = 1000
		endUs := startUs + s.durationUs
		events = append(events,
			event{startUs, i*2 + 1, fmt.Sprintf("%16s-%d   (    100) [001] ....  10.%06d: tracing_mark_write: B|100|%s", s.comm, s.tid, startUs, s.name)},
			event{endUs, i*2 + 2, fmt.Sprintf("%16s-%d   (    100) [001] ....  10.%06d: tracing_mark_write: E|100", s.comm, s.tid, endUs)})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].us != events[j].us {
			return events[i].us < events[j].us
		}
		return events[i].order < events[j].order
	})
	var trace strings.Builder
	trace.WriteString("# tracer: nop\n")
	trace.WriteString("            main-100   (    100) [000] ....  10.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=main next_pid=100 next_prio=120\n")
	for _, e := range events {
		trace.WriteString(e.text + "\n")
	}
	trace.WriteString("            main-100   (    100) [000] ....  12.000000: sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120\n")
	path := filepath.Join(t.TempDir(), "synthetic.ftrace")
	if err := os.WriteFile(path, []byte(trace.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(idx, Query{View: "critical_blocking_calls", TimeStart: 10, TimeEnd: 12})
	var facts strings.Builder
	for _, fact := range res.EvidencePack {
		facts.WriteString(fact.Subject + " " + fact.Summary + "\n")
	}
	text := facts.String()
	if strings.Contains(text, "gcw") {
		t.Fatalf("a vocabulary-only row took an evidence seat:\n%s", text)
	}
	for _, want := range []string{"ver5", "ver6", "ver7"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%s lost its evidence seat:\n%s", want, text)
		}
	}
}
