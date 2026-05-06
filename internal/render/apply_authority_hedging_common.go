package render

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Common authority-hedging helpers preserved across the V1 → V2
// carrier migration (B8-T2, block_only_carrier.md §5.8). The V1
// per-shape hedgers (hedgeSteps / hedgeSymbols / hedgeSummary /
// hedgeBoolean) are gone; the structural helpers below survive
// because they are consumed by the V2 hedger AND by orchestrator
// reviewers / strip paths that operate on rendered prose regardless
// of the underlying carrier.

// Sentinels visible in user-facing output. HedgeMarker* are inline
// annotations next to specific anchors; AuthorityCaveatPrefix is
// the public visible prefix on the doc-level caveat;
// authorityCaveatTag is a system-private token embedded inside the
// caveat (zero-width-space brackets so it's invisible to the user
// but unambiguously machine-grep-able).
const (
	HedgeMarkerConditional  = "[hedged]"
	HedgeMarkerHistorical   = "[historical]"
	HedgeMarkerIllustrative = "[illustrative]"
	AuthorityCaveatPrefix   = "Authority: "
	authorityCaveatTag      = "​[auth-axis]​"
)

// AuthorityCaveatTag returns the system-private tag embedded inside
// every system-generated authority caveat. Consumers (orchestrator
// gate, downstream Strip) use this for grep so they can distinguish
// system-injected caveats from LLM-written caveats that happen to
// share the public prefix.
func AuthorityCaveatTag() string { return authorityCaveatTag }

// hedgeMarkerFor returns the bare sentinel marker for a ceiling, or
// "" for factual / unknown.
func hedgeMarkerFor(c types.AuthorityCeiling) string {
	switch c {
	case types.AuthorityConditional:
		return HedgeMarkerConditional
	case types.AuthorityHistorical:
		return HedgeMarkerHistorical
	case types.AuthorityIllustrative:
		return HedgeMarkerIllustrative
	}
	return ""
}

// hedgeMarkerSeverity returns a comparable severity score for the
// markers (illustrative > historical > conditional > none). Used by
// upsert helpers to decide whether to UPGRADE a stale label.
func hedgeMarkerSeverity(marker string) int {
	switch marker {
	case HedgeMarkerIllustrative:
		return 3
	case HedgeMarkerHistorical:
		return 2
	case HedgeMarkerConditional:
		return 1
	}
	return 0
}

// leadingHedgeMarker reports whether text begins with one of the
// system markers and returns which one. Order: most-severe first.
func leadingHedgeMarker(text string) (bool, string) {
	switch {
	case strings.HasPrefix(text, HedgeMarkerIllustrative):
		return true, HedgeMarkerIllustrative
	case strings.HasPrefix(text, HedgeMarkerHistorical):
		return true, HedgeMarkerHistorical
	case strings.HasPrefix(text, HedgeMarkerConditional):
		return true, HedgeMarkerConditional
	}
	return false, ""
}

// knownSystemTerseReasons is the canonical set of bracketed reason
// phrases the system might have prepended next to a hedge marker.
func knownSystemTerseReasons() []string {
	return []string{
		"(log-derived; code drifted)",
		"(historical observation)",
		"(illustrative only)",
		"（基于日志，当前代码已漂移）",
		"（旧构建历史观察）",
		"（示例，未验证）",
	}
}

// stripLeadingSystemTerseReason removes a leading bracketed phrase
// IFF it matches one of the system's known terse reasons.
func stripLeadingSystemTerseReason(text string) string {
	for _, reason := range knownSystemTerseReasons() {
		if strings.HasPrefix(text, reason) {
			return strings.TrimSpace(text[len(reason):])
		}
	}
	return text
}

// stripLeadingMarkerAndReason drops a leading marker AND any
// immediately-following SYSTEM-GENERATED terse reason.
func stripLeadingMarkerAndReason(line string) string {
	rest := strings.TrimLeft(line, " \t")
	leadingSpaces := line[:len(line)-len(rest)]
	hasMarker, marker := leadingHedgeMarker(rest)
	if !hasMarker {
		return line
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, marker))
	rest = stripLeadingSystemTerseReason(rest)
	return leadingSpaces + rest
}

