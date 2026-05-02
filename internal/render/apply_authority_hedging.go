package render

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ApplyAuthorityHedging is the render-time projection from a typed
// AnswerDocument + evidence pool into a hedge-aware AnswerDocument
// the renderer can serialise without further awareness. Pure
// function (no I/O, no LLM) — same inputs always produce the same
// output. Returns the in-place mutated doc for caller convenience.
//
// Hedging strategy by document shape:
//
//   - Steps: each AnswerStep's Description is prefixed with a
//     hedge sentinel when the step's CitationRef points at an anchor
//     whose underlying evidence has Authority ∈ {conditional,
//     historical}. The strongest authority among matching evidence
//     wins (HighestAuthorityFor) — even one factual support permits
//     a strong claim, so steps with mixed support stay un-hedged.
//
//   - Symbols: each AnswerSymbol with Authority ∈ {conditional,
//     historical} has its Rationale prefixed with a hedge sentinel.
//     Symbols with Authority=illustrative are MOVED to a separate
//     "Unverified Leads" tier (modelled by a per-symbol prefix the
//     downstream renderer surfaces with strikethrough), so they
//     cannot enter causal chains in the user-visible prose.
//
//   - Summary: when ANY citation in doc.Citations[] points at an
//     anchor whose evidence is non-factual, prefix doc.Summary with
//     the strongest required hedge sentinel. Covers shape=value /
//     config_value / explanation where Summary IS the body (or its
//     lead-in) — without this, the contract checker would fire on
//     a summary that cites drift-bounded evidence and force a retry
//     the LLM cannot satisfy (the renderer is the only injector).
//
//   - Boolean.Rationale: same projection as Summary but scoped to
//     the Boolean shape's free-form rationale field, which the
//     renderer prints as the body of YES/NO answers.
//
//   - Caveats: when the underlying evidence pool contains any
//     conditional / historical / illustrative items, append a
//     drift-bounded caveat to doc.Caveats[]. The caveat is
//     bilingual + reflects which downgrade severities are present
//     so a multi-shape answer doesn't get a generic "results may
//     vary" footnote.
//
// Locale: the hedge templates have zh / en variants. Pass the same
// language string the surrounding renderer normalises (typically
// e.language on the finalizer evaluator). Other values fall back to
// English templates.
//
// nil-doc safe: returns the input unchanged.
func ApplyAuthorityHedging(doc *types.AnswerDocument, evidence []types.EvidenceItem, lang string) *types.AnswerDocument {
	if doc == nil {
		return doc
	}
	l := normalizeAnswerDocLang(lang)

	hedgeSteps(doc, evidence, l)
	hedgeSymbols(doc, l)
	hedgeSummary(doc, evidence, l)
	hedgeBoolean(doc, evidence, l)
	addAuthorityCaveat(doc, evidence, l)

	return doc
}

func hedgeSteps(doc *types.AnswerDocument, evidence []types.EvidenceItem, l answerDocLang) {
	if len(doc.Steps) == 0 || len(doc.Citations) == 0 {
		return
	}
	for i := range doc.Steps {
		step := &doc.Steps[i]
		ref := step.CitationRef
		if ref < 0 || ref >= len(doc.Citations) {
			continue
		}
		cit := doc.Citations[ref]
		// Use HighestAuthorityFor — at least one factual underlying
		// item permits a strong claim. The user-friendly principle is
		// "credit the LLM for the strongest support it has".
		ceiling := authority.HighestAuthorityFor(evidence, cit.File, cit.Line)
		prefix := hedgePrefixFor(ceiling, l)
		if prefix == "" {
			continue
		}
		step.Description = strings.TrimSpace(prefix + " " + step.Description)
	}
}

func hedgeSymbols(doc *types.AnswerDocument, l answerDocLang) {
	if len(doc.Symbols) == 0 {
		return
	}
	for i := range doc.Symbols {
		sym := &doc.Symbols[i]
		prefix := hedgePrefixFor(sym.Authority, l)
		if prefix == "" {
			continue
		}
		// Inject the hedge into the rationale (which is the prose
		// surface the renderer puts after the symbol name). When
		// rationale is empty, set it to the prefix alone — the
		// renderer treats non-empty rationale as a parenthetical so
		// the hedge becomes visible without restructuring the doc.
		if strings.TrimSpace(sym.Rationale) == "" {
			sym.Rationale = prefix
			continue
		}
		sym.Rationale = strings.TrimSpace(prefix + " " + sym.Rationale)
	}
}

