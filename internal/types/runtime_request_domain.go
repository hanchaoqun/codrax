package types

import (
	"context"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// hasRuntimeRequestDomain describes the material family of an accepted request,
// not material availability, query success, or evidence sufficiency. Named
// captures need not have been attached or prepared when emit_analysis accepts
// their typed scope. Absent/unspecified declarations retain the legacy route.
func (rm RequestModel) hasRuntimeRequestDomain() bool {
	if rm.runtimeMaterialDomain || rm.RuntimeArtifactScopeProfile.Active() {
		return true
	}
	if rm.RuntimeQuestionProfile == nil {
		return false
	}
	switch rm.RuntimeQuestionProfile.Scope {
	case RuntimeQuestionScopeBoundedFactSet, RuntimeQuestionScopeBoundedEffectVerdict,
		RuntimeQuestionScopeCausalDiagnosis, RuntimeQuestionScopeRelationAnalysis, RuntimeQuestionScopeSystemOverview:
		return true
	}
	return false
}

// preparedRuntimeMaterialDomain reuses private preparation receipts without
// preparing files or publishing observations. Revalidate each generation;
// restoring an old request or a serialized preview cannot recreate this hint.
func preparedRuntimeMaterialDomain(ctx context.Context, preparer TraceInputPreparer, attached *attachment.TraceMaterial) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	valid := func(material *attachment.TraceMaterial) bool {
		return material != nil && material.Validate(ctx, material.Preview()) == nil
	}
	if valid(attached) {
		return true
	}
	if preparer != nil {
		for _, material := range preparer.PreparedMaterials() {
			if valid(material) {
				return true
			}
		}
	}
	return false
}
