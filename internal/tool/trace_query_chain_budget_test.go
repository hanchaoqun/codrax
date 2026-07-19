package tool

// CHAIN-BUDGET publication pins (user rulings 2026-07-18): a branch carrying
// budget-expanded extra segment sub-chains is a TREE — flattening it into one
// path string would re-mint the huadong_78 pseudo-linear pathology — so the
// branch path record serializes the guaranteed PRIMARY SPINE with a
// side_chains disclosure note, every side-chain edge publishes its own true
// leaf-to-target path note, and the LLM view budget (typed family row cap)
// stays separate from the exploration budget: a bigger chain never widens the
// published record population beyond the cap.
//
// MUTATION self-checks:
//   - reverting the spine walk (sorting ALL branch nodes into the path) reds
//     TestTraceQueryChainBudgetBranchPathIsPrimarySpine;
//   - stamping the spine path onto side-chain edges reds
//     TestTraceQueryChainBudgetSideChainEdgeCarriesLeafPath;
//   - dropping the side_chains note reds the notes assertion;
//   - removing the row cap from the path/edge record loops reds
//     TestTraceQueryChainBudgetViewCapBoundsPublishedRecords.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// chainBudgetShapeChain models the engine's CHAIN-BUDGET tree shape: one
// branch whose primary spine is depC -> depA -> target, with one extra
// segment sub-chain depB -> depA (segment_ordinal 2 — depA's top-2 sleep
// segment woken by depB).
func chainBudgetShapeChain() tracequery.ChainResult {
	target := tracequery.ThreadRef{Comm: "app", PID: 100}
	depA := tracequery.ThreadRef{Comm: "depA", PID: 200}
	depB := tracequery.ThreadRef{Comm: "depB", PID: 300}
	depC := tracequery.ThreadRef{Comm: "depC", PID: 400}
	chain := tracequery.ChainResult{
		Target: target,
		Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.05},
		Nodes: []tracequery.ChainNode{
			{ID: "n1", Thread: target, Branch: 1, Depth: 0},
			{ID: "n2", Thread: depA, Branch: 1, Depth: 1},
			{ID: "n3", Thread: depC, Branch: 1, Depth: 2},
			{ID: "n4", Thread: depB, Branch: 1, Depth: 2, SegmentOrdinal: 2},
		},
		Edges: []tracequery.WakeupEdge{
			{From: "n2", To: "n1", Branch: 1, Waker: depA, Wakee: target, WakeupTs: 5.040, WakeupLine: 19},
			{From: "n3", To: "n2", Branch: 1, Waker: depC, Wakee: depA, WakeupTs: 5.012, WakeupLine: 7},
			{From: "n4", To: "n2", Branch: 1, Waker: depB, Wakee: depA, WakeupTs: 5.020, WakeupLine: 11, SegmentOrdinal: 2},
		},
	}
	return chain
}

func TestTraceQueryChainBudgetBranchPathIsPrimarySpine(t *testing.T) {
	chain := chainBudgetShapeChain()
	branches := traceQueryWakeupChainBranches(chain)
	if len(branches) != 1 {
		t.Fatalf("one branch record per branch, got %d", len(branches))
	}
	br := branches[0]
	if want := "depC-400 -> depA-200 -> app-100"; br.Path != want {
		t.Fatalf("the branch path must be the primary spine (never a flattened tree walk):\n got %q\nwant %q", br.Path, want)
	}
	if br.Nodes != 4 || br.Edges != 3 {
		t.Fatalf("the record keeps the whole-branch account (4 nodes / 3 edges), got %d/%d", br.Nodes, br.Edges)
	}
	if br.SideChains != 1 {
		t.Fatalf("side_chains must count the branch's extra segment expansions, got %d", br.SideChains)
	}
	notes := traceQueryTypedWakeupBranchPathRichNotes(chain, br, len(branches))
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "side_chains=1") {
		t.Fatalf("the branch record must disclose its side chains, got notes:\n%s", joined)
	}
}

// The zero-side-chain lane keeps the legacy all-node serialization
// byte-identical (degenerate compatibility — every pre-CHAIN-BUDGET result
// and the tightest-budget engine output land here).
func TestTraceQueryChainBudgetLinearBranchKeepsLegacyPathAndNotes(t *testing.T) {
	chain := chainBudgetShapeChain()
	// Remove the side chain: linear branch.
	chain.Nodes = chain.Nodes[:3]
	chain.Edges = chain.Edges[:2]
	branches := traceQueryWakeupChainBranches(chain)
	if len(branches) != 1 {
		t.Fatalf("one branch record, got %d", len(branches))
	}
	br := branches[0]
	if want := "depC-400 -> depA-200 -> app-100"; br.Path != want {
		t.Fatalf("legacy linear path drifted: got %q want %q", br.Path, want)
	}
	if br.SideChains != 0 {
		t.Fatalf("a linear branch has no side chains, got %d", br.SideChains)
	}
	notes := strings.Join(traceQueryTypedWakeupBranchPathRichNotes(chain, br, 1), "\n")
	if strings.Contains(notes, "side_chains") {
		t.Fatalf("zero-emission: no side_chains note on a linear branch (byte-stable legacy notes), got:\n%s", notes)
	}
}