// stripInlineMarkerWithReason removes every occurrence of marker AND
// any immediately-following SYSTEM-GENERATED terse reason.
func stripInlineMarkerWithReason(line, marker string) string {
	for {
		idx := strings.Index(line, marker)
		if idx < 0 {
			return line
		}
		head := line[:idx]
		tail := strings.TrimLeft(line[idx+len(marker):], " \t")
		tail = stripLeadingSystemTerseReason(tail)
		separator := ""
		if strings.TrimRight(head, " \t") != "" && tail != "" {
			separator = " "
		}
		line = strings.TrimRight(head, " \t") + separator + tail
	}
}

// StripAuthorityArtifacts removes ALL system-injected authority
// markers, terse reason phrases, and the doc-level Authority caveat
// from rendered draft text. Used by reviewers (SelfConsistency,
// ExternalArtifactDecoded) so they don't see the system's own
// annotations as content.
func StripAuthorityArtifacts(text string) string {
	if text == "" {
		return text
	}
	out := make([]string, 0, len(text)/40+1)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, authorityCaveatTag) {
			continue
		}
		line = stripLeadingMarkerAndReason(line)
		for _, m := range []string{HedgeMarkerIllustrative, HedgeMarkerHistorical, HedgeMarkerConditional} {
			line = stripInlineMarkerWithReason(line, m)
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// StripAuthorityArtifactsForRender removes only the user-visible
// authority markup that should not appear in rendered prose, while
// preserving the system-private authority caveat tag embedded inside
// the caveat line. This lets user-facing output stay clean while the
// rendered draft still carries a machine-grep-able signal for
// contract/reviewer checks.
//
// Differences from StripAuthorityArtifacts:
//   - inline hedge markers / terse reasons are stripped
//   - lines containing authorityCaveatTag are preserved
//   - the private tag itself remains in the rendered string (it's
//     invisible to users because the token contains zero-width chars)
func StripAuthorityArtifactsForRender(text string) string {
	if text == "" {
		return text
	}
	out := make([]string, 0, len(text)/40+1)
	for _, line := range strings.Split(text, "\n") {
		line = stripLeadingMarkerAndReason(line)
		for _, m := range []string{HedgeMarkerIllustrative, HedgeMarkerHistorical, HedgeMarkerConditional} {
			line = stripInlineMarkerWithReason(line, m)
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// authorityCaveatText returns the localized caveat string with the
// system-private tag embedded.
func authorityCaveatText(hist map[types.AuthorityCeiling]int, l answerDocLang) string {
	cond := hist[types.AuthorityConditional]
	histN := hist[types.AuthorityHistorical]
	illust := hist[types.AuthorityIllustrative]

	parts := make([]string, 0, 3)
	if l == answerDocLangZH {
		if cond > 0 {
			parts = append(parts, fmt.Sprintf("%d 条证据来自漂移后的日志/轨迹（已对冲）", cond))
		}
		if histN > 0 {
			parts = append(parts, fmt.Sprintf("%d 条证据是无法映射回当前代码的历史观察", histN))
		}
		if illust > 0 {
			parts = append(parts, fmt.Sprintf("%d 条仅作示例，未参与因果链", illust))
		}
		if len(parts) == 0 {
			return ""
		}
		return AuthorityCaveatPrefix + authorityCaveatTag + "本回答基于多源证据：" + strings.Join(parts, "；") + "。强结论仅由当前仓库内已对齐的证据支撑。"
	}
	if cond > 0 {
		parts = append(parts, fmt.Sprintf("%d evidence item(s) from drifted log/trace (hedged)", cond))
	}
	if histN > 0 {
		parts = append(parts, fmt.Sprintf("%d historical observation(s) without a current-code mapping", histN))
	}
	if illust > 0 {
		parts = append(parts, fmt.Sprintf("%d illustrative item(s) excluded from causal chains", illust))
	}
	if len(parts) == 0 {
		return ""
	}
	return AuthorityCaveatPrefix + authorityCaveatTag + "Answer rests on mixed-authority evidence: " + strings.Join(parts, "; ") + ". Strong claims are made only from current-repo anchors that aligned cleanly."
}

// SynthesiseAuthorityCaveatFor returns the canonical system caveat
// string for the given evidence pool, OR "" when the pool contains
// no non-factual items. Used by the raw-prose fallback path in the
// finalizer evaluator when emit_answer_document was bypassed.
func SynthesiseAuthorityCaveatFor(evidence []types.EvidenceItem, lang string) string {
	hist := authority.AuthorityHistogram(evidence)
	if hist[types.AuthorityConditional]+hist[types.AuthorityHistorical]+hist[types.AuthorityIllustrative] == 0 {
		return ""
	}
	return authorityCaveatText(hist, normalizeAnswerDocLang(lang))
}
