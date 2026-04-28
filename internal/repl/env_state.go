package repl

import (
	"github.com/hanchaoqun/codrax/internal/env/cache"
)

// envCacheRef opens the disk cache lazily for /env cache subcommands.
// Returns nil on failure (filesystem permissions, no $HOME).
// Operates only when env_recommend is enabled.
func envCacheRef() *cache.Cache {
	c, err := cache.Open(0) // 0 → use default TTL
	if err != nil {
		return nil
	}
	return c
}
