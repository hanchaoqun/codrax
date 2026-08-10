package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// maybeShortCircuitChitchat ends a read-mode run right after analyze
// when the analyzer classified the turn as pure chitchat AND authored a
// reply (CHATFIX-1, customer log 2026-08-10: the REPL turn-policy
// classifier hard-times-out at 10s on slow self-hosted models — qwen
// first byte 30s+ — so a bare 「你好」 fell through into a full explore
// burn: 12 tool calls, 7m42s, Ctrl+C). This is the second line of
// defense behind that classifier; it also covers the classifier being
// disabled or misrouting.
//
// Every gate is a precise typed signal (架构红线: hard behavior changes
// key on precise signals only):
//
//   - RequestModel.Scenario == ScenarioChitchat — a schema-constrained
//     LLM enum choice (never a system-side keyword match; the analyzer
//     LLM makes the classification, reconcileScenario never mints it);
//   - non-empty analyzer-authored ChitchatReply — model-owned content,
//     rendered verbatim (系统不可代替 LLM 写用户面板答案: the system
//     supplies zero words of the reply);
//   - no attached runtime artifacts — an attached log/trace means the
//     user wants analysis regardless of greeting-shaped wording, so
//     the short-circuit refuses and the normal pipeline runs.
//
// Degenerate emissions (chitchat scenario with an empty reply) fail
// OPEN to the normal pipeline: worst case is exactly today's behavior,
// never a stranded turn. Terminal shape mirrors the empty-repo read
// short-circuit (SetResultPlain + IsTerminal + StageFinalize +
// EventPipelineEnd).
func (o *Orchestrator) maybeShortCircuitChitchat(ir *types.AnalysisIR) bool {
	if ir == nil {
		return false
	}
	if ir.RequestModel.Scenario != types.ScenarioChitchat {
		return false
	}
	reply := strings.TrimSpace(ir.RequestModel.ChitchatReply)
	if reply == "" {
		logging.Warning("[orchestrator] chitchat scenario WITHOUT a reply (degenerate emission) — failing open to the normal pipeline")
		return false
	}
	if strings.TrimSpace(o.busCtx.AttachedLog) != "" || strings.TrimSpace(o.busCtx.AttachedHitrace) != "" {
		logging.Info("[orchestrator] chitchat scenario with attached runtime artifact — refusing short-circuit, running full analysis")
		return false
	}
	logging.Info("[orchestrator] chitchat short-circuit: analyzer classified pure smalltalk; answering with the analyzer-authored reply (no exploration)")
	o.busCtx.Mutable.SetResultPlain(reply)
	o.busCtx.TaskState.IsTerminal = true
	o.busCtx.TaskState.Stage = types.StageFinalize
	o.busCtx.PipelineStage = types.StageFinalize
	// No EventPipelineEnd here (复核 F-F): unlike the empty-repo lane —
	// which returns EARLY from Run and must emit its own end event —
	// this lane returns through runTaskPhase and the normal Run tail
	// emits pipeline end exactly once.
	return true
}
