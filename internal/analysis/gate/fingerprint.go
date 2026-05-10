// Package gate — fingerprint.go
//
// computeGateFingerprint produces a stable hash of a rejection's
// SHAPE (which checks failed and which rule prefixes within each
// detail), NOT its specific quoted entity values. The orchestrator
// uses it to detect a retry storm — same failure shape repeating
// across attempts means the LLM is stuck in a loop, not converging.
//
// Algorithm:
//
//  1. Per failed check: collect the check Name plus the rule code
//     prefix found in each segment of the Detail string (rules are
//     joined by " | " in the multi-rule path). Rule prefix is
//     "R<num>.<num> <slug>:" — extracted via rulePrefixRE.
//  2. Fallback for non-rule-prefixed details: drop quoted strings
//     (which carry entity / scope names that vary between attempts)
//     and collapse digit runs to "N" (counts vary between attempts).
//     Both are signal-side noise we don't want fingerprint-keyed on.
//  3. Sort the collected keys, join with ";", SHA-256, hex-encode
//     the first 6 bytes (12 hex chars). Bounded length, deterministic
//     across runs.
//
// Returns empty string when the report is not rejected — empty
// fingerprint means "no storm to detect".
//
// Design doc: docs/design/multirepo_entity_scope_separation.md §4.6.
package gate

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	// rulePrefixRE matches the "R<digits>.<digits> <slug>:" prefix at
	// the start of a coherence Detail segment. Optional " (advisory)"
	// for SOFT advisory rules. Same prefix list as analyzer.go ::
	// stripCoherencePrefix; kept independent here so a future package
	// move doesn't carry a hidden import cycle.
	rulePrefixRE = regexp.MustCompile(`^R\d+\.\d+ [a-z_]+(?:\s*\(advisory\))?:`)
	digitRE      = regexp.MustCompile(`\d+`)
	quotedRE     = regexp.MustCompile(`"[^"]*"`)
	bracketedRE  = regexp.MustCompile(`\[[^\]]*\]`)
)

// computeGateFingerprint returns a stable 12-hex-char hash of the
// failure shape. Returns "" when report.Rejected is false.
func computeGateFingerprint(report types.GateReport) string {
	if !report.Rejected {
		return ""
	}
	var keys []string
	for _, c := range report.Checks {
		if c.Passed {
			continue
		}
		segments := strings.Split(c.Detail, " | ")
		any := false
		for _, seg := range segments {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			if m := rulePrefixRE.FindString(seg); m != "" {
				// Rule-prefixed detail — fingerprint on the rule
				// name + check name. Specific entity values that
				// follow the prefix (which differ between attempts)
				// are intentionally NOT in the fingerprint.
				keys = append(keys, c.Name+":"+strings.TrimSuffix(m, ":"))
				any = true
				continue
			}
			// Non-rule-prefixed (legacy) — drop quoted spans, drop
			// bracketed lists, collapse digit runs. The remaining
			// shape captures "this kind of failure" while staying
			// stable across attempts.
			clean := quotedRE.ReplaceAllString(seg, `""`)
			clean = bracketedRE.ReplaceAllString(clean, `[]`)
			clean = digitRE.ReplaceAllString(clean, "N")
			keys = append(keys, c.Name+":"+clean)
			any = true
		}
		if !any {
			// Failed check with no detail at all — still record the
			// name so two empty-detail attempts still match.
			keys = append(keys, c.Name+":")
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	h := sha256.Sum256([]byte(strings.Join(keys, ";")))
	return hex.EncodeToString(h[:6])
}
