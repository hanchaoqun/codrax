package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC233NativeIORequestDisplaySafetyNet(t *testing.T) {
	path := hmc081WriteTrace(t, "issuer-41 (41) [001] .... 5.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [issuer]\n"+
		"issuer-41 (41) [001] .... 5.000000: block_rq_issue: 8,0 R 4096 () 456 + 8 [issuer]\n"+
		"issuer-41 (41) [001] .... 5.000000: block_bio_queue: 8,0 R 789 + 8 [issuer]\n"+
		"irq-2 (2) [001] .... 5.010000: block_rq_complete: 8,0 R () 123 + 8 [0]\n"+
		"irq-2 (2) [001] .... 5.010000: block_rq_complete: 8,0 R () 456 + 8 [0]\n"+
		"irq-2 (2) [001] .... 5.010000: block_bio_complete: 8,0 R 789 + 8 [0]\n")
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("storage request observations")}
	var nodes []types.TraceCausalProjectionNode
	ids := map[string]bool{}
	for _, limit := range []int{2, 3} {
		result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "critical_blocking_calls", "time_start": 4.999, "time_end": 5.011, "limit": limit})
		published := 0
		for _, record := range result.Observations {
			if record.Predicate != "critical_blocking" || !strings.Contains(strings.Join(record.RichNotes, "\n"), "type=io_latency") {
				continue
			}
			if key, handled := types.TraceObservationIOPublicationKey(record); !handled || key == "" {
				t.Fatalf("native request has no authenticated identity: %s endpoints=%+v", record.ID, record.IORequestEndpoints)
			}
			ids[record.ID] = true
			// Compile independently so the renderer's safety net is tested even
			// if the complete-ledger projection has already removed duplicates.
			p := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{record})
			if len(p.BackgroundCauses) != 1 {
				t.Fatalf("one native request expected, got %+v", p.BackgroundCauses)
			}
			nodes = append(nodes, p.BackgroundCauses[0])
			published++
		}
		if published != limit {
			t.Fatalf("native query must return its requested bounded population: got %d want %d", published, limit)
		}
	}
	if len(ids) != 5 {
		t.Fatalf("different query publications must have five evidence IDs so exact-ID dedup cannot mask identity handling: got %d", len(ids))
	}
	for _, reverse := range []bool{false, true} {
		ordered := append([]types.TraceCausalProjectionNode(nil), nodes...)
		if reverse {
			for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
		got := runtimeTraceProjAdjacentNodesForDisplay(ordered)
		if len(got) != 3 {
			t.Errorf("renderer must retain three requests and coalesce only each request's repeated publication: reverse=%t got=%d", reverse, len(got))
		}
		calibers := map[string]int{}
		for _, node := range got {
			calibers[node.IOValueCaliber]++
			if node.ImpactMS != 10 || node.CumulativeImpactMS != 10 || node.DuplicatePublications > 2 || node.ResourceCompletionClosure {
				t.Errorf("renderer changed request quantity or authority: reverse=%t id=%s impact=%g duplicates=%d closure=%t", reverse, node.EvidenceID, node.ImpactMS, node.DuplicatePublications, node.ResourceCompletionClosure)
			}
		}
		if calibers[types.TraceIOValueCaliberRQResidence] != 2 || calibers[types.TraceIOValueCaliberBIOResidence] != 1 {
			t.Errorf("renderer swapped request rulers: reverse=%t %+v", reverse, calibers)
		}
		for i := 1; i < 3; i++ {
			if runtimeTraceProjTrunkSameStateOccurrencePair(ordered[0], ordered[i]) {
				t.Errorf("request publication entered the scheduler-state SUM arm: reverse=%t pair=%d", reverse, i)
			}
		}
		projection := types.TraceCausalProjection{BackgroundCauses: got}
		index := newRuntimeTraceCausalProjectionEvidenceIndex()
		buildRuntimeTraceProjTreeModel(projection, index, false)
		if len(index.order) != 3 {
			t.Fatalf("each request needs its own short evidence locator: got %d", len(index.order))
		}
		refs := map[string]bool{}
		for _, entry := range index.order {
			refs[entry.Ref] = true
		}
		if len(refs) != 3 {
			t.Fatal("evidence locators merged independent native endpoint lines")
		}
		blocks := runtimeTraceCausalProjectionCluster(projection, "en", runtimeTraceProjUserFocus{})
		for _, block := range blocks {
			if block.ID != "runtime_trace_causal_projection" && block.ID != "runtime_trace_causal_projection_detail_full" {
				continue
			}
			text := types.AnswerBlockVisibleSurface(block)
			for _, entry := range index.order {
				if !strings.Contains(text, entry.ID) {
					t.Errorf("surface %s lost distinct request locator %s", block.ID, entry.ID)
				}
			}
		}
	}
}
