package agent

import (
	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Keep the outcome of an exact selected query even when no sample can prove a
// subject. This is a query boundary, not observed thread ownership: it neither
// rewrites Subject nor supplies a causal/principal fact or aggregate identity.
// The native decoder binds the actual source, window, selector and outcome.
func answerDocSamplingQueryBoundaryMatchesTarget(record types.ObservationRecord, rm *types.RequestModel) bool {
	if rm == nil {
		return false
	}
	tree, ok := tool.DecodeTracePerfSamplingTree(record)
	if !ok || tree.TargetScope != "thread" || tree.SampleCount != 0 || tree.CohortCount != 0 ||
		(tree.Status != "unavailable" && tree.Status != "available") {
		return false
	}
	pid, name, valid := tracequery.NormalizePerfSamplingSelector(tree.TargetPID, tree.TargetThread)
	if !valid || pid != tree.TargetPID || name != tree.TargetThread || (pid == 0 && name == "") {
		return false
	}
	for _, target := range rm.RuntimeTargets {
		if target.Kind == types.RuntimeTargetKindProcess || types.RuntimeTargetIsExplorationCursorSource(target.Source) {
			continue
		}
		wantPID, wantName, valid := tracequery.NormalizePerfSamplingSelector(target.PID, target.Thread)
		if !valid {
			continue
		}
		if wantPID > 0 {
			if pid == wantPID {
				return true // same query selector, not a claim that this TID exists
			}
			continue
		}
		if pid == 0 && wantName != "" && name == threadidentity.CleanName(wantName) {
			return true // exact name query only; no fuzzy/discovered identity
		}
	}
	return false
}
