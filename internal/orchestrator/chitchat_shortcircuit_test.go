package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// chitchat_shortcircuit_test.go — CHATFIX-1 (customer log 2026-08-10):
// the read pipeline answers a pure greeting with the analyzer-authored
// reply right after analyze, instead of burning explore rounds when
// the REPL turn-policy classifier timed out / misrouted / is disabled.

func chitchatIR(scenario types.Scenario, reply string) *types.AnalysisIR {
	return &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest:    "你好",
			Scenario:      scenario,
			ChitchatReply: reply,
		},
		TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{{ID: "n1", Type: types.NodeEvidence}}},
	}
}

func chitchatOrchestrator(ir *types.AnalysisIR) *Orchestrator {
	return &Orchestrator{
		busCtx: &types.BusContext{
			Mutable:    types.NewMutableState("你好"),
			AnalysisIR: ir,
			TaskState:  types.TaskState{},
		},
		emit: func(render.Event) {},
	}
}

func TestChitchatShortCircuitAnswersWithAnalyzerReply(t *testing.T) {
	reply := "你好！我是仓库分析助手，可以帮你调查代码结构、调用链或性能问题。"
	o := chitchatOrchestrator(chitchatIR(types.ScenarioChitchat, reply))
	if !o.maybeShortCircuitChitchat(o.busCtx.AnalysisIR) {
		t.Fatal("chitchat scenario with a reply must short-circuit")
	}
	if got := o.busCtx.Mutable.Result(); got != reply {
		t.Fatalf("result must be the analyzer-authored reply verbatim, got %q", got)
	}
	if !o.busCtx.Mutable.ResultIsPlain() {
		t.Fatal("chitchat reply must ride the plain-result lane (no document machinery)")
	}
	if !o.busCtx.TaskState.IsTerminal {
		t.Fatal("short-circuit must terminate the run")
	}
}

func TestChitchatShortCircuitEmptyReplyFailsOpen(t *testing.T) {
	// Degenerate emission: chitchat with no reply → the pipeline must
	// proceed normally (fail-open — worst case is today's behavior).
	o := chitchatOrchestrator(chitchatIR(types.ScenarioChitchat, "   "))
	if o.maybeShortCircuitChitchat(o.busCtx.AnalysisIR) {
		t.Fatal("empty reply must fail open to the normal pipeline")
	}
	if o.busCtx.TaskState.IsTerminal {
		t.Fatal("fail-open path must not terminate the run")
	}
}

func TestChitchatShortCircuitRefusesWithAttachedArtifact(t *testing.T) {
	// An attached log/trace means the user wants analysis regardless
	// of greeting-shaped wording.
	for name, mutate := range map[string]func(*types.BusContext){
		"log":     func(b *types.BusContext) { b.AttachedLog = "panic: nil deref" },
		"hitrace": func(b *types.BusContext) { b.AttachedHitrace = "trace body" },
	} {
		o := chitchatOrchestrator(chitchatIR(types.ScenarioChitchat, "hi"))
		mutate(o.busCtx)
		if o.maybeShortCircuitChitchat(o.busCtx.AnalysisIR) {
			t.Fatalf("[%s] attached artifact must veto the short-circuit", name)
		}
	}
}

func TestChitchatShortCircuitIgnoresAnalysisScenarios(t *testing.T) {
	// A stray reply on a non-chitchat scenario must never fire (the
	// emit layer drops it too — this is the second, independent arm).
	for _, s := range []types.Scenario{types.ScenarioGeneric, types.ScenarioArchitectureExplain, types.ScenarioRootCause} {
		o := chitchatOrchestrator(chitchatIR(s, "hi"))
		if o.maybeShortCircuitChitchat(o.busCtx.AnalysisIR) {
			t.Fatalf("scenario %s must never short-circuit", s)
		}
	}
}

func TestRunTaskGraphWiresChitchatShortCircuit(t *testing.T) {
	// 发布接线正向 pin: the guard must fire FROM runTaskGraph before the
	// scheduler loop — zero steps consumed, reply published. If the
	// call-site wiring is ever dropped, the loop would run with no
	// agents and the result would not be the reply.
	reply := "你好！有什么可以帮你的？"
	o := chitchatOrchestrator(chitchatIR(types.ScenarioChitchat, reply))
	if used := o.runTaskGraph(10); used != 0 {
		t.Fatalf("short-circuited run must consume zero steps, got %d", used)
	}
	if got := o.busCtx.Mutable.Result(); got != reply {
		t.Fatalf("runTaskGraph must publish the reply, got %q", got)
	}
}

func TestChitchatReplyNeverClaimsRepoFactsIsModelOwned(t *testing.T) {
	// Ownership doc-pin: the short-circuit renders ONLY the analyzer's
	// text — the system contributes zero words. Guard the helper
	// against future "helpful" system-side prefixes/suffixes.
	reply := "unique-marker-7f3a"
	o := chitchatOrchestrator(chitchatIR(types.ScenarioChitchat, reply))
	o.maybeShortCircuitChitchat(o.busCtx.AnalysisIR)
	if got := o.busCtx.Mutable.Result(); got != reply || strings.Contains(got, "codrax") {
		t.Fatalf("system must not decorate the model-authored reply, got %q", got)
	}
}
