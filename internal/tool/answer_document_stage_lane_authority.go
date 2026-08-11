package tool

import (
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// diagramVerifiedReadModeStagePrecedence is the narrow bridge from the shared
// checkout provider into diagram validation.  Only source-code flow views in
// the current read mode may consume it. Runtime/root-cause Trace keeps its
// separate report-local causal/temporal authority.
func diagramVerifiedReadModeStagePrecedence(ctx *types.BusContext, view *types.AnswerSemanticView) []stageauthority.PrecedenceRelation {
	if ctx == nil || ctx.AnalysisIR == nil || view == nil || view.Family == types.QFRootCauseTrace ||
		view.RelationAxis != types.AxisFlow ||
		(ctx.Mode != "" && ctx.Mode != types.ModeRead) {
		return nil
	}
	authority, ok := stageauthority.LoadReadMode(ctx.RepoRoot)
	if !ok || !stageauthority.MatchesRequiredMainStageParticipantSlate(ctx.AnalysisIR.RequestModel, authority.Main) {
		return nil
	}
	return authority.Precedence
}
