// Package recommend is Layer 3 of env_recommend — turning a
// Diagnosis + EnvFacts into a sorted list of install candidates
// the Renderer surfaces to the user.
//
// Four-stage decision flow:
//
//   1. Stage 1 — system-stable deterministic registry. Only
//      systemRecommender (registered under name "") runs here —
//      it handles git_state + system_lib_missing because those
//      install commands are universal (`git init` is `git init`,
//      `libssl-dev` is `libssl-dev`) and don't drift across
//      distros. Per-runner DiagKinds skip this stage.
//
//   2. Stage 2 — disk cache. Recommendations keyed by
//      (DiagKind, Runner, OSFamily) so every machine pays the
//      LLM round-trip at most once per diagnosis class. TTL via
//      `env_cache_ttl_days` (default 90).
//
//   3. Stage 3 — LLM dispatch (`agents.env_recommender`, cheap
//      model). Synthesises an env-aware command line; output
//      passes `emit_env_recommendation` schema validation; result
//      is written back to Stage 2. Disabled by
//      `env_recommend_llm_enabled = false`.
//
//   4. Stage 4 — DocsLink fallback. Surfaces the runner's
//      canonical docs URL from `runnerDocsURL` (see docslink.go).
//      Unknown runners drop through to the Renderer's generic
//      "no actionable command" footer.
package recommend

import (
	"sort"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Recommender is the deterministic-stage policy interface. The
// only registered impl is systemRecommender ("" name); per-runner
// DiagKinds are served by the LLM stage. Future stable add-ons
// (e.g. a universal runner with non-trivial OS branching) can plug
// in without redesigning the dispatcher.
type Recommender interface {
	Recommend(d types.Diagnosis, env *types.EnvFacts, settings types.EnvRecommendSettings) []types.Recommendation
}

var registry = map[string]Recommender{}

// Register associates a runner name with its policy. Called from
// recommend_system.go's init() under name "".
func Register(name string, r Recommender) {
	registry[name] = r
}

// Recommend is the public entry point. Walks Stage 1 → 2 → 3 → 4
// and returns the merged sorted list. Empty result means "no
// actionable recommendation" — the Renderer surfaces a "consult
// docs" footer in that case.
//
// Each call increments the counters in metrics.go; read via
// Snapshot(), reset via ResetMetrics().
func Recommend(d types.Diagnosis, env *types.EnvFacts, settings types.EnvRecommendSettings) []types.Recommendation {
	metrics.calls.Add(1)
	if !settings.Enabled {
		metrics.disabledCalls.Add(1)
		return nil
	}
	var recs []types.Recommendation

	// Stage 1: deterministic — runs only for the system-stable
	// dispatcher (git / system_lib). Per-runner DiagKinds skip
	// straight to Stage 2/3.
	if r, ok := registry[""]; ok {
		recs = append(recs, r.Recommend(d, env, settings)...)
	}
	if len(recs) > 0 {
		metrics.stage1Hits.Add(1)
	}

	// Stage 2: cache (only if Stage 1 returned nothing).
	if len(recs) == 0 {
		cached := recommendFromCache(d, env)
		if len(cached) > 0 {
			metrics.cacheHits.Add(1)
		}
		recs = append(recs, cached...)
	}

	// Stage 3: LLM fallback (gated by settings.LLMEnabled).
	if len(recs) == 0 && settings.LLMEnabled {
		recs = append(recs, recommendFromLLM(d, env, settings)...)
	}

	// Stage 4: DocsLink fallback when previous stages produced
	// nothing and the runner is in the canonical map.
	if len(recs) == 0 {
		dl := docsLinkFallback(d)
		if len(dl) > 0 {
			metrics.docsLinkFallbacks.Add(1)
		}
		recs = append(recs, dl...)
	}

	if len(recs) == 0 {
		metrics.emptyResults.Add(1)
	}

	// Filter global-install candidates when forbidden (R8 default).
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
