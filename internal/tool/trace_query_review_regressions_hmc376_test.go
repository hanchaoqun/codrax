package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The running breakdown names only work the target ran: a recorded wait is
// priced from the sleep it covers and cannot be the largest part of running time.
func TestHMC376RunningBreakdownNeverPointsAtARecordedWait(t *testing.T) {
	const target = "ease.cloudmusic-11077"
	row := func(badge int, tag, span string, impact float64) runtimeTraceProjTreeRow {
		return runtimeTraceProjTreeRow{
			Kind: runtimeTraceProjTreeRowSemantic, HasData: true, Badge: badge, EvidenceTag: tag,
			Node: types.TraceCausalProjectionNode{Subject: target, SpanName: span, EffectiveImpactMS: impact},
		}
	}
	model := runtimeTraceProjTreeModel{Target: target, TreeRows: []runtimeTraceProjTreeRow{
		row(1, "E1", "GC: Wait For Completion Alloc", 123.003),
		row(2, "E2", "JIT compiling void a.b()", 0.071),
	}}
	tag, count, best := runtimeTraceProjFourStateSemanticPointer(model)
	if tag != " ➋ [E2]" || count != 1 || best != 0.071 {
		t.Fatalf("pointer=%q count=%d best=%.3f, want the executed work only", tag, count, best)
	}
	// A vocabulary name that records work being executed stays in the breakdown.
	const executed = "Background concurrent copying GC"
	if hit, ok := tracequery.TraceSpanVocabulary(executed); !ok || hit.Stance != "executes" {
		t.Fatalf("control: %q must be a vocabulary row that executes work, got %+v ok=%v", executed, hit, ok)
	}
	model.TreeRows = append(model.TreeRows, row(3, "E3", executed, 12.5))
	if tag, count, best := runtimeTraceProjFourStateSemanticPointer(model); tag != " ➌ [E3]" || count != 2 || best != 12.5 {
		t.Fatalf("pointer=%q count=%d best=%.3f, want the executed collection to lead the running breakdown", tag, count, best)
	}
	model.TreeRows = model.TreeRows[:1]
	if tag, count, _ := runtimeTraceProjFourStateSemanticPointer(model); tag != "" || count != 0 {
		t.Fatalf("a window with only a recorded wait has no running pointer, got %q count=%d", tag, count)
	}
}

// Whole capture: the chain member's IO wait keeps its seat in the blocking list
// when a longer wait of an off-chain thread becomes classified work.
func TestHMC376WholeOriginalOnChainIOWaitKeepsItsBlockingSeat(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_NETEASE_GCWAIT",
		"ae52285a00c4afce43e48bae519615110973052370537e7c53f5e4351073f2e0", 106869666)
	native := hmc376Query(t, path, "critical_blocking_calls", 34044, 5775.523119, 5775.790999)
	if native.CriticalBlocking == nil {
		t.Fatal("the view must publish its list")
	}
	onChainIO, offChainWait := false, false
	for _, item := range native.CriticalBlocking.Items {
		onChainIO = onChainIO || (item.Thread.PID == 25647 && item.ChainRelevance == "on_chain")
		offChainWait = offChainWait || item.Thread.PID == 34235
	}
	if !onChainIO || offChainWait {
		t.Fatalf("on-chain IO wait kept=%v, off-chain collection wait seated=%v", onChainIO, offChainWait)
	}
}

// Whole capture: a collection and the phases recorded inside it are one stretch of
// time. Nothing published for the collector can exceed the analysed window.
func TestHMC376WholeOriginalCollectorPhasesAreNotAddedToTheirCollection(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_NETEASE_REBOOT_FRAME",
		"48cf2fd8b91a81f2276a16a5988ef14729c078e5d1f23615eb166d2f6e2c8cbc", 104144939)
	const windowMS = 123.745
	result, _ := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 11077, "target_scope": "thread"}, 153.763595, 153.887340)
	projection := hmc376Projection(result)
	seen := false
	for _, nodes := range [][]types.TraceCausalProjectionNode{projection.OnChainCauses, projection.AdjacentCauses, projection.BackgroundCauses} {
		for _, node := range nodes {
			if !strings.Contains(node.Subject, "HeapTaskDaemon-11098") || node.Object != "trace_span" {
				continue
			}
			seen = true
			if node.ImpactMS > windowMS+0.0015 {
				t.Fatalf("%s publishes %.3f ms in a %.3f ms window (merged=%d)", node.Subject, node.ImpactMS, windowMS, node.MergedCount)
			}
		}
	}
	if !seen {
		t.Fatal("control: the collector's span rows must be published")
	}
}

// The thread choices a diagram may select read in thread order, not in the order
// of lane ids that hash where the capture was read from.
func TestHMC376DiagramThreadChoicesFollowTheThreadLabel(t *testing.T) {
	blockProps := map[string]any{"diagram": map[string]any{"properties": map[string]any{}}}
	view := &types.AnswerSemanticView{DiagramPlan: &types.DiagramFacetGraph{Kind: types.DiagramSequence}}
	projectNativeDiagramParticipants(blockProps, view, []RuntimeDiagramRelation{{
		FromNode: "rt_thread_aaaa", FromLabel: "sensors-9569", ToNode: "rt_thread_ffff", ToLabel: "Jit thread pool-17284",
	}})
	participants := blockProps["diagram"].(map[string]any)["properties"].(map[string]any)["runtime_participants_v1"].(map[string]any)
	choices := participants["items"].(map[string]any)["oneOf"].([]any)
	first := choices[0].(map[string]any)["properties"].(map[string]any)["native_participant"].(map[string]any)["enum"].([]string)[0]
	if len(choices) != 2 || first != "rt_thread_ffff" {
		t.Fatalf("choices=%d first=%q, want the alphabetically first thread label first", len(choices), first)
	}
}
