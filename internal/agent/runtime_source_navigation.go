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
			view.SourceOwnerRequired = authority.CanHardBlockCompletion || authority.CurrentSourceSatisfied
		} else {
			view.SourceOwnerRequired = view.CurrentSourceLane.RequiresCurrentSource()
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
