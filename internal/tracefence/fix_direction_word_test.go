package tracefence_test

// fix_direction_word_test.go — FREQDIR-1 返工 P3-3 (双复核, 2026-07-19): the
// registry↔word-table sync pin. tracefence.FixDirectionWord (Table ⑦) is THE
// single display word source for the fix-direction closed set; the closed set
// itself is owned by the tracequery causal-token registry
// (CausalTokenFixDirectionFor). A new direction admitted on the registry side
// without a word-table row would publish rows the display and the LLM-facing
// board feed silently drop (ok=false fail-open) — this pin turns that silent
// divergence into a red test. Test-only import: tracefence is a leaf package
// (tracequery does not import it), so the production import graph stays
// acyclic.

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// TestFixDirectionWordCoversRegistryClosedSet — every fix direction the
// registry can stamp on any token of the causal-token universe must resolve
// BOTH word faces in Table ⑦ (registry 加第 7 方向而词表缺 → 红); the
// unresolved sentinel and unknown tokens must resolve NOTHING (fail-open,
// absence never guesses).
func TestFixDirectionWordCoversRegistryClosedSet(t *testing.T) {
	seen := map[string]bool{}
	for _, token := range tracequery.CausalTokenUniverse() {
		direction := tracequery.CausalTokenFixDirectionFor(token)
		if direction == tracequery.CausalFixDirectionUnresolved {
			continue
		}
		seen[string(direction)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("registry sweep looks broken: only %d resolved direction(s) found", len(seen))
	}
	for direction := range seen {
		for _, zh := range []bool{true, false} {
			word, ok := tracefence.FixDirectionWord(direction, zh)
			if !ok || word == "" {
				t.Fatalf("registry direction %q (zh=%v) has no word-table row — Table ⑦ must cover the registry closed set", direction, zh)
			}
		}
	}
	// Fail-open arms: the unresolved sentinel and an unregistered token
	// never resolve a word.
	for _, bogus := range []string{string(tracequery.CausalFixDirectionUnresolved), "", "bogus_direction"} {
		if _, ok := tracefence.FixDirectionWord(bogus, true); ok {
			t.Fatalf("token %q must not resolve a display word", bogus)
		}
	}
}
