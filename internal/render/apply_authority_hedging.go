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
//   - Summary: when the underlying evidence pool contains any
//     conditional / historical / illustrative items, append a
//     drift-bounded caveat to doc.Caveats[]. The caveat is
//     bilingual + reflects which downgrade severities are present
//     so a multi-shape answer doesn't get a generic "results may
//     vary" footnote.
//
// Gate semantics: when authority.Enabled() returns false, the
// function returns the doc unchanged. This preserves byte-identical
// legacy behaviour for deployments that haven't opted into the axis.
//
// Locale: the hedge templates have zh / en variants. Pass the same
// language string the surrounding renderer normalises (typically
// e.language on the finalizer evaluator). Other values fall back to
// English templates.
func ApplyAuthorityHedging(doc *types.AnswerDocument, evidence []types.EvidenceItem, lang string) *types.AnswerDocument {
	if doc == nil || !authority.Enabled() {
		return doc
	}
	l := normalizeAnswerDocLang(lang)

	hedgeSteps(doc, evidence, l)
	hedgeSymbols(doc, l)
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
		return authorityCaveatPrefix + "本回答基于多源证据：" + strings.Join(parts, "；") + "。强结论仅由当前仓库内已对齐的证据支撑。"
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
	return authorityCaveatPrefix + "Answer rests on mixed-authority evidence: " + strings.Join(parts, "; ") + ". Strong claims are made only from current-repo anchors that aligned cleanly."
}

// isAuthorityCaveat reports whether s carries the authority-caveat
// sentinel. Used by ApplyAuthorityHedging to dedupe across retries.
func isAuthorityCaveat(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), authorityCaveatPrefix)
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
	authorityCaveatPrefix   = "Authority: "
)

// Backwards-compatible lowercase aliases used inside this file's
// hedge-injection branches. Keeping them avoids touching the per-
// branch string concatenations during the export refactor.
const (
	hedgeMarkerConditional  = HedgeMarkerConditional
	hedgeMarkerHistorical   = HedgeMarkerHistorical
	hedgeMarkerIllustrative = HedgeMarkerIllustrative
)
