package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Called only within an existing reject branch for a required effect verdict
// without a required causal role. A root classifier detects a contradiction;
// it never supplies the missing role or grants causal authority. The repair
// must not name a unique finite target that the unchanged classifiers would
// make the next consistency check reject. No request/model prose is consumed.
func runtimeQuestionConflictingClassifierRepairHint(profile *types.RuntimeQuestionProfile, intent types.Intent, scenario types.Scenario) string {
	if intent != types.IntentRootCause && scenario != types.ScenarioRootCause {
		return ""
	}
	return fmt.Sprintf("runtime_question_profile.scope=%s has a required target_effect_verdict but no required causal role and a conflicting root-cause classifier (intent=%s, scenario=%s); no unique finite repair target is established. target_effect_verdict alone cannot authorize cause discovery or full Trace causal projection. Reclassify the current request into one complete coherent tuple. Full causal request: use causal_diagnosis, omit fact_families, and add the independently requested required causal_attribution or causal_contributor_set; preserve the independent required target_effect_verdict instead of relabeling it merely to pass validation. Keep diagnostic flags consistent with any retained root-cause classifier; those flags do not supply the missing causal role. Finite-only request: only if the current request asks for no independent cause discovery, use bounded_effect_verdict with all requested observed fact_families, exactly one required target_effect_verdict, no required causal role, non-root-cause intent and scenario, and all diagnostic predicate/profile flags false. Preserve every independent requested dimension, runtime_work_relation_requested=%t, and frame_causality_requested=%t during this structural repair unless the model deliberately reclassifies the request itself. Re-emit the next COMPLETE model-owned object; the system neither chooses an alternative, adds a role, nor accepts or rewrites this rejected object", profile.Scope, intent, scenario, profile.RuntimeWorkRelationRequested, profile.FrameCausalityRequested)
}