// hedgeSummary projects the strongest hedge ceiling implied by
// doc.Citations[] onto doc.Summary. Covers value / config_value /
// explanation shapes whose Summary is either the answer body or its
// lead-in. Without this, runAuthorityOverreachCheck would fire on a
// summary that cites drift-bounded evidence and force an unsatisfiable
// retry (the LLM cannot inject sentinels — only the renderer can).
//
// Idempotent: when the summary already starts with a hedge sentinel
// (re-render path), this is a no-op.
func hedgeSummary(doc *types.AnswerDocument, evidence []types.EvidenceItem, l answerDocLang) {
	summary := strings.TrimSpace(doc.Summary)
	if summary == "" || alreadyHasHedgePrefix(summary) {
		return
	}
	ceiling := strongestCitedCeiling(doc, evidence)
	prefix := hedgePrefixFor(ceiling, l)
	if prefix == "" {
		return
	}
	doc.Summary = strings.TrimSpace(prefix + " " + summary)
}

// hedgeBoolean is the Boolean shape's analogue: prefix
// doc.Boolean.Rationale with the hedge implied by Boolean.CitationRef
// (and the broader citation pool, in case the rationale draws on
// multiple cites). The renderer prints rationale verbatim after the
// YES/NO marker, so injecting the sentinel here surfaces it to the
// user.
func hedgeBoolean(doc *types.AnswerDocument, evidence []types.EvidenceItem, l answerDocLang) {
	if doc.Boolean == nil {
		return
	}
	rationale := strings.TrimSpace(doc.Boolean.Rationale)
	if rationale == "" || alreadyHasHedgePrefix(rationale) {
		return
	}
	// Walk the boolean's own citation first, then fall back to the
	// broader citation pool — a boolean answer often draws on multiple
	// anchors but only carries one citation_ref.
	ceiling := types.AuthorityUnknown
	if ref := doc.Boolean.CitationRef; ref >= 0 && ref < len(doc.Citations) {
		cit := doc.Citations[ref]
		ceiling = authority.HighestAuthorityFor(evidence, cit.File, cit.Line)
	}
	if ceiling == types.AuthorityUnknown || ceiling == types.AuthorityFactual {
		// Tighten with the broader citation pool — if any other cite
		// is hedged, surface the strongest required ceiling.
		ceiling = strongestCitedCeiling(doc, evidence)
	}
	prefix := hedgePrefixFor(ceiling, l)
	if prefix == "" {
		return
	}
	doc.Boolean.Rationale = strings.TrimSpace(prefix + " " + rationale)
}

// strongestCitedCeiling walks every Citation in doc and returns the
// STRONGEST non-factual ceiling implied by the underlying evidence
// pool. Used by hedgeSummary / hedgeBoolean to pick the right hedge
// label when no single citation_ref is authoritative for the body.
//
// "Strongest" here means most severe (illustrative > historical >
// conditional), which is the opposite of HighestAuthorityFor's "best
// available evidence" — we want the hedge to reflect the worst-case
// claim the answer rests on.
func strongestCitedCeiling(doc *types.AnswerDocument, evidence []types.EvidenceItem) types.AuthorityCeiling {
	worst := types.AuthorityUnknown
	worstRank := 5 // higher than any real ceiling rank in this scope
	for _, cit := range doc.Citations {
		ceiling := authority.HighestAuthorityFor(evidence, cit.File, cit.Line)
		switch ceiling {
		case types.AuthorityIllustrative:
			if 1 < worstRank {
				worst = ceiling
				worstRank = 1
			}
		case types.AuthorityHistorical:
			if 2 < worstRank {
				worst = ceiling
				worstRank = 2
			}
		case types.AuthorityConditional:
			if 3 < worstRank {
				worst = ceiling
				worstRank = 3
			}
		}
	}
	return worst
}

// alreadyHasHedgePrefix reports whether s starts with one of the
// system-injected hedge sentinels. Used to make hedgeSummary /
// hedgeBoolean idempotent across re-render paths so a retry doesn't
// double-prefix.
func alreadyHasHedgePrefix(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, hedgeMarkerConditional) ||
		strings.HasPrefix(s, hedgeMarkerHistorical) ||
		strings.HasPrefix(s, hedgeMarkerIllustrative)
}

