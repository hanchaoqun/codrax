package index

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// ParseAttempt is the structured result of a single parse pass.
// One attempt is produced per (file, tier) combination during the
// fallback chain; the "winner" is written into FileInfo.
//
// Tier semantics (canonical for ArkTS + Cangjie):
//
//	1 = primary grammar (TS for ArkTS, Go-native scanner for Cangjie)
//	2 = secondary path  (regex post-pass salvage)
//	3 = path-only       (no symbols, file still ranks)
//
// Languages with a single supported grammar (Go, Java, Python …)
// do not use this chain — parser.go's main switch handles them
// directly. The chain only exists where multiple parse strategies
// are meaningful.
type ParseAttempt struct {
	Tier      int
	Symbols   []types.Symbol
	Imports   []types.Import
	Relations []types.Relation
	Package   string
	Reason    string // first line of the failure cause (Tier > 1 only)
}

// TierDiscount returns the rank multiplier for a parse tier.
// Used by retrieve.rank.go to deprioritise lower-confidence parses
// so a Tier-3 file cannot outrank a Tier-1 sibling on identical
// keyword evidence (red line L-Fallback-2 — no anti-incentive).
//
// The constants are deliberately quoted in code rather than yaml
// because the rank pipeline is hot and the curve must not change
// per-deployment; if tuning is needed, change here and recompile.
func TierDiscount(tier int) float64 {
	switch tier {
	case 0, 1:
		return 1.0
	case 2:
		return 0.85
	case 3:
		return 0.6
	default: // 4+ "path-only" or unknown
		return 0.3
	}
}

// parseTreeSitterIfPossible tries to parse `source` with the
// tree-sitter language for `lang`. Returns (root, true) on success
// or (nil, false) if the grammar is not registered, parsing failed,
// or the root produced no children. Callers fall through to the
// regex/Go-native salvage path on a false return.
//
// Centralised so every language extractor that wants tree-sitter
// shares the same nil-and-error handling instead of each one
// re-implementing the dance.
func parseTreeSitterIfPossible(lang string, source []byte) (*sitter.Node, bool) {
	tsLang := types.GetSitterLanguage(lang)
	if tsLang == nil {
		return nil, false
	}
	parser := sitter.NewParser()
	parser.SetLanguage(tsLang)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil || tree == nil {
		return nil, false
	}
	root := tree.RootNode()
	if root == nil {
		return nil, false
	}
	return root, true
}

// recordFallback annotates a FileInfo with the tier + reason and
// emits the WARN log mandated by red line L-Fallback-1. Use this
// from every extractor that downgrades to a non-primary tier so
// the operator log stays consistent across languages.
func recordFallback(fi *types.FileInfo, fromTier, toTier int, reason string) {
	fi.ParseTier = toTier
	fi.FallbackReason = reason
	if reason == "" {
		reason = "no detail"
	}
	logging.Warning("repomap: %s %s tier %d→%d: %s",
		fi.Language, fi.RelPath, fromTier, toTier, reason)
}

// fallbackBannerThreshold is the per-language Tier-2 ratio above
// which the scanner emits the once-per-build "consider grammar
// update" banner. ArkTS uses 0.4 (TS grammar is well-aligned;
// 40%+ falling through is a real signal); Cangjie uses 0.5
// (the regex Tier 2 sees more action because the Go-native
// extractor is still maturing — relax the bar).
//
// Public test seam: a unit test reads these to assert no silent
// drift if a future change tightens the bar.
var fallbackBannerThreshold = map[string]float64{
	types.LangArkTS:   0.4,
	types.LangCangjie: 0.5,
}

// reportFallbackRatios is called once per ScanFiles run. It walks
// the produced FileInfos, computes per-language Tier-2 share, and
// emits the banner if any language exceeds its threshold.
//
// Idempotent: callers do not need to gate this on "first time"
// — it just emits at most one Warning per language per call.
func reportFallbackRatios(files []*types.FileInfo) {
	type counts struct{ total, t2 int }
	stats := map[string]*counts{}
	for _, fi := range files {
		switch fi.Language {
		case types.LangArkTS, types.LangCangjie:
		default:
			continue
		}
		c, ok := stats[fi.Language]
		if !ok {
			c = &counts{}
			stats[fi.Language] = c
		}
		c.total++
		if fi.ParseTier == 2 {
			c.t2++
		}
	}
	for lang, c := range stats {
		if c.total < 5 {
			continue // too few files to draw conclusions
		}
		threshold, ok := fallbackBannerThreshold[lang]
		if !ok {
			continue
		}
		ratio := float64(c.t2) / float64(c.total)
		if ratio > threshold {
			logging.Warning(
				"repomap: %d%% of %s files (%d/%d) fell back to Tier-2 — consider extractor/grammar update",
				int(ratio*100), lang, c.t2, c.total)
		}
	}
}
