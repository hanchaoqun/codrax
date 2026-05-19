package agent

import (
	"path"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// projectEntityProvenance populates typed metadata parallel to the
// analyzer's untyped entity lanes. COPY semantics: it never removes or
// rewrites AnalyzerHints.Entities / SubTopic.Entities, and no current
// consumer reads this metadata. Batch 2 is telemetry-only by design.
func projectEntityProvenance(ctx *types.AgentContext, rm *types.RequestModel) {
	if rm == nil {
		return
	}
	seenBlob := ""
	if ctx != nil && ctx.Mutable != nil {
		seenBlob = ctx.Mutable.PrescanSummaryBlob()
	}
	rm.AnalyzerHints.EntityProvenance = buildEntityProvenance(
		ctx,
		rm.AnalyzerHints.Entities,
		types.EntityOriginAnalyzerEntity,
		seenBlob,
	)
	for i := range rm.SubTopics {
		rm.SubTopics[i].EntityProvenance = buildEntityProvenance(
			ctx,
			rm.SubTopics[i].Entities,
			types.EntityOriginSubTopicEntity,
			seenBlob,
		)
	}
	logEntityProvenanceSummary(rm.AnalyzerHints.EntityProvenance)
}

func buildEntityProvenance(
	ctx *types.AgentContext,
	entities []string,
	origin types.EntityOrigin,
	seenBlob string,
) []types.EntityProvenance {
	if len(entities) == 0 {
		return nil
	}
	out := make([]types.EntityProvenance, 0, len(entities))
	seen := make(map[string]bool, len(entities))
	for _, raw := range entities {
		surface := strings.TrimSpace(raw)
		if surface == "" {
			continue
		}
		key := normalizer.NormalizeCodeKey(surface)
		if key == "" {
			key = strings.ToLower(surface)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, classifyEntityProvenance(ctx, surface, origin, seenBlob))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func classifyEntityProvenance(
	ctx *types.AgentContext,
	surface string,
	origin types.EntityOrigin,
	seenBlob string,
) types.EntityProvenance {
	prov := types.EntityProvenance{
		Surface:    surface,
		Origin:     origin,
		Resolution: types.EntityResolutionInferredConcept,
		NoiseScore: 0.7,
	}
	switch {
	case entityMatchesActiveScope(ctx, surface):
		prov.Resolution = types.EntityResolutionScope
		prov.Resolved = true
		prov.NoiseScore = 0
		prov.UseForSearch = true
		prov.UseForShape = true
	case entityFileExists(ctx, surface):
		prov.Resolution = types.EntityResolutionFile
		prov.Resolved = true
		prov.NoiseScore = 0
		prov.UseForSearch = true
		prov.UseForShape = true
	case entitySymbolExists(ctx, surface):
		prov.Resolution = types.EntityResolutionSymbol
		prov.Resolved = true
		prov.NoiseScore = 0
		prov.UseForSearch = true
		prov.UseForShape = true
	case entitySeenInPrescan(seenBlob, surface):
		prov.Resolution = types.EntityResolutionPrescanAnchor
		prov.NoiseScore = 0.2
		prov.UseForSearch = true
	default:
		prov.UseForSearch = false
		prov.UseForShape = false
	}
	return prov
}

func entityMatchesActiveScope(ctx *types.AgentContext, surface string) bool {
	canon := normalizer.NormalizeCodeKey(strings.TrimSpace(surface))
	if canon == "" {
		return false
	}
	_, ok := activeScopeCanonMap(ctx)[canon]
	return ok
}

func entitySymbolExists(ctx *types.AgentContext, surface string) bool {
	oracle := analyzerOracleFromCtx(ctx, entityGraphFromCtx(ctx))
	if oracle == nil {
		return false
	}
	ok, _ := oracle.SymbolExistsFlat(surface)
	return ok
}

func entityFileExists(ctx *types.AgentContext, surface string) bool {
	canon := canonicalEntityPath(surface)
	if canon == "" {
		return false
	}
	if mg := repomap.MultiGraphFromAgentContext(ctx); mg != nil {
		found := false
		mg.IterateFileIndex(func(internalRel string, _ *rmtypes.FileInfo, _ *topology.SubRepo) bool {
			if canonicalEntityPath(internalRel) == canon {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	if graph := entityGraphFromCtx(ctx); graph != nil && graph.FileIndex != nil {
		_, ok := graph.FileIndex[canon]
		return ok
	}
	return false
}

func entitySeenInPrescan(seenBlob, surface string) bool {
	seenBlob = strings.TrimSpace(seenBlob)
	surface = strings.TrimSpace(surface)
	return seenBlob != "" && surface != "" && strings.Contains(seenBlob, strings.ToLower(surface))
}

func entityGraphFromCtx(ctx *types.AgentContext) *repomap.Graph {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repomap.Graph)
	return graph
}

func canonicalEntityPath(surface string) string {
	surface = strings.TrimSpace(strings.ReplaceAll(surface, "\\", "/"))
	if surface == "" {
		return ""
	}
	for strings.HasPrefix(surface, "./") {
		surface = strings.TrimPrefix(surface, "./")
	}
	surface = strings.TrimPrefix(surface, "/")
	cleaned := path.Clean(surface)
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return ""
	}
	return cleaned
}

func logEntityProvenanceSummary(provenance []types.EntityProvenance) {
	if len(provenance) == 0 {
		return
	}
	counts := map[types.EntityResolution]int{}
	search, shape := 0, 0
	totalNoise := 0.0
	for _, p := range provenance {
		counts[p.Resolution]++
		if p.UseForSearch {
			search++
		}
		if p.UseForShape {
			shape++
		}
		totalNoise += p.NoiseScore
	}
	logging.Info(
		"[analyzer] entity provenance summary: total=%d scope=%d symbol=%d file=%d prescan_anchor=%d inferred_concept=%d use_for_search=%d use_for_shape=%d mean_noise=%.2f",
		len(provenance),
		counts[types.EntityResolutionScope],
		counts[types.EntityResolutionSymbol],
		counts[types.EntityResolutionFile],
		counts[types.EntityResolutionPrescanAnchor],
		counts[types.EntityResolutionInferredConcept],
		search,
		shape,
		totalNoise/float64(len(provenance)),
	)
}
