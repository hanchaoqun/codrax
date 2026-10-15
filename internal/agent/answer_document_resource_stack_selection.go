package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Resource members are not a single Subject. Only this finite display lane
// projects their decoded same-capture owners; the audit/causal ledger is never
// rewritten and a matching member cannot admit the rest of an aggregate.
func resourceStackNamedDisplayRequest(ctx *types.AgentContext) *types.RequestModel {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := &ctx.AnalysisIR.RequestModel
	p := rm.RuntimeQuestionProfile
	if p == nil || !p.SuppressesRootCauseRankingPrompt() || !p.CarriesBoundedFactFamilies() || !answerDocHasUserRuntimeTarget(rm) {
		return nil
	}
	return rm
}

func renderAnswerDocScopedResourceStacks(ctx *types.AgentContext, accepted, principal types.ObservationLedger) resourceStackPresentation {
	if resourceStackNamedDisplayRequest(ctx) == nil {
		return renderAnswerDocResourceStackPresentation(ctx, principal)
	}
	// Keep the original scope index and all conflicting copies for the existing
	// source/window/conflict gates, but never send other filtered families here.
	candidates := accepted
	candidates.Records = nil
	for _, r := range accepted.Records {
		if _, ok := tool.DecodeTraceResourceStack(r); ok {
			candidates.Records = append(candidates.Records, r)
		}
	}
	return renderAnswerDocResourceStackPresentation(ctx, candidates)
}

func resourceStackDisplayMembers(p tracequery.ResourceStackResult, rm *types.RequestModel) ([]int, bool) {
	request := &types.RequestModel{}
	for _, target := range rm.RuntimeTargets {
		if !types.RuntimeTargetIsExplorationCursorSource(target.Source) && (target.Kind == types.RuntimeTargetKindThread || target.Kind == types.RuntimeTargetKindProcess) {
			request.RuntimeTargets = append(request.RuntimeTargets, target)
		}
	}
	if len(request.RuntimeTargets) == 0 {
		return nil, false
	}
	selected := make([]int, 0, len(p.Events))
	for i, e := range p.Events {
		known := true // DecodeTraceResourceStack has validated positive native owners.
		row := types.TraceEventSearchInventoryRow{EmitterTID: e.Source.TID, EmitterTGID: e.Source.PID,
			EmitterTIDKnown: &known, EmitterTGIDKnown: &known, Comm: e.Source.Thread}
		if types.TraceEventRowMatchesEmitterTarget(row, request) {
			selected = append(selected, i)
		}
	}
	// An executed exact selector can retain empty/partial/unavailable query
	// boundaries. It does not prove that a thread exists or a capture is complete.
	return selected, len(selected) > 0 || resourceStackQueryMatchesTarget(p, request)
}

func resourceStackQueryMatchesTarget(p tracequery.ResourceStackResult, rm *types.RequestModel) bool {
	for _, target := range rm.RuntimeTargets {
		if target.Kind == types.RuntimeTargetKindProcess {
			if p.TargetScope == tracequery.TargetScopeProcess && target.PID > 0 && target.PID <= types.RuntimeTargetMaxPID && p.TargetPID == target.PID {
				return true
			}
			continue
		}
		if target.Kind != types.RuntimeTargetKindThread || p.TargetScope != tracequery.TargetScopeThread {
			continue
		}
		pid, name, valid := threadidentity.QuerySelector(target.PID, target.Thread)
		if !valid {
			continue
		}
		if pid > 0 {
			if p.TargetPID == pid {
				return true
			}
			continue
		}
		if p.TargetPID == 0 && name != "" && strings.EqualFold(name, p.TargetThread) {
			return true
		}
	}
	return false
}

type resourceStackPresentation struct {
	text string
	// Actual budget-admitted query records, used only for the source-authority
	// census. They never re-enter generic compact rows or publish raw RichNotes.
	published []types.ObservationRecord
}

func (p resourceStackPresentation) authorityText(ctx *types.AgentContext, principal types.ObservationLedger) string {
	if len(p.published) == 0 {
		return ""
	}
	return renderAnswerDocRuntimeSourceAuthority(ctx, p.authorityLedger(principal))
}

func (p resourceStackPresentation) authorityLedger(principal types.ObservationLedger) types.ObservationLedger {
	if len(p.published) == 0 {
		return principal
	}
	out := principal
	out.Records = append([]types.ObservationRecord(nil), principal.Records...)
	for _, r := range p.published {
		present := false
		for _, old := range out.Records {
			if old.ID == r.ID && old.SourceRef == r.SourceRef {
				present = true
				break
			}
		}
		if !present {
			out.Records = append(out.Records, r)
		}
	}
	return out
}
