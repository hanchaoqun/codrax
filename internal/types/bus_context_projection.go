package types

// bus_context_projection.go — Phase 3 of the commercial-grade
// remediation. Two named helpers replace the brittle field-by-field
// white-list pattern previously used at every BusContext / AgentContext
// narrowing site.
//
// The narrowing problem: tonight's mr_pin_isolation eval surfaced a
// silent typed-signal drop in agent/agent.go::buildToolBusContext —
// MultiGraph, TypedDenials, PendingSubRepos and the cancellation Ctx
// were missing from the BusContext that tools see, so the L1
// active-set gate's `if mg == nil` short-circuit fired and let
// LLM-side read_file calls cross sub-repo pin boundaries.
//
// White-list pattern is brittle: every new typed field added to
// BusContext / AgentContext must be hand-mirrored at every narrowing
// site. Forgetting one site silently downgrades a gate. Black-list
// (struct-copy + explicit drop) plus a compile-time-style test
// (TestBusContextProjection_AllTypedSignalsPropagated) makes the
// drop-set explicit and the propagation completeness self-pinning:
// future typed fields are auto-included; only intentionally-dropped
// fields show up in the explicit drop list.
//
// Two helpers because the narrowing direction differs:
//
//   - ToolBusContext(ctx, activeName) — AgentContext → BusContext
//     for tool dispatch (issued by BaseAgent.executeTool). Mutable
//     propagates as a pointer because tools may mutate the shared
//     task list / search graph through it; everything else is
//     read-only state the tool must observe.
//
//   - SubAgentContext(bus, req) — BusContext → AgentContext for
//     sub-agent dispatch (issued by BuildSubAgentContext). Mutable
//     is intentionally DROPPED — sub-agents are read-only views
//     of parent state, must not mutate the parent's Mutable.
//
// projectionTypedSignalFields enumerates the typed-signal field
// names every narrowing site MUST propagate. The build-time test
// reflects on BusContext / AgentContext, populates each listed
// field with a non-zero sentinel, runs the helper, and asserts the
// output preserves the field. Adding a new typed-signal field to
// BusContext (and its mirror in AgentContext) requires adding the
// field name here so the test fails until all narrowing sites are
// updated.
//
// "Typed signal" here means a field whose value affects gate
// enforcement (MultiGraph / TypedDenials / PendingSubRepos /
// MultiRepoInactivePreviewCount), tool capability (Memory /
// EnvFacts / EnvRecommendSettings / WorktreePath), or cancellation
// propagation (Ctx). Stage-scoped fields (RepoFacts / EvidenceItems /
// TaskState / FlowFindings) are NOT typed signals — they are
// stage outputs that get filtered per-sub-agent or rebuilt
// per-tool-call and don't belong in this list.

// projectionTypedSignalFields is the canonical list of typed-signal
// field names. Read by the build-time test
// TestBusContextProjection_AllTypedSignalsPropagated to verify every
// narrowing helper copies these fields. Order does not matter; the
// test indexes by name via reflect.
var projectionTypedSignalFields = []string{
	"MultiGraph",
	"SubRepos",
	"ActiveSubRepo",
	"PendingSubRepos",
	"MultiRepoInactivePreviewCount",
	"MultiRepoFocusDecision",
	"TypedDenials",
	"Memory",
	"Ctx",
	"EnvFacts",
	"EnvRecommendSettings",
	"EvalDisableGitHistory",
	"WorktreePath",
	"ReadDispatchPolicy",
	"SourceInventoryFollowupDebt",
}

// ProjectionTypedSignalFields returns a copy of the typed-signal
// field-name list. Exposed for tests outside the types package
// (e.g. agent / orchestrator integration tests that want to verify
// downstream consumers respect the propagation contract).
func ProjectionTypedSignalFields() []string {
	out := make([]string, len(projectionTypedSignalFields))
	copy(out, projectionTypedSignalFields)
	return out
}

