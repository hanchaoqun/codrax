package orchestrator

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// System-driven explorer fan-out — parallelism decision pushed from the
// LLM (which had to call propose_sub_agents) to the system, gated on
// PRECISE typed signals already on the AnalysisIR. See architecture.md
// §1.5: precise signals drive hard gates, noisy signals drive soft
// guidance. The LLM's call to propose_sub_agents remains a valid
// alternative path; system-driven fan-out is an EARLIER trigger when
// the analyzer has already done the typed decomposition.
//
// Forensic anchor (2026-05-17 explorer-budget audit): the
// "compare codrax and opencode" run produced 3 typed sub-topics with
// disjoint entity sets and IsCrossComponent=true, yet the explorer
// chewed through them serially over ~5 minutes because the LLM did not
// call propose_sub_agents (the tool's description biases toward "use
// when modules are UNRELATED" and the LLM judged cross-component
// comparison too coupled). System gate corrects this without
// retraining or rewording the LLM-facing prompt.

const (
	// subTopicFanoutMinSubTopics is the floor below which parallel
	// dispatch is wasted overhead. With 1 sub-topic there is nothing
	// to parallelise.
	subTopicFanoutMinSubTopics = 2

	// subTopicFanoutMaxSubTopics mirrors SubAgentValidator.maxSubTasks
	// (= 8) so the validator never rejects a system-built proposal
	// on cardinality. If the analyzer ever emits > 8 sub-topics, the
	// gate stays off and the single explorer handles the full set.
	subTopicFanoutMaxSubTopics = 8
)

// explorerSubTopicFanoutEligible decides whether the typed sub-topic
// decomposition justifies parallel sub_explorer dispatch.
//
// PRECISE-SIGNAL invariant (red-line §1.5): every condition below is a
// boolean / integer / set-disjointness check over typed AnalysisIR
// fields. No prose inspection, no LLM-confidence scoring, no string
// substring match against the raw request.
//
// All conditions must hold:
//
//  1. len(SubTopics) ∈ [2, 8]
//  2. Predicates.IsCrossComponent OR Predicates.IsCategoryEnumeration
//     — typed shapes where per-topic answers are independent.
//     Scalar-count / role-locate / config-precedence questions have a
//     single load-bearing answer and are NOT decomposable.
//  3. NOT (IsScalarAnswer AND IsCountQuestion) — the count IS the
//     answer; member sets are support evidence, not principal slates.
//  4. Each SubTopic carries non-empty Entities OR Scopes. The
//     downstream validator requires a non-empty Scope[] on every
//     SubAgentRequest; a sub-topic with neither cannot be assembled.
//  5. Sub-topic entity sets are pairwise DISJOINT after
//     NormalizeCodeKey. Overlap implies the sub-explorers would
//     duplicate read work AND the append-only reducer would bloat
//     evidence with near-duplicates.
func explorerSubTopicFanoutEligible(ir *types.AnalysisIR) bool {
	if ir == nil {
		return false
	}
	rm := ir.RequestModel

	if len(rm.SubTopics) < subTopicFanoutMinSubTopics ||
		len(rm.SubTopics) > subTopicFanoutMaxSubTopics {
		return false
	}

	p := rm.Predicates
	if !p.IsCrossComponent && !p.IsCategoryEnumeration {
		return false
	}

	// Negative carve-out: scalar count answers are never decomposable.
	if p.IsScalarAnswer && p.IsCountQuestion {
		return false
	}

	// Per-sub-topic shape gate: must have at least one typed identifier
	// surface so a non-empty Scope[] can be assembled. The validator
	// hard-rejects empty Scope[].
	for i := range rm.SubTopics {
		st := rm.SubTopics[i]
		if len(st.Entities) == 0 && len(st.Scopes) == 0 {
			return false
		}
	}

	// Pairwise entity disjointness (NormalizeCodeKey-equivalence).
	// Implementation: build a one-pass map of normalized key → owning
	// sub-topic index. Any second-seen key whose owner differs from
	// the current sub-topic index aborts the gate.
	owner := make(map[string]int, 16)
	for i := range rm.SubTopics {
		for _, ent := range rm.SubTopics[i].Entities {
			key := normalizer.NormalizeCodeKey(strings.TrimSpace(ent))
			if key == "" {
				continue
			}
			if prev, ok := owner[key]; ok && prev != i {
				return false
			}
			owner[key] = i
		}
	}

	return true
}

