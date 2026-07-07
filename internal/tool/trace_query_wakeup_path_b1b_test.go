package tool

// §22 B1-b F1(b) producer pins (huadong_01 CHAIN-PATH audit 2026-07-07): the
// wakeup_chain path record must terminate at the typed chain target.
//
// The flattening walk (traceQueryWakeupChainPath over traceQuerySortedWakeupEdges)
// orders edges by From-node Impact.ChainDepth DESCENDING, and nil-Impact
// transit nodes (no independent impact row this run) default to depth 0 — their
// edges sort LAST, so the serialized walk overshoots the target and ends on an
// artifact transit suffix while the true target sits mid-path (specimen: root =
// VSyncGenerator = path end, user thread 42591 mid-path, "…省略26节点" of
// stitched transits). The fix truncates the walk at the target label's LAST
// occurrence; target absent keeps the legacy full walk (fail-open — no hard
// gate on a noisy shape). Display/LLM face only: the wakeup_chain_edge records
// keep every edge, and the tracequery rank/attribution lanes never read this
// string (L 红线 untouched).
//
// MUTATION self-check: removing the truncation reds
// TestTraceQueryWakeupChainPathStopsAtTargetB1b (the walk end reverts to the
// transit artifact).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// b1bOvershootChain is the specimen shape: VSync (impact depth 1) wakes the
// target; a nil-impact transit wakes VSync — the transit edge sorts last and
// the un-truncated walk ends on VSync AFTER passing the target.
func b1bOvershootChain() tracequery.ChainResult {
	return tracequery.ChainResult{
		Target: tracequery.ThreadRef{Comm: "oney.hmn.berlin", PID: 42591},
		Window: tracequery.TimeWindow{StartTs: 1.0, EndTs: 2.0},
		Nodes: []tracequery.ChainNode{
			{ID: "n-target", Thread: tracequery.ThreadRef{Comm: "oney.hmn.berlin", PID: 42591}},
			{ID: "n-vsync", Thread: tracequery.ThreadRef{Comm: "VSyncGenerator", PID: 2270},
				Impact: &tracequery.WakeupCausalImpact{ChainDepth: 1, TotalMs: 4.4}},
			// The zero-impact transit: no independent impact row → Impact nil →
			// sorted depth defaults to 0 → its edge walks LAST.
			{ID: "n-transit", Thread: tracequery.ThreadRef{Comm: "tppmgr-idle-7", PID: 189}},
		},
		Edges: []tracequery.WakeupEdge{
			{From: "n-vsync", To: "n-target",
				Waker:    tracequery.ThreadRef{Comm: "VSyncGenerator", PID: 2270},
				Wakee:    tracequery.ThreadRef{Comm: "oney.hmn.berlin", PID: 42591},
				WakeupTs: 1.10, WakeupLine: 100},
			{From: "n-transit", To: "n-vsync",
				Waker:    tracequery.ThreadRef{Comm: "tppmgr-idle-7", PID: 189},
				Wakee:    tracequery.ThreadRef{Comm: "VSyncGenerator", PID: 2270},
				WakeupTs: 1.05, WakeupLine: 90},
		},
	}
}

func TestTraceQueryWakeupChainPathStopsAtTargetB1b(t *testing.T) {
	got := traceQueryWakeupChainPath(b1bOvershootChain())
	want := "VSyncGenerator-2270 -> oney.hmn.berlin-42591"
	if got != want {
		t.Fatalf("B1-b F1(b): the path record must terminate at the typed target,\n got %q\nwant %q", got, want)
	}
	if strings.Contains(got, "tppmgr") {
		t.Fatalf("B1-b F1(b): the post-target transit overshoot must not publish, got %q", got)
	}
}

// Fail-open: a chain whose target never appears on the walk (or carries no
// identity) keeps the legacy full walk — the truncation is a precise-signal
// trim, never a hard gate on a noisy shape.
func TestTraceQueryWakeupChainPathTargetAbsentKeepsWalkB1b(t *testing.T) {
	chain := b1bOvershootChain()
	chain.Target = tracequery.ThreadRef{Comm: "elsewhere", PID: 777}
	got := traceQueryWakeupChainPath(chain)
	want := "VSyncGenerator-2270 -> oney.hmn.berlin-42591 -> tppmgr-idle-7-189 -> VSyncGenerator-2270"
	if got != want {
		t.Fatalf("B1-b F1(b): a target absent from the walk keeps the legacy full path,\n got %q\nwant %q", got, want)
	}

	unset := b1bOvershootChain()
	unset.Target = tracequery.ThreadRef{}
	if got := traceQueryWakeupChainPath(unset); got != want {
		t.Fatalf("B1-b F1(b): an unset target keeps the legacy full path,\n got %q\nwant %q", got, want)
	}
}

// Well-formed walks (already target-terminated) are byte-identical under the
// truncation — the trim only ever removes a strict post-target suffix.
func TestTraceQueryWakeupChainPathWellFormedUnchangedB1b(t *testing.T) {
	chain := b1bOvershootChain()
	chain.Nodes = chain.Nodes[:2] // drop the transit node
	chain.Edges = chain.Edges[:1] // waker -> target only
	got := traceQueryWakeupChainPath(chain)
	want := "VSyncGenerator-2270 -> oney.hmn.berlin-42591"
	if got != want {
		t.Fatalf("B1-b F1(b): a well-formed walk must stay byte-identical,\n got %q\nwant %q", got, want)
	}
}
