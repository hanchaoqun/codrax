package agent

import "github.com/hanchaoqun/codrax/internal/types"

// analyzerMentionedEntityCandidates extends the analyzer-authored entity
// shortlist with concrete high-confidence required-file hints before the
// existing deterministic RawRequest provenance check runs. The hints remain
// recommendations unless MentionedEntitiesFromRawRequest confirms their exact
// surface in the current request; this helper itself grants no scope authority.
func analyzerMentionedEntityCandidates(rm types.RequestModel) []string {
	out := append([]string(nil), rm.AnalyzerHints.PrimaryEntities...)
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.8 {
			continue
		}
		path := types.CanonicalRequiredFileHintPath(hint.Path, "")
		if path == "" || !types.HasCodeOrConfigPathSuffix(path) {
			continue
		}
		out = append(out, path)
	}
	return out
}