func TestTraceQueryChainBudgetSideChainEdgeCarriesLeafPath(t *testing.T) {
	chain := chainBudgetShapeChain()
	branches := traceQueryWakeupChainBranches(chain)
	resolver := traceQueryWakeupChainEdgePathResolver(chain, traceQueryWakeupChainBranchPathByID(branches))
	spinePath := "depC-400 -> depA-200 -> app-100"
	for _, edge := range chain.Edges {
		got, ok := resolver(edge)
		if !ok {
			t.Fatalf("resolver must resolve every branch-stamped edge, failed on %+v", edge)
		}
		if edge.SegmentOrdinal >= 2 {
			if want := "depB-300 -> depA-200 -> app-100"; got != want {
				t.Fatalf("a side-chain edge must carry its OWN leaf-to-target walk, never the spine it is not on:\n got %q\nwant %q", got, want)
			}
			continue
		}
		if got != spinePath {
			t.Fatalf("guaranteed-lane edges keep the branch spine path verbatim, got %q", got)
		}
	}
}

// 返工 P3⑤ — segment_ordinal 观测注面: the extra-lane segment ordinal the
// wire payload carries also travels on the observation-note text faces (the
// note reader and the JSON consumer see the same lane identity); the
// primary lane keeps zero-emission absence, mirroring the wire's omitempty
// form (legacy notes byte-stable).
func TestTraceQueryChainBudgetSegmentOrdinalOnNoteFaces(t *testing.T) {
	chain := chainBudgetShapeChain()
	for _, edge := range chain.Edges {
		notes := strings.Join(traceQueryTypedWakeupEdgeRichNotes(edge, "p"), "\n")
		summary := traceQueryWakeupEdgeSummary(edge)
		if edge.SegmentOrdinal >= 2 {
			if !strings.Contains(notes, "segment_ordinal=2") {
				t.Fatalf("extra-lane edge notes must carry the segment ordinal, got:\n%s", notes)
			}
			if !strings.Contains(summary, "segment_ordinal=2") {
				t.Fatalf("extra-lane edge summary must carry the segment ordinal, got %q", summary)
			}
			continue
		}
		if strings.Contains(notes, "segment_ordinal") || strings.Contains(summary, "segment_ordinal") {
			t.Fatalf("primary-lane faces keep zero-emission absence (byte-stable legacy notes), got notes:\n%s\nsummary %q", notes, summary)
		}
	}
}

// Pin ⑩ — 视图帽在岗: the LLM-facing path/edge record population is bounded
// by the typed family row cap regardless of how large the exploration budget
// let the chain grow (视图预算与探索预算分离).
func TestTraceQueryChainBudgetViewCapBoundsPublishedRecords(t *testing.T) {
	target := tracequery.ThreadRef{Comm: "app", PID: 100}
	chain := tracequery.ChainResult{
		Target: target,
		Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 6.0},
	}
	// 60 branches, each a one-hop chain: well beyond the row cap.
	for branch := 1; branch <= 60; branch++ {
		waker := tracequery.ThreadRef{Comm: fmt.Sprintf("w%03d", branch), PID: 1000 + branch}
		targetID := fmt.Sprintf("n%d", 2*branch-1)
		wakerID := fmt.Sprintf("n%d", 2*branch)
		chain.Nodes = append(chain.Nodes,
			tracequery.ChainNode{ID: targetID, Thread: target, Branch: branch, Depth: 0},
			tracequery.ChainNode{ID: wakerID, Thread: waker, Branch: branch, Depth: 1})
		chain.Edges = append(chain.Edges, tracequery.WakeupEdge{
			From: wakerID, To: targetID, Branch: branch, Waker: waker, Wakee: target,
			WakeupTs: 5.0 + float64(branch)*0.01, WakeupLine: 100 + branch,
		})
	}
	result := tracequery.Result{View: "wakeup_chain", SourcePath: "chain_budget_cap.systrace", WakeupChain: &chain}
	records := traceQueryTypedObservations(result, "chain_budget_cap.systrace", "payload", "raw-ref", "", time.Unix(1752800000, 0).UTC())
	cap := traceQueryWidthTypedFamilyRowCap()
	pathRecords, edgeRecords := 0, 0
	for _, record := range records {
		switch record.Predicate {
		case "wakeup_chain":
			pathRecords++
		case "wakeup_chain_edge":
			edgeRecords++
		}
	}
	if pathRecords == 0 || edgeRecords == 0 {
		t.Fatalf("expected path and edge records, got %d/%d", pathRecords, edgeRecords)
	}
	if pathRecords > cap {
		t.Fatalf("path records must stay within the typed family row cap %d, got %d", cap, pathRecords)
	}
	if edgeRecords > cap {
		t.Fatalf("edge records must stay within the typed family row cap %d, got %d", cap, edgeRecords)
	}
}
