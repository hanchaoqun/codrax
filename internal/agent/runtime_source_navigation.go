package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type runtimeSourceNavigationPhase string

const (
	runtimeSourceNavigationPhaseNone                  runtimeSourceNavigationPhase = ""
	runtimeSourceNavigationPhaseRuntimeProbeFirst     runtimeSourceNavigationPhase = "runtime_probe_first"
	runtimeSourceNavigationPhaseRuntimeProbeSatisfied runtimeSourceNavigationPhase = "runtime_probe_satisfied"
)

type runtimeSourceNavigationPhaseView struct {
	Phase                             runtimeSourceNavigationPhase
	TraceQueryAvailable               bool
	RuntimeTraceCarrier               bool
	TraceQueryAttempted               bool
	RuntimeObservationAvailable       bool
	CurrentSourceLane                 types.CurrentSourceLaneDecision
	CurrentSourceRequirement          types.RuntimeSourceRequirementPrecision
	CurrentSourceSatisfied            bool
	CurrentSourceHardBlock            bool
	SourceOwnerRequired               bool
	RuntimeProbePreferredBeforeSource bool
	RuntimeProbeHardRequired          bool
}

func runtimeSourceNavigationPhaseForExplorer(ctx *types.AgentContext, traceQueryInCurrentSurface bool) runtimeSourceNavigationPhaseView {
	view := runtimeSourceNavigationPhaseView{}
	rm := requestModelFromContext(ctx)
	if rm != nil {
		view.CurrentSourceLane = rm.CurrentSourceLaneDecision()
		authority := runtimeSourceAnswerAuthorityForExplorer(ctx)
		view.CurrentSourceRequirement = authority.CurrentSourceRequirement
		view.CurrentSourceSatisfied = authority.CurrentSourceSatisfied
		view.CurrentSourceHardBlock = authority.CanHardBlockCompletion
		if authority.Active {
			view.SourceOwnerRequired = authority.KeepsCurrentSourceLaneLoadBearing()
		} else {
			view.SourceOwnerRequired = types.RuntimeSourceRequestCurrentSourceRequirementPrecision(
				rm,
				ctx.TurnRouteHint,
			) == types.RuntimeSourceRequirementPrecise
		}
	}
	view.TraceQueryAvailable = traceQueryInCurrentSurface && traceQueryToolAvailable(ctx)
	view.TraceQueryAttempted = explorerTraceQueryAlreadyAttempted(ctx)
	view.RuntimeObservationAvailable = explorerTraceQueryRuntimeEvidenceAvailable(ctx)
	view.RuntimeTraceCarrier = explorerHasRuntimeTraceArtifact(ctx, rm)
	if !view.TraceQueryAvailable || !view.RuntimeTraceCarrier {
		return view
	}
	if view.TraceQueryAttempted || view.RuntimeObservationAvailable {
		view.Phase = runtimeSourceNavigationPhaseRuntimeProbeSatisfied
		return view
	}
	view.Phase = runtimeSourceNavigationPhaseRuntimeProbeFirst
	view.RuntimeProbePreferredBeforeSource = true
	view.RuntimeProbeHardRequired = !view.SourceOwnerRequired
	return view
}

func runtimeSourceTraceProbePromptPreferred(ctx *types.AgentContext) bool {
	if !explorerHasTraceQueryRuntimeTraceCarrier(ctx) {
		return false
	}
	view := runtimeSourceNavigationPhaseForExplorer(ctx, true)
	return view.RuntimeProbePreferredBeforeSource
}

func renderRuntimeSourceNavigationPhasePrompt(ctx *types.AgentContext) string {
	view := runtimeSourceNavigationPhaseForExplorer(ctx, true)
	if !view.RuntimeProbePreferredBeforeSource {
		return ""
	}
	var b strings.Builder
	b.WriteString("Typed navigation phase: ")
	requirement := strings.TrimSpace(string(view.CurrentSourceRequirement))
	if requirement == "" {
		requirement = "none"
	}
	fmt.Fprintf(&b, "`phase=%s`, `current_source_lane=%s`, `current_source_requirement=%s`.\n", view.Phase, view.CurrentSourceLane, requirement)
	b.WriteString("- First run one bounded `trace_query` runtime probe for the attached trace. Do not start with `repo_map`, `grep`, `list_files`, or `read_file` while this step is pending.\n")
	if view.SourceOwnerRequired {
		b.WriteString("- Then collect focused current-source evidence after the runtime probe; use source-owner tools only for the unresolved source mechanism, not for broad repo discovery.\n\n")
	} else {
		b.WriteString("- Then use source-owner tools only if the trace result leaves a precise source question unresolved or reports unsupported/incomplete coverage; soft current-source gaps should converge through bounded follow-up or a caveat, not broad repo discovery.\n\n")
	}
	return b.String()
}

// renderCurrentSourceMechanismCoveragePrompt teaches a language- and
// implementation-neutral evidence ladder for mechanism/flow explanations.
// It is deliberately soft guidance: it neither scans answer prose nor blocks
// completion. Its job is to prevent a parser field read, a sibling event
// family, or an async path from being overclaimed as proof of a stateful
// correlator and its final projection.
func renderCurrentSourceMechanismCoveragePrompt(ctx *types.AgentContext) string {
	rm := requestModelFromContext(ctx)
	if rm == nil || rm.CurrentSourceExplanationProfile == nil || !rm.CurrentSourceExplanationProfile.Active() {
		return ""
	}
	requested := false
	for _, mode := range rm.CurrentSourceExplanationProfile.Modes {
		if mode == types.CurrentSourceExplanationExplainCurrentMechanism ||
			mode == types.CurrentSourceExplanationTraceCurrentFlow {
			requested = true
			break
		}
	}
	if !requested {
		return ""
	}
	return "### Current-Source Mechanism Coverage Ladder (soft guidance)\n\n" +
		"When explaining a current implementation or flow, keep these source-owned layers separate and collect focused evidence for every layer the conclusion claims:\n" +
		"- **Decode / normalize:** how raw records, fields, identities, and direction markers are parsed. This layer alone does not prove how records are paired or accumulated.\n" +
		"- **Stateful correlation / lifecycle:** the exact correlation key and source identity; direction; stack, queue, nesting, or adjacency semantics; lifecycle/reset boundaries; malformed/order handling; and fail-open/fail-closed behavior. Do not borrow these semantics from a sibling event family, another language adapter, or an async path.\n" +
		"- **Consumer / projection:** how matched state becomes durations, spans, graph relations, emitted rows, or user-visible output, including filtering and completeness boundaries.\n" +
		"Report which layers current-source evidence actually covers and state a boundary for unvisited layers. This is evidence guidance, not a completion gate and not permission to invent missing mechanism details.\n\n"
}
