package types

import "strings"

const (
	SourceInventoryProvenanceRepoLensToolQuery = "repo_lens:tool_query"
	SourceInventoryProvenancePreExplore        = "pre_explore_typed_request"
	SourceInventoryProvenanceStageAnalyze      = "repo_lens:stage:analyze"
	SourceInventoryProvenanceStageExplore      = "repo_lens:stage:explore"
)

func sourceInventoryExecutionProvenance(provenance []string) (executable, hasToolQuery, hasStage, hasAnalyzeStage bool) {
	for _, raw := range provenance {
		p := strings.TrimSpace(raw)
		switch {
		case p == SourceInventoryProvenancePreExplore || p == SourceInventoryProvenanceStageExplore:
			executable = true
		case p == SourceInventoryProvenanceRepoLensToolQuery:
			hasToolQuery = true
		case p == SourceInventoryProvenanceStageAnalyze:
			hasStage, hasAnalyzeStage = true, true
		case strings.HasPrefix(p, "repo_lens:stage:"):
			hasStage = true
		}
	}
	return executable, hasToolQuery, hasStage, hasAnalyzeStage
}
