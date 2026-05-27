package index

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

var errTreeSitterParseTimeout = errors.New("tree-sitter parse timed out")

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
	root, _, err := parseTreeSitterRoot(lang, source)
	if err != nil {
		return nil, false
	}
	return root, root != nil
}

func parseTreeSitterRoot(lang string, source []byte) (*sitter.Node, time.Duration, error) {
	tsLang := types.GetSitterLanguage(lang)
	if tsLang == nil {
		return nil, 0, fmt.Errorf("no tree-sitter grammar for %s", lang)
	}
	parser := sitter.NewParser()
	parser.SetLanguage(tsLang)
	ctx := context.Background()
	var cancel context.CancelFunc
	timeout := treeSitterParseTimeout
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	start := time.Now()
	tree, err := parser.ParseCtx(ctx, nil, source)
	elapsed := time.Since(start)
	if err != nil || tree == nil {
		if timeout > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, elapsed, fmt.Errorf("%w after %s", errTreeSitterParseTimeout, timeout)
		}
		return nil, elapsed, err
	}
	root := tree.RootNode()
	if root == nil {
		return nil, elapsed, fmt.Errorf("tree-sitter parse produced nil root")
	}
	return root, elapsed, nil
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

// fallbackBannerThreshold is the legacy per-language Tier-2 ratio
// above which the scanner emits the once-per-build "consider
// grammar update" warning. ArkTS uses 0.4 (TS grammar is well-
// aligned; 40%+ falling through is a real signal); Cangjie uses
// 0.5 (the regex Tier 2 sees more action because the Go-native
// extractor is still maturing — relax the bar). Languages not
// listed here fall through to defaultTierAlertRatio for back-
// compat with the original two-language banner.
//
// Public test seam: a unit test reads these to assert no silent
// drift if a future change tightens the bar.
var fallbackBannerThreshold = map[string]float64{
	types.LangArkTS:   0.4,
	types.LangCangjie: 0.5,
}

// Default thresholds for the global tier-distribution alert
// (T3.2). Operators can override via codrax.yaml ::
// repomap_tier_warn_ratio / repomap_tier_alert_ratio. WARN
// boundary is the lower bar (signals an emerging issue);
// ALERT is the higher bar that triggers per-language remediation
// banner. INFO summary always emits when any language has files,
// regardless of threshold.
const (
	defaultTierWarnRatio  = 0.30
	defaultTierAlertRatio = 0.50
	tierStatsMinFiles     = 5 // languages with fewer files: skip ratio judgement
)

var (
	tierWarnRatio  = defaultTierWarnRatio
	tierAlertRatio = defaultTierAlertRatio
)

// SetTierThresholds plumbs the yaml-resolved thresholds into the
// repomap reporter. Callers (cmd/root.go) invoke this once at
// startup before any ScanFiles runs. Zero on either field
// inherits the package default.
func SetTierThresholds(warn, alert float64) {
	if warn > 0 {
		tierWarnRatio = warn
	}
	if alert > 0 {
		tierAlertRatio = alert
	}
}

// reportFallbackRatios is called once per ScanFiles run. It walks
// the produced FileInfos and emits:
//
//  1. ONE INFO summary line listing per-language Tier 1/2/3/4
//     distribution for every language with >= tierStatsMinFiles
//     files. Always fires so operators can see the parse-tier
//     health at a glance, even when nothing exceeds a threshold.
//
//  2. ONE WARN line per language whose Tier-2+ ratio exceeds
//     the alert threshold (fallbackBannerThreshold or
//     tierAlertRatio default), naming the language + ratio +
//     remediation hint.
//
// Idempotent: callers do not need to gate this on "first time"
// — it just emits at most one of each per language per call.
func reportFallbackRatios(files []*types.FileInfo) {
	type counts struct {
		total int
		tier  [5]int // tier[1..4]; tier[0] unused
	}
	stats := map[string]*counts{}
	for _, fi := range files {
		if strings.TrimSpace(fi.Language) == "" {
			continue
		}
		c, ok := stats[fi.Language]
		if !ok {
			c = &counts{}
			stats[fi.Language] = c
		}
		c.total++
		t := fi.ParseTier
		if t < 1 {
			t = 1 // empty / pre-Tier code path counts as Tier 1
		}
		if t > 4 {
			t = 4
		}
		c.tier[t]++
	}

	// Stage 1: INFO summary. Sorted languages so the line is
	// stable across runs (no map-iteration jitter in test logs).
	if len(stats) > 0 {
		langs := make([]string, 0, len(stats))
		for lang, c := range stats {
			if c.total >= tierStatsMinFiles {
				langs = append(langs, lang)
			}
		}
		if len(langs) > 0 {
			sort.Strings(langs)
			parts := make([]string, 0, len(langs))
			for _, lang := range langs {
				c := stats[lang]
				degradedPct := int(100 * float64(c.tier[2]+c.tier[3]+c.tier[4]) / float64(c.total))
				if degradedPct == 0 {
					parts = append(parts, fmt.Sprintf("%s: %d files all T1", lang, c.total))
					continue
				}
				parts = append(parts, fmt.Sprintf("%s: %d files (T1=%d T2=%d T3=%d T4=%d, %d%% degraded)",
					lang, c.total, c.tier[1], c.tier[2], c.tier[3], c.tier[4], degradedPct))
			}
			logging.Info("repomap parse-tier breakdown — %s", strings.Join(parts, "; "))
		}
	}

	// Stage 2: per-language WARN above alert threshold.
	for lang, c := range stats {
		if c.total < tierStatsMinFiles {
			continue
		}
		degraded := c.tier[2] + c.tier[3] + c.tier[4]
		ratio := float64(degraded) / float64(c.total)
		threshold := tierAlertRatio
		if perLang, ok := fallbackBannerThreshold[lang]; ok {
			threshold = perLang
		}
		if ratio > threshold {
			logging.Warning(
				"repomap: %d%% of %s files (%d/%d) fell back to Tier-2+ (T2=%d T3=%d T4=%d) — consider extractor/grammar update",
				int(ratio*100), lang, degraded, c.total, c.tier[2], c.tier[3], c.tier[4])
		} else if ratio > tierWarnRatio {
			// Below alert but above the soft warn threshold:
			// log at INFO with a milder phrasing so operators
			// can see emerging degradation before it breaches.
			logging.Info(
				"repomap: %d%% of %s files (%d/%d) below Tier-1 — within tolerance but trending toward extractor maintenance",
				int(ratio*100), lang, degraded, c.total)
		}
	}
}
