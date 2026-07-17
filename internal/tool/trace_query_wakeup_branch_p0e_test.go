package tool

// P0-E CHAIN-PATH 根修 publication pins (ledger §22.1, huadong_78 §24.11-A
// witness, 2026-07-09): the wakeup_chain path face publishes ONE record per
// REAL branch — the cross-branch flattened walk is retired from every
// publication face for branch-stamped engine results. The specimen shape is
// the huadong_78 29-element dump: seven VSync→oney ping-pong branches plus
// one deep branch; the flattened walk serialized them as
// "… -> oney⇄VSync ×7 …" and minted fake L26/L27 trunk depths downstream.
//
// MUTATION self-checks:
//   - re-routing branch-stamped results through traceQueryWakeupChainPath
//     (reverting the publication switch) reds
//     TestTraceQueryWakeupChainRecordsPerBranchP0E (the flattened Object
//     reappears) and TestTraceQueryWakeupChainBranchPathsHuadongShape (the
//     ping-pong serialization reappears);
//   - dropping the branch= note reds the notes assertion;
//   - deleting the legacy fallback reds
//     TestTraceQueryWakeupChainLegacyFixtureKeepsFlattenedRecord.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// huadongShapeChain models the huadong_78 witness (§24.11-A): the target
// oney.hmn.berlin-42591 expanded eight segments — seven shallow VSync wakeups
// (the ping-pong fodder) and one deep branch tppmgr → gpu-pm-release →
// RSUniRenderThre → target.
func huadongShapeChain() tracequery.ChainResult {
	target := tracequery.ThreadRef{Comm: "oney.hmn.berlin", PID: 42591}
	vsync := tracequery.ThreadRef{Comm: "VSyncGenerator", PID: 2270}
	chain := tracequery.ChainResult{
		Target: target,
		Window: tracequery.TimeWindow{StartTs: 6793224.900, EndTs: 6793225.001},
	}
	nodeID := 0
	mint := func(thread tracequery.ThreadRef, branch, depth int, impactMs float64) string {
		nodeID++
		id := fmt.Sprintf("n%d", nodeID)
		node := tracequery.ChainNode{ID: id, Thread: thread, Branch: branch, Depth: depth}
		if impactMs > 0 {
			node.Impact = &tracequery.WakeupCausalImpact{
				Thread: thread, ChainDepth: depth, ChainBranch: branch,
				TotalMs: impactMs, DominantImpactMs: impactMs,
				DominantState: string(tracequery.StateSSleep),
				LineStart:     100 * nodeID, LineEnd: 100*nodeID + 1,
			}
			chain.CausalImpacts = append(chain.CausalImpacts, *node.Impact)
		}
		chain.Nodes = append(chain.Nodes, node)
		return id
	}
	ts := 6793224.910
	for branch := 1; branch <= 7; branch++ {
		targetID := mint(target, branch, 0, 0)
		vsyncID := mint(vsync, branch, 1, 1.2)
		chain.Edges = append(chain.Edges, tracequery.WakeupEdge{
			From: vsyncID, To: targetID, Branch: branch,
			Waker: vsync, Wakee: target,
			WakeupTs: ts, WakeupLine: 1000 + branch,
		})
		ts += 0.010
	}
	rsuni := tracequery.ThreadRef{Comm: "RSUniRenderThre", PID: 1963}
	gpupm := tracequery.ThreadRef{Comm: "gpu-pm-release", PID: 327}
	tppmgr := tracequery.ThreadRef{Comm: "tppmgr-idle-0", PID: 185}
	targetID := mint(target, 8, 0, 0)
	rsuniID := mint(rsuni, 8, 1, 6.4)
	gpupmID := mint(gpupm, 8, 2, 3.1)
	tppmgrID := mint(tppmgr, 8, 3, 2.0)
	chain.Edges = append(chain.Edges,
		tracequery.WakeupEdge{From: rsuniID, To: targetID, Branch: 8, Waker: rsuni, Wakee: target, WakeupTs: ts, WakeupLine: 2001},
		tracequery.WakeupEdge{From: gpupmID, To: rsuniID, Branch: 8, Waker: gpupm, Wakee: rsuni, WakeupTs: ts - 0.002, WakeupLine: 2002},
		tracequery.WakeupEdge{From: tppmgrID, To: gpupmID, Branch: 8, Waker: tppmgr, Wakee: gpupm, WakeupTs: ts - 0.004, WakeupLine: 2003},
	)
	return chain
}

const huadongDeepPath = "tppmgr-idle-0-185 -> gpu-pm-release-327 -> RSUniRenderThre-1963 -> oney.hmn.berlin-42591"