// buildSystemSubAgentProposal assembles a SubAgentProposal from the
// analyzer's typed sub_topic decomposition. Only called when
// explorerSubTopicFanoutEligible returns true; the result is fed
// directly to SubAgentRuntime.Run.
//
// Per-sub-topic scope assembly:
//
//  1. Use SubTopic.Scopes verbatim if non-empty (post-emit
//     scope_projection already mapped entity → sub-repo names).
//  2. Otherwise fall back to deduped entities. The sub_explorer's
//     buildScopedSearchGraph uses looksLikePath to filter path-shaped
//     scope entries; non-path entries still satisfy the validator's
//     "non-empty scope" requirement and surface to the sub_explorer
//     prompt as investigation hints.
//
// Returns nil if the proposal would have fewer than
// subTopicFanoutMinSubTopics valid sub-tasks after assembly (defensive
// — explorerSubTopicFanoutEligible already guards the same
// invariant).
func buildSystemSubAgentProposal(ir *types.AnalysisIR) *types.SubAgentProposal {
	if ir == nil {
		return nil
	}
	rm := ir.RequestModel

	subTasks := make([]types.SubTask, 0, len(rm.SubTopics))
	for i := range rm.SubTopics {
		st := rm.SubTopics[i]

		scope := dedupedNonEmpty(st.Scopes)
		if len(scope) == 0 {
			scope = dedupedNonEmpty(st.Entities)
		}
		if len(scope) == 0 {
			continue
		}

		objective := strings.TrimSpace(st.Summary)
		if objective == "" {
			// Validator hard-rejects empty objective. Fall back to a
			// synthesised summary from entities so the dispatch path
			// never crashes on a degenerate analyzer emit.
			objective = fmt.Sprintf("Investigate %s", strings.Join(scope, ", "))
		}

		subTasks = append(subTasks, types.SubTask{
			ID:        fmt.Sprintf("system-st-%d", i),
			Title:     objective,
			SubAgent:  "explorer", // SubExplorer.Name() returns "explorer"
			Objective: objective,
			Scope:     scope,
		})
	}

	if len(subTasks) < subTopicFanoutMinSubTopics {
		return nil
	}

	return &types.SubAgentProposal{
		Reason:     "system-driven: analyzer produced disjoint sub-topics with parallelisable shape",
		Goal:       "parallel evidence collection per typed sub-topic",
		SubTasks:   subTasks,
		ReduceHint: "sub-topics are independent investigations; append evidence by sub-topic",
	}
}

// executeExplorerStageAgent is the unified explorer-stage entry point
// invoked from dispatchStage. It prefers system-driven parallel
// sub_explorer fan-out when the precise-signal gate fires, and falls
// back to the single LLM explorer otherwise.
//
// Fall-back triggers (any of):
//   - explorerSubTopicFanoutEligible returned false (gate off)
//   - buildSystemSubAgentProposal returned nil (assembly degenerate)
//   - o.subRuntime is nil (SubAgentRuntime not wired — unit-test path)
//   - SubAgentRuntime.Run returned an error (validator reject /
//     sub-explorer crash / merge failure)
//   - SubAgentRuntime.Run returned a nil merged output
//
// On fall-back, the LLM explorer runs as if the system fan-out had
// never been attempted — original semantics fully preserved. The
// LLM-triggered propose_sub_agents path downstream (orchestrator.go:6384)
// still operates if the LLM emits the tool call; system fan-out is an
// additive earlier trigger, not a replacement.
//
// dispatchStage callers must use this helper ONLY for stage ==
// StageExplore. Other stages have no fan-out hook and call ag.Execute
// directly.
func (o *Orchestrator) executeExplorerStageAgent(
	ag agent.Agent,
	agentCtx *types.AgentContext,
	sk *skill.Config,
	stage types.PipelineStage,
	agentName types.AgentName,
) (*agent.StageOutput, error) {
	if o == nil || o.subRuntime == nil || o.busCtx == nil {
		return ag.Execute(agentCtx, sk)
	}
	if !explorerSubTopicFanoutEligible(o.busCtx.AnalysisIR) {
		return ag.Execute(agentCtx, sk)
	}
	proposal := buildSystemSubAgentProposal(o.busCtx.AnalysisIR)
	if proposal == nil {
		return ag.Execute(agentCtx, sk)
	}

	merged, err := o.dispatchSystemSubAgentFanout(stage, agentName, proposal)
	if err != nil || merged == nil {
		if err != nil {
			logging.Warning("[orchestrator] system fan-out failed (%v); falling back to single LLM explorer", err)
		}
		return ag.Execute(agentCtx, sk)
	}
	return merged, nil
}

