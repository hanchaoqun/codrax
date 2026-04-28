package recommend

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/hanchaoqun/codrax/internal/env/cache"
	"github.com/hanchaoqun/codrax/internal/types"
)

// recommenderCache is the process-wide handle into the disk
// cache. Lazily initialised on first call so test callers that
// never use real cache don't pay the file-IO cost.
var (
	cacheOnce sync.Once
	cacheRef  *cache.Cache
	cacheTTL  = 90
)

// SetCacheTTL is called from cmd/root.go after yaml resolves
// EnvRecommendSettings.CacheTTLDays. Affects subsequent Open.
func SetCacheTTL(days int) { cacheTTL = days }

func ensureCache() *cache.Cache {
	cacheOnce.Do(func() {
		c, err := cache.Open(cacheTTL)
		if err != nil {
			// Non-fatal: cache disabled, every lookup misses.
			return
		}
		cacheRef = c
	})
	return cacheRef
}

// recommendFromCache is Stage 2 — looks up the deterministic key
// for (DiagKind + Tool + OSFamily) and returns previously
// stored Recommendations.
func recommendFromCache(d types.Diagnosis, env *types.EnvFacts) []types.Recommendation {
	c := ensureCache()
	if c == nil {
		return nil
	}
	key := cacheKey(d, env)
	raw, ok := c.Get(key)
	if !ok {
		return nil
	}
	var recs []types.Recommendation
	if err := json.Unmarshal(raw, &recs); err != nil {
		return nil
	}
	return recs
}

// storeInCache writes a fresh recommendation list to the cache so
// the next identical lookup hits Stage 2.
func storeInCache(d types.Diagnosis, env *types.EnvFacts, recs []types.Recommendation, source string) {
	c := ensureCache()
	if c == nil {
		return
	}
	data, err := json.Marshal(recs)
	if err != nil {
		return
	}
	_ = c.Set(cacheKey(d, env), data, source)
}

// cacheKey builds the deterministic key from a Diagnosis. Tool +
// runner + OSFamily are the load-bearing inputs; the stderr
// excerpt is normalised (digits / paths stripped) so two
// occurrences of the same kind of failure share the same cache.
func cacheKey(d types.Diagnosis, env *types.EnvFacts) string {
	osFamily := ""
	if env != nil {
		osFamily = env.OSFamily
	}
	parts := string(d.Kind) + "|" + d.Runner + "|" + d.Tool + "|" + osFamily
	h := sha256.Sum256([]byte(parts))
	return "rec:" + hex.EncodeToString(h[:8])
}
