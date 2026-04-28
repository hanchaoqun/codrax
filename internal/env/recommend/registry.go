// Package recommend implements Layer 3 of env_recommend — the
// Recommender registry that turns a Diagnosis + EnvFacts into a
// sorted list of install candidates the Renderer surfaces to the
// user.
//
// Pipeline:
//   1. Stage 1 — deterministic per-runner table (this file dispatches)
//   2. Stage 2 — disk cache (env_cache.json, see internal/env/cache)
//   3. Stage 3 — LLM fallback (see llm_*.go)
//   4. Stage 4 — DocsLink-only fallback
//
// New languages add a Recommender via Register().
package recommend

import (
	"sort"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Recommender is the per-runner policy interface. Each impl owns
// a small switch on Diagnosis.Kind and returns the ordered
// candidate list for that diagnosis.
type Recommender interface {
	Recommend(d types.Diagnosis, env *types.EnvFacts, settings types.EnvRecommendSettings) []types.Recommendation
}

var registry = map[string]Recommender{}

// Register associates a runner name with its policy. Called from
// each language's init(). System-level recommendations (toolchain
// missing where runner is empty) register under name "" — see
// system.go.
func Register(name string, r Recommender) {
	registry[name] = r
}

// Recommend is the public entry point. Tries Stage 1 → 2 → 3 → 4
// in order and returns the merged sorted list. Empty result means
// "no actionable recommendation" — caller surfaces a generic
// docs link.
func Recommend(d types.Diagnosis, env *types.EnvFacts, settings types.EnvRecommendSettings) []types.Recommendation {
	if !settings.Enabled {
		return nil
	}
	var recs []types.Recommendation

	// Stage 1: deterministic table.
	if r, ok := registry[d.Runner]; ok {
		recs = append(recs, r.Recommend(d, env, settings)...)
	}
	// Always try the system-level recommender too — it covers
	// toolchain_missing / git_state cases that can apply across
	// runners.
	if r, ok := registry[""]; ok {
		recs = append(recs, r.Recommend(d, env, settings)...)
	}

	// Stage 2: cache (only if Stage 1 returned nothing).
	if len(recs) == 0 {
		recs = append(recs, recommendFromCache(d, env)...)
	}

	// Stage 3: LLM fallback (gated by settings.LLMEnabled).
	if len(recs) == 0 && settings.LLMEnabled {
		recs = append(recs, recommendFromLLM(d, env, settings)...)
	}

	// Stage 4: filter out global-install candidates if forbidden.
	if !settings.RecommendGlobalInstall {
		recs = filterGlobalInstall(recs)
	}

	// Network-aware demotion: if probed unreachable, push
	// NeedsNetwork=true candidates down.
	if env != nil && env.Network.ProbedAt.Year() > 0 && !env.Network.Reachable {
		demoteNetworkCandidates(recs)
	}

	// Sort by Priority (lower wins).
	sort.SliceStable(recs, func(i, j int) bool {
		return recs[i].Priority < recs[j].Priority
	})

	// Cap returned candidates so the renderer doesn't print a wall
	// of options. 3 is the sweet spot.
	if len(recs) > 3 {
		recs = recs[:3]
	}
	return recs
}

func filterGlobalInstall(recs []types.Recommendation) []types.Recommendation {
	out := recs[:0]
	for _, r := range recs {
		if r.Strategy == types.StrategyGlobalInstall {
			continue
		}
		out = append(out, r)
	}
	return out
}

func demoteNetworkCandidates(recs []types.Recommendation) {
	for i := range recs {
		if recs[i].NeedsNetwork {
			recs[i].Priority += 100
			recs[i].Caveats = append(recs[i].Caveats,
				"network appears unreachable — this command may hang or fail")
		}
	}
}