func TestTraceQueryWakeupChainBranchPathsHuadongShape(t *testing.T) {
	chain := huadongShapeChain()
	branches := traceQueryWakeupChainBranches(chain)
	if len(branches) != 8 {
		t.Fatalf("expected 8 branch paths, got %d: %+v", len(branches), branches)
	}
	for _, br := range branches {
		if br.Branch <= 7 {
			want := "VSyncGenerator-2270 -> oney.hmn.berlin-42591"
			if br.Path != want {
				t.Fatalf("branch %d path\n got %q\nwant %q", br.Branch, br.Path, want)
			}
		}
		// A REAL branch path terminates at the target BY CONSTRUCTION — the
		// target never appears mid-path, so the oney⇄VSync ping-pong ladder
		// cannot serialize on this lane.
		if strings.Contains(br.Path, "oney.hmn.berlin-42591 -> ") {
			t.Fatalf("branch %d path re-serializes the target mid-path (ping-pong shape): %q", br.Branch, br.Path)
		}
	}
	if branches[7].Path != huadongDeepPath {
		t.Fatalf("deep branch path\n got %q\nwant %q", branches[7].Path, huadongDeepPath)
	}
	if branches[7].Edges != 3 || branches[7].Nodes != 4 {
		t.Fatalf("deep branch counts must be branch-scoped, got edges=%d nodes=%d", branches[7].Edges, branches[7].Nodes)
	}
	// Regression witness: the RETIRED flattened walk on this same shape
	// stitches the ping-pong ladder — exactly what the per-branch face
	// removes from every publication surface.
	if flattened := traceQueryWakeupChainPath(chain); !strings.Contains(flattened, "oney.hmn.berlin-42591 -> VSyncGenerator-2270") {
		t.Fatalf("specimen self-check: the legacy flattened walk should exhibit the ping-pong shape, got %q", flattened)
	}
}

func TestTraceQueryWakeupChainRecordsPerBranchP0E(t *testing.T) {
	chain := huadongShapeChain()
	result := tracequery.Result{WakeupChain: &chain}
	records := traceQueryTypedObservations(result, "huadong.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	flattened := traceQueryWakeupChainPath(chain)
	var paths []string
	claimKeys := map[string]bool{}
	for _, record := range records {
		if record.Predicate != "wakeup_chain" {
			continue
		}
		if record.Object == flattened {
			t.Fatalf("the retired flattened walk must not publish for a branch-stamped result: %q", record.Object)
		}
		if claimKeys[record.ClaimKey] {
			t.Fatalf("duplicate wakeup_chain claim key %q — per-branch records need distinct keys", record.ClaimKey)
		}
		claimKeys[record.ClaimKey] = true
		// RANKDIS-EXT A2 (§29.104.16.1 M2): the per-branch key wears the same
		// branch= word as the typed branch= rich note — the retired path#N
		// spelling shared the #N ordinal glyph with board-seat chips.
		if !strings.HasPrefix(record.ClaimKey, "wakeup_chain:branch=") {
			t.Fatalf("per-branch claim key shape wanted wakeup_chain:branch=<branch>, got %q", record.ClaimKey)
		}
		if strings.Contains(record.ClaimKey, "path#") {
			t.Fatalf("per-branch claim key must not wear the retired path#N ordinal glyph, got %q", record.ClaimKey)
		}
		hasBranch, hasBranches := false, false
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, "branch=") {
				hasBranch = true
			}
			if note == "branches=8" {
				hasBranches = true
			}
		}
		if !hasBranch || !hasBranches {
			t.Fatalf("per-branch record must carry typed branch=/branches= notes, got %v", record.RichNotes)
		}
		paths = append(paths, record.Object)
	}
	if len(paths) != 8 {
		t.Fatalf("expected 8 per-branch wakeup_chain records, got %d: %v", len(paths), paths)
	}
	found := false
	for _, p := range paths {
		if p == huadongDeepPath {
			found = true
		}
	}
	if !found {
		t.Fatalf("deep branch path record missing from publication: %v", paths)
	}
}

// Fail-open: an identity-less legacy fixture (no ChainNode.Branch) keeps the
// single flattened record — the degraded lane never goes dark.
func TestTraceQueryWakeupChainLegacyFixtureKeepsFlattenedRecord(t *testing.T) {
	chain := huadongShapeChain()
	for i := range chain.Nodes {
		chain.Nodes[i].Branch, chain.Nodes[i].Depth = 0, 0
	}
	for i := range chain.Edges {
		chain.Edges[i].Branch = 0
	}
	for i := range chain.CausalImpacts {
		chain.CausalImpacts[i].ChainBranch = 0
	}
	result := tracequery.Result{WakeupChain: &chain}
	records := traceQueryTypedObservations(result, "legacy.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	var chainRecords []string
	for _, record := range records {
		if record.Predicate == "wakeup_chain" {
			chainRecords = append(chainRecords, record.ClaimKey)
		}
	}
	if len(chainRecords) != 1 || chainRecords[0] != "wakeup_chain:path" {
		t.Fatalf("legacy fixture must keep exactly the one flattened record, got %v", chainRecords)
	}
}

// Per-edge records name their OWNING branch's path — never the retired
// flattened string.
func TestTraceQueryWakeupEdgeRecordsCarryOwningBranchPathP0E(t *testing.T) {
	chain := huadongShapeChain()
	result := tracequery.Result{WakeupChain: &chain}
	records := traceQueryTypedObservations(result, "huadong.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	deepEdgeSeen := false
	for _, record := range records {
		if record.Predicate != "wakeup_chain_edge" {
			continue
		}
		if record.Subject != "tppmgr-idle-0-185" {
			continue
		}
		deepEdgeSeen = true
		want := "path=" + huadongDeepPath
		found := false
		for _, note := range record.RichNotes {
			if note == want {
				found = true
			}
			if strings.HasPrefix(note, "path=") && note != want {
				t.Fatalf("deep edge carries a foreign path note %q, want %q", note, want)
			}
		}
		if !found {
			t.Fatalf("deep edge must carry its owning branch path, notes=%v", record.RichNotes)
		}
	}
	if !deepEdgeSeen {
		t.Fatal("deep branch edge record missing (wire cap ate it?)")
	}
}
