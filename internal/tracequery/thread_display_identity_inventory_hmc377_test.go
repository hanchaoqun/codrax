package tracequery

import (
	"fmt"
	"testing"
)

// HMC377 F1 — the publication seam. The renamed pool worker 90 of the identity
// fixture records thirty class-verification spans while it still carries the
// name "worker"; for most of the window it is "task-b". Thirty spans exceed
// the sixteen semantic seats of the TraceSpans listing, so every semantic-span family is folded from
// the complete unexported inventory behind the listing. A family folded after
// publication must name its thread by the published display name, like every
// other face of the result.
func hmc377InventoryTraceLines() []string {
	lines := hmc377IdentityTraceLines()
	var spans []string
	for i := 0; i < 30; i++ {
		begin := 1.0310 + float64(i)*0.00028
		spans = append(spans,
			fmt.Sprintf("       worker-90 (56) [001] .... %.6f: tracing_mark_write: B|56|VerifyClass com.example.K%d", begin, i),
			fmt.Sprintf("       worker-90 (56) [001] .... %.6f: tracing_mark_write: E|56", begin+0.0002))
	}
	// After the 1.030 switch to worker (line 8) and before its 1.040 switch out.
	out := append([]string{}, lines[:8]...)
	out = append(out, spans...)
	return append(out, lines[8:]...)
}

func TestHMC377PublicationNamesTheFullSpanInventory(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "inventory.ftrace", hmc377InventoryTraceLines()...)
	idx, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"window_stats", "root_cause_rank"} {
		res := Run(idx, Query{View: view, PID: 56, TimeStart: 1.0, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true})
		stats := res.WindowStats
		if stats == nil || len(stats.traceSpanFullInventory) <= len(stats.TraceSpans) {
			t.Fatalf("%s: the fixture must fold from the inventory behind the listing: %+v", view, stats)
		}
		// The published faces name 90 by its display name ...
		if names := hmc377NamesByTID(t, res, 90)[90]; len(names) != 1 || !names["task-b"] {
			t.Fatalf("%s: published faces name TID 90 %v", view, names)
		}
		// ... and so does every face folded from the result after publication:
		// the semantic-span families of the complete inventory (the
		// trace_query tool's semantic span and span-member records read
		// these) and the native target timeline.
		found := false
		for _, fam := range SemanticSpanFamiliesForStats(res.WakeupChain, res.WindowStats) {
			if fam.Thread.PID != 90 {
				continue
			}
			found = true
			if fam.Thread.Comm != "task-b" {
				t.Fatalf("%s: family thread %q, want the display name task-b", view, fam.Thread.Comm)
			}
			for _, member := range fam.Members {
				if member.Thread != fam.Thread {
					t.Fatalf("%s: member thread %+v differs from its family's %+v", view, member.Thread, fam.Thread)
				}
			}
			if len(fam.Members) != 30 {
				t.Fatalf("%s: the family folds the complete inventory: %d members", view, len(fam.Members))
			}
		}
		if !found {
			t.Fatalf("%s: no class-verification family for TID 90", view)
		}
		if timeline := res.NativeTargetTimeline(); timeline != nil && timeline.Thread.PID == 56 && timeline.Thread.Comm != "app.main" {
			t.Fatalf("%s: native target timeline names TID 56 %q", view, timeline.Thread.Comm)
		}
	}
}
