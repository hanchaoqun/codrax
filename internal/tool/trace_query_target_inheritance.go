package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Query selection only: equivalent numeric targets must not become ambiguous
// when a later cursor omits the name. Source generation/incarnation and causal
// ownership remain the native query's responsibility. Never rewrite the roster.
func traceQuerySingleRuntimeTargetFromRequestModel(rm *types.RequestModel) (traceQueryRequestTarget, bool) {
	if rm == nil {
		return traceQueryRequestTarget{}, false
	}
	type selection struct {
		target       traceQueryRequestTarget
		name         string
		nameConflict bool
	}
	targets := map[string]selection{}
	for _, rt := range rm.RuntimeTargets {
		target := traceQueryRequestTarget{PID: rt.PID, Thread: strings.TrimSpace(rt.Thread), Source: strings.TrimSpace(rt.Source), TargetScope: traceSupplementRuntimeTargetScope(rt.Kind)}
		if !traceQueryTypedRuntimeTargetSafe(target) {
			continue
		}
		name, conflict := target.Thread, false
		if pid, parsedName, ok := tracequery.ParseThreadSelectorIdentity(name); ok {
			switch {
			case target.PID <= 0:
				target.PID, target.Thread, name = pid, parsedName, parsedName
			case target.PID == pid:
				// Preserve accepted display spelling while comparing canonical names.
				name = parsedName
			default:
				// A conflicting decorated spelling cannot change the numeric target.
				name, target.Thread, conflict = "", "", true
			}
		} else if target.PID > 0 && threadidentity.Parse(name).HasPID {
			// Explicit matching ID carriers can qualify a literal numeric name
			// (Thread-10 [56284]); do not treat its name suffix as another TID.
			if _, parsedName, ok := threadidentity.QuerySelector(target.PID, name); ok {
				name = parsedName
			} else {
				name, target.Thread, conflict = "", "", true
			}
		}
		if target.Source == "" {
			target.Source = "request_model_runtime_targets"
		}
		kind := types.NormalizeRuntimeTargetKind(rt.Kind)
		key := fmt.Sprintf("%s:name:%s", kind, strings.ToLower(name))
		if target.PID > 0 {
			key = fmt.Sprintf("%s:pid:%d", kind, target.PID)
		}
		prior, exists := targets[key]
		if !exists {
			targets[key] = selection{target, name, conflict}
			continue
		}
		prior.nameConflict = prior.nameConflict || conflict || prior.name != "" && name != "" && !strings.EqualFold(prior.name, name)
		if prior.name == "" && name != "" {
			prior.name, prior.target.Thread = name, target.Thread
		}
		// The exploration cursor does not replace request provenance, even when
		// it is the later equivalent spelling. Names still require agreement.
		if types.RuntimeTargetIsExplorationCursorSource(prior.target.Source) && !types.RuntimeTargetIsExplorationCursorSource(target.Source) {
			prior.target.Source = target.Source
			if name != "" {
				prior.target.Thread = target.Thread
			}
		}
		if prior.nameConflict {
			prior.target.Thread = ""
		}
		targets[key] = prior
	}
	if len(targets) != 1 {
		return traceQueryRequestTarget{}, false
	}
	for _, target := range targets {
		return target.target, true
	}
	return traceQueryRequestTarget{}, false
}