// ToolBusContext narrows an AgentContext into a BusContext for tool
// dispatch. Every typed-signal field listed in
// projectionTypedSignalFields propagates so the tool sees the full
// gate-relevant state. Mutable propagates as a pointer because tools
// may write through it; the rest of BusContext is constructed fresh.
//
// activeName overrides ctx.AgentName when non-empty; the agent
// dispatch path uses it to track which agent issued the call.
//
// nil-safe: returns an empty BusContext when ctx is nil (matches the
// pre-projection buildToolBusContext behavior).
//
// TypedDenials propagates as the SAME pointer (both types store
// *TypedDenialSet): a denial the tool stamps during this call lands
// on the Run-level set, so subsequent tool calls' L1 gates, the L2
// prompt sanitiser, and the orchestrator's L3 answer validator all
// see it. A value copy here is the historic bug this contract pins
// against — tool-stamped denials silently died on the discarded
// per-call projection from 2026-05-08 until 2026-06-12.
func ToolBusContext(ctx *AgentContext, activeName AgentName) *BusContext {
	if ctx == nil {
		return &BusContext{}
	}
	if activeName == "" {
		activeName = ctx.AgentName
	}
	bc := &BusContext{
		Mutable:               ctx.Mutable,
		PipelineStage:         ctx.Stage,
		ActiveAgent:           activeName,
		RepoRoot:              ctx.RepoRoot,
		Branch:                ctx.Branch,
		Commit:                ctx.Commit,
		WorkDir:               ctx.WorkDir,
		MainRepoRoot:          ctx.MainRepoRoot,
		WorktreePath:          ctx.WorktreePath,
		AnalysisIR:            ctx.AnalysisIR,
		AttachedLog:           ctx.AttachedLog,
		AttachedHitrace:       ctx.AttachedHitrace,
		AttachedHitraceSource: ctx.AttachedHitraceSource,
		Language:              ctx.Language,
		Preferences:           ctx.Preferences,
		Mode:                  ctx.Mode,

		// Typed signals — every entry in projectionTypedSignalFields
		// must appear here. Adding a new typed signal to AgentContext
		// (and its BusContext mirror) requires updating this block;
		// TestBusContextProjection_AllTypedSignalsPropagated pins the
		// contract.
		MultiGraph:                    ctx.MultiGraph,
		SearchGraph:                   ctx.SearchGraph,
		SubRepos:                      ctx.SubRepos,
		ActiveSubRepo:                 ctx.ActiveSubRepo,
		PendingSubRepos:               ctx.PendingSubRepos,
		MultiRepoInactivePreviewCount: ctx.MultiRepoInactivePreviewCount,
		MultiRepoFocusDecision:        ctx.MultiRepoFocusDecision,
		TypedDenials:                  ctx.TypedDenials,
		Memory:                        ctx.Memory,
		Ctx:                           ctx.Ctx,
		EnvFacts:                      ctx.EnvFacts,
		EnvRecommendSettings:          ctx.EnvRecommendSettings,
		EvalDisableGitHistory:         ctx.EvalDisableGitHistory,
		ReadDispatchPolicy:            ctx.ReadDispatchPolicy,
		SourceInventoryFollowupDebt:   ctx.SourceInventoryFollowupDebt,
	}
	return bc
}

// SubAgentContext narrows a BusContext into an AgentContext for
// sub-agent dispatch. Mutable is intentionally DROPPED — sub-agents
// are read-only views of parent state and must not mutate the
// parent's task list / search graph. All other typed-signal fields
// propagate so the sub-agent's downstream tool calls see the same
// gate-relevant state as the parent.
//
// Stage-scoped artifacts (RepoFacts / EvidenceItems / FlowFindings /
// ToolResults / MCPResponses) are intentionally NOT propagated by
// this helper — the calling site (BuildSubAgentContext in
// internal/context/builder.go) applies the per-sub-agent scope
// filtering needed to project parent's full artifact lists down to
// the sub-agent's relevant subset.
//
// nil-safe: returns an empty AgentContext when bus is nil.
//
// TypedDenials propagates as the SAME pointer so tool denials stamped
// during sub-agent dispatch are visible to the parent (and vice
// versa). The sub-agent shares the Run-level denial set rather than
// getting a fresh copy.
func SubAgentContext(bus *BusContext, req *SubAgentRequest) *AgentContext {
	if bus == nil {
		return &AgentContext{}
	}
	subAgentName := AgentName("")
	objective := ""
	var constraints []string
	if req != nil {
		subAgentName = AgentName(req.SubAgent)
		objective = req.Objective
		constraints = append(append([]string{}, req.Scope...), req.Constraints...)
	}
	ac := &AgentContext{
		AgentName:    subAgentName,
		Stage:        bus.PipelineStage,
		Objective:    objective,
		Constraints:  constraints,
		MissingPiece: bus.TaskState.Missing,
		RepoRoot:     bus.RepoRoot,
		Branch:       bus.Branch,
		Commit:       bus.Commit,
		WorkDir:      bus.WorkDir,
		MainRepoRoot: bus.MainRepoRoot,
		WorktreePath: bus.WorktreePath,
		Mode:         bus.Mode,

		// Typed signals — every entry in projectionTypedSignalFields
		// must appear here. Adding a new typed signal to BusContext
		// (and its AgentContext mirror) requires updating this block.
		MultiGraph:                    bus.MultiGraph,
		SearchGraph:                   bus.SearchGraph,
		SubRepos:                      append([]SubRepoSnapshot(nil), bus.SubRepos...),
		ActiveSubRepo:                 bus.ActiveSubRepo,
		PendingSubRepos:               append([]string(nil), bus.PendingSubRepos...),
		MultiRepoInactivePreviewCount: bus.MultiRepoInactivePreviewCount,
		MultiRepoFocusDecision:        bus.MultiRepoFocusDecision,
		TypedDenials:                  bus.TypedDenials,
		Memory:                        bus.Memory,
		Ctx:                           bus.Ctx,
		EnvFacts:                      bus.EnvFacts,
		EnvRecommendSettings:          bus.EnvRecommendSettings,
		EvalDisableGitHistory:         bus.EvalDisableGitHistory,
		ReadDispatchPolicy:            bus.ReadDispatchPolicy,
		SourceInventoryFollowupDebt:   bus.SourceInventoryFollowupDebt,
	}
	return ac
}