// dispatchSystemSubAgentFanout invokes SubAgentRuntime.Run with a
// system-built proposal and surfaces the sub-agent start/end events to
// the renderer. Pure parallel-execution wrapper; no fall-back decisions
// here (the caller chooses single vs fan-out).
//
// UI thread-safety contract:
//
//   - render.EventEmitter is documented "safe for concurrent use"
//     (render/event.go:443); Renderer.handleEvent serialises every
//     event under r.mu (renderer_dock.go:51-53). All concurrent emit
//     calls from N sub_explorer goroutines + this dispatch wrapper
//     are therefore race-free.
//   - EventSubAgentStart creates one sub-agent row in the dock; the
//     sub_explorers' internal tool events route to the parent
//     (stage, agent) row via findRunningStageRow. The dock has been
//     handling this pattern since the LLM-triggered propose_sub_agents
//     path landed; no new UI invariant is introduced here.
//   - Free-floating NoticeSystemSubAgentFanoutStart is emitted FIRST
//     so the user sees a single readable status line ("› 并行探索 N
//     个独立方向") in both REPL dock and CLI scrollback, regardless
//     of how the dock chooses to coalesce N internal tool events.
//
// EventSubAgentStart / EventSubAgentEnd mirror the LLM-triggered path
// at orchestrator.go:6391 so the dock/UI sees identical surface chrome
// regardless of which trigger fired the fan-out.
//
// Cancellation: o.busCtx.Ctx is the parent context. SubAgentRuntime
// does not consume Context directly, but each sub_explorer's
// BaseAgent.Execute propagates cancellation via the LLM adapter's ctx
// argument. If the user cancels (Ctrl+C / `/exit`) mid-fan-out, all
// in-flight sub_explorer LLM calls return promptly and the wait group
// drains.
func (o *Orchestrator) dispatchSystemSubAgentFanout(
	stage types.PipelineStage,
	agentName types.AgentName,
	proposal *types.SubAgentProposal,
) (*agent.StageOutput, error) {
	logging.Info("[orchestrator] system fan-out: %d sub-task(s) (typed sub-topic decomposition)", len(proposal.SubTasks))

	// User-visible status line — free-floating progress-class
	// (青色 ›) notice routed to scrollback, NOT row-tied. Mirrors
	// selfConsistencyReviewStartMessage / semanticQualityReviewStartMessage
	// so all three "system is doing extra work in parallel" surfaces
	// share one visual idiom across REPL + CLI.
	lang := ""
	if o.busCtx != nil {
		lang = o.busCtx.Language
	}
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeSystemSubAgentFanoutStart,
		Reasoning:  systemSubAgentFanoutStartMessage(lang, len(proposal.SubTasks)),
	})

	subTitle := ""
	if len(proposal.SubTasks) > 0 {
		subTitle = proposal.SubTasks[0].Title
	}
	subAgentID := o.busCtx.TraceID + "-system-fanout"

	o.emit(render.Event{
		Kind:         render.EventSubAgentStart,
		Timestamp:    time.Now(),
		Stage:        stage,
		SubAgentName: string(agentName),
		SubAgentID:   subAgentID,
		SubTaskTitle: subTitle,
		SubTaskCount: len(proposal.SubTasks),
	})

	merged, runErr := o.subRuntime.Run(o.busCtx, proposal)

	subErr := ""
	subTools := 0
	subFacts := 0
	if runErr != nil {
		subErr = runErr.Error()
	} else if merged != nil {
		subTools = len(merged.ToolResults)
		subFacts = len(merged.NewFacts)
	}
	o.emit(render.Event{
		Kind:          render.EventSubAgentEnd,
		Timestamp:     time.Now(),
		Stage:         stage,
		SubAgentName:  string(agentName),
		SubAgentID:    subAgentID,
		ToolCallCount: subTools,
		FactCount:     subFacts,
		Error:         subErr,
	})

	return merged, runErr
}

// dedupedNonEmpty trims, drops empties, and deduplicates a string
// slice while preserving first-seen order. Used to assemble Scope[]
// entries without losing analyzer-emitted ordering.
func dedupedNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