func addAuthorityCaveat(doc *types.AnswerDocument, evidence []types.EvidenceItem, l answerDocLang) {
	hist := authority.AuthorityHistogram(evidence)
	hasConditional := hist[types.AuthorityConditional] > 0
	hasHistorical := hist[types.AuthorityHistorical] > 0
	hasIllustrative := hist[types.AuthorityIllustrative] > 0
	if !hasConditional && !hasHistorical && !hasIllustrative {
		return
	}

	caveat := authorityCaveatText(hist, l)
	if caveat == "" {
		return
	}
	// Avoid duplicate caveats on retry — when the prior pass already
	// appended an authority caveat, replacing rather than appending.
	for i, existing := range doc.Caveats {
		if isAuthorityCaveat(existing) {
			doc.Caveats[i] = caveat
			return
		}
	}
	doc.Caveats = append(doc.Caveats, caveat)
}

// hedgePrefixFor returns the hedge sentinel (with leading sentinel
// marker the renderer keys off) for a given AuthorityCeiling, or ""
// when no hedge is needed.
//
// Sentinels are unicode-decorated so they survive markdown rendering
// without colliding with normal prose, AND so the contract checker
// (commit 6) can grep for them as proof that hedge was applied.
func hedgePrefixFor(c types.AuthorityCeiling, l answerDocLang) string {
	switch c {
	case types.AuthorityConditional:
		return hedgeMarkerConditional + " " + hedgeBodyFor(c, l)
	case types.AuthorityHistorical:
		return hedgeMarkerHistorical + " " + hedgeBodyFor(c, l)
	case types.AuthorityIllustrative:
		return hedgeMarkerIllustrative + " " + hedgeBodyFor(c, l)
	}
	return ""
}

func hedgeBodyFor(c types.AuthorityCeiling, l answerDocLang) string {
	if l == answerDocLangZH {
		switch c {
		case types.AuthorityConditional:
			return "（基于日志/性能轨迹观察，当前代码已有漂移；以下结论需谨慎）"
		case types.AuthorityHistorical:
			return "（旧构建中的历史观察；当前代码已重构或对应符号已不存在）"
		case types.AuthorityIllustrative:
			return "（仅作示例，未经验证）"
		}
		return ""
	}
	switch c {
	case types.AuthorityConditional:
		return "(based on log/perf observation; current code has drifted — claim hedged)"
	case types.AuthorityHistorical:
		return "(historical observation from an older build; current code has refactored or removed the corresponding implementation)"
	case types.AuthorityIllustrative:
		return "(illustrative only, not verified)"
	}
	return ""
}

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
		return AuthorityCaveatPrefix + "本回答基于多源证据：" + strings.Join(parts, "；") + "。强结论仅由当前仓库内已对齐的证据支撑。"
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
	return AuthorityCaveatPrefix + "Answer rests on mixed-authority evidence: " + strings.Join(parts, "; ") + ". Strong claims are made only from current-repo anchors that aligned cleanly."
}

// isAuthorityCaveat reports whether s carries the authority-caveat
// sentinel. Used by ApplyAuthorityHedging to dedupe across retries.
func isAuthorityCaveat(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), AuthorityCaveatPrefix)
}

// Sentinels — chosen to be visible in the rendered output but
// distinctive enough that the contract checker (commit 6) can grep
// for them as proof that hedge injection ran. Exported so
// orchestrator.runAuthorityOverreachCheck can grep without
// duplicating string literals (single source of truth).
//
// The prefix string on caveats is grep-able via isAuthorityCaveat
// for the dedup-on-retry path.
const (
	HedgeMarkerConditional  = "[hedged]"
	HedgeMarkerHistorical   = "[historical]"
	HedgeMarkerIllustrative = "[illustrative]"
	AuthorityCaveatPrefix   = "Authority: "
)

// Backwards-compatible lowercase aliases used inside this file's
// hedge-injection branches. Keeping them avoids touching the per-
// branch string concatenations during the export refactor.
const (
	hedgeMarkerConditional  = HedgeMarkerConditional
	hedgeMarkerHistorical   = HedgeMarkerHistorical
	hedgeMarkerIllustrative = HedgeMarkerIllustrative
)
