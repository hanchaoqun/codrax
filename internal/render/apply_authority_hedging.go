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
		// HighestAuthorityFor — at least one factual underlying item
		// permits a strong claim. Per-step uses MARKER-ONLY (no body)
		// because the doc-level Caveat already carries the prose
		// explanation; repeating it on every step dilutes the answer.
		ceiling := authority.HighestAuthorityFor(evidence, cit.File, cit.Line)
		marker := hedgeMarkerFor(ceiling)
		if marker == "" {
			continue
		}
		step.Description = upsertHedgeMarker(step.Description, marker)
	}
}

func hedgeSymbols(doc *types.AnswerDocument, l answerDocLang) {
	if len(doc.Symbols) == 0 {
		return
	}
	for i := range doc.Symbols {
		sym := &doc.Symbols[i]
		marker := hedgeMarkerFor(sym.Authority)
		if marker == "" {
			continue
		}
		// Marker-only inline (the doc-level Caveat carries the body).
		// When rationale is empty, set it to a short signed phrase so
		// the renderer's table column has visible content; otherwise
		// upsert the marker prefix.
		if strings.TrimSpace(sym.Rationale) == "" {
			sym.Rationale = marker + " " + shortHedgeReasonFor(sym.Authority, l)
			continue
		}
		sym.Rationale = upsertHedgeMarker(sym.Rationale, marker)
	}
}

// hedgeSummary projects the strongest hedge ceiling implied by
// doc.Citations[] onto doc.Summary. Covers value / config_value /
// explanation shapes whose Summary is the answer body or its lead-in.
//
// Summary uses MARKER + TERSE reason (≤30 chars) because Summary is
// the prominent prose anchor; per-step + per-symbol use marker only.
// Idempotent across retries via upsertHedgeMarker which also UPGRADES
// the ceiling label when stronger evidence arrives on retry.
func hedgeSummary(doc *types.AnswerDocument, evidence []types.EvidenceItem, l answerDocLang) {
	summary := strings.TrimSpace(doc.Summary)
	if summary == "" {
		return
	}
	// Summary spans the whole answer — use the full citation pool.
	ceiling := strongestCitedCeiling(doc.Citations, evidence)
	marker := hedgeMarkerFor(ceiling)
	if marker == "" {
		return
	}
	prefix := marker + " " + shortHedgeReasonFor(ceiling, l)
	doc.Summary = upsertHedgePrefix(summary, marker, prefix)
}

// hedgeBoolean is the Boolean shape's analogue: project the hedge
// onto doc.Boolean.Rationale. Picks the ceiling first from
// Boolean.CitationRef, then falls back to the broader citation pool
// (a YES/NO answer often draws on multiple anchors but only carries
// one citation_ref). Marker + terse reason (rationale is prominent).
func hedgeBoolean(doc *types.AnswerDocument, evidence []types.EvidenceItem, l answerDocLang) {
	if doc.Boolean == nil {
		return
	}
	rationale := strings.TrimSpace(doc.Boolean.Rationale)
	if rationale == "" {
		return
	}
	ceiling := types.AuthorityUnknown
	if ref := doc.Boolean.CitationRef; ref >= 0 && ref < len(doc.Citations) {
		cit := doc.Citations[ref]
		ceiling = authority.HighestAuthorityFor(evidence, cit.File, cit.Line)
	}
	if ceiling == types.AuthorityUnknown || ceiling == types.AuthorityFactual {
		ceiling = strongestCitedCeiling(doc.Citations, evidence)
	}
	marker := hedgeMarkerFor(ceiling)
	if marker == "" {
		return
	}
	prefix := marker + " " + shortHedgeReasonFor(ceiling, l)
	doc.Boolean.Rationale = upsertHedgePrefix(rationale, marker, prefix)
}

// strongestCitedCeiling walks every Citation in `cites` and returns
// the STRONGEST non-factual ceiling implied by the underlying
// evidence pool.
//
// "Strongest" here means most severe (illustrative > historical >
// conditional), which is the opposite of HighestAuthorityFor's "best
// available evidence" — we want the hedge to reflect the worst-case
// claim the answer rests on.
//
// Round-4 refinement: takes a CITATION SUBSET (not the full doc) so
// callers can scope the worst-case calculation to only the cites
// that actually back the prose surface being hedged. hedgeSummary
// passes the full pool (Summary spans the whole answer); hedgeStep
// / hedgeSymbol pass just their own citation_ref. Without this,
// hedging a single-cite step inherited the worst-case label of the
// whole doc — over-hedging factual sub-topics in multi-topic
// explanations.
func strongestCitedCeiling(cites []types.Citation, evidence []types.EvidenceItem) types.AuthorityCeiling {
	worst := types.AuthorityUnknown
	worstRank := 5 // higher than any real ceiling rank in this scope
	for _, cit := range cites {
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

// (alreadyHasHedgePrefix retired — superseded by upsertHedgeMarker
// + upsertHedgePrefix which both subsume the idempotency check AND
// allow ceiling upgrades.)

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

// hedgeMarkerFor returns the bare sentinel marker for a ceiling, or
// "" for factual / unknown. Used inline (per-step, per-symbol) where
// the doc-level Caveat already carries the prose explanation —
// repeating it on every step / symbol dilutes the answer with
// redundant prose and trains user attention to ignore the signal.
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

// shortHedgeReasonFor returns a TERSE bilingual reason (≤30 chars)
// suitable for prominent prose anchors (Summary / Boolean.Rationale)
// where bare marker would feel cryptic. The doc-level Caveat carries
// the long-form explanation; keep this terse so it doesn't crowd out
// the actual answer content.
func shortHedgeReasonFor(c types.AuthorityCeiling, l answerDocLang) string {
	if l == answerDocLangZH {
		switch c {
		case types.AuthorityConditional:
			return "（基于日志，当前代码已漂移）"
		case types.AuthorityHistorical:
			return "（旧构建历史观察）"
		case types.AuthorityIllustrative:
			return "（示例，未验证）"
		}
		return ""
	}
	switch c {
	case types.AuthorityConditional:
		return "(log-derived; code drifted)"
	case types.AuthorityHistorical:
		return "(historical observation)"
	case types.AuthorityIllustrative:
		return "(illustrative only)"
	}
	return ""
}

// upsertHedgeMarker prepends the marker IFF text doesn't already
// carry it. Allows ceiling UPGRADES across retries: if text begins
// with [hedged] and we now need [historical] (worse case), replace
// the leading sentinel with the stronger one. This prevents stale
// labels in retry chains where the evidence pool grew in severity.
func upsertHedgeMarker(text, marker string) string {
	text = strings.TrimSpace(text)
	if hasMarker, current := leadingHedgeMarker(text); hasMarker {
		if hedgeMarkerSeverity(current) >= hedgeMarkerSeverity(marker) {
			return text
		}
		// Upgrade — strip the old leading marker and re-prepend.
		text = strings.TrimSpace(strings.TrimPrefix(text, current))
	}
	return strings.TrimSpace(marker + " " + text)
}

// upsertHedgePrefix prepends a marker + reason phrase IFF the text's
// leading marker is weaker (or absent). Mirrors upsertHedgeMarker for
// callers (Summary / Rationale) that want the terse reason in
// addition to the marker.
func upsertHedgePrefix(text, marker, fullPrefix string) string {
	text = strings.TrimSpace(text)
	if hasMarker, current := leadingHedgeMarker(text); hasMarker {
		if hedgeMarkerSeverity(current) >= hedgeMarkerSeverity(marker) {
			return text
		}
		text = strings.TrimSpace(strings.TrimPrefix(text, current))
		// Drop the leading short-reason that travelled with the older
		// marker. Byte limits are generous (round-4): Chinese terse
		// reason is up to ~80 bytes UTF-8; long-form prose seen on
		// upgrade-from-prior-Caveat paths can be longer.
		if strings.HasPrefix(text, "(") {
			if end := strings.Index(text, ")"); end > 0 && end < 200 {
				text = strings.TrimSpace(text[end+1:])
			}
		} else if strings.HasPrefix(text, "（") {
			if end := strings.Index(text, "）"); end > 0 && end < 400 {
				text = strings.TrimSpace(text[end+len("）"):])
			}
		}
	}
	return strings.TrimSpace(fullPrefix + " " + text)
}

// leadingHedgeMarker reports whether text begins with one of the
// system markers and returns which one. Order: most-severe first,
// so a string starting with [illustrative] reports illustrative
// (not a substring trip on [hedged]).
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

// StripAuthorityArtifacts removes ALL system-injected authority
// markers, terse reason phrases, and the doc-level Authority caveat
// from the rendered draft text. Used by downstream reviewers
// (SelfConsistency, ExternalArtifactDecoded) so they don't see the
// system's own annotations as content — without stripping, the
// reviewer LLM treats "[hedged] X" vs "X" as a factual contradiction,
// and the artifact-decoded check counts hedge-body tokens like "log"
// / "perf" as proof the LLM decoded the bundle.
//
// Best-effort: strips the markers + terse `(...)` / `（...）` reasons
// that follow them + any line beginning with the AuthorityCaveatPrefix
// sentinel. The caller's reviewer prompt operates on what remains.
//
// Round-4 refinement: also strips inline terse reasons that appear
// after a marker mid-line (the marker ReplaceAll alone left the
// `(log-derived; code drifted)` phrase behind, polluting downstream
// runExternalArtifactDecodedCheck token-match scans).
func StripAuthorityArtifacts(text string) string {
	if text == "" {
		return text
	}
	out := make([]string, 0, len(text)/40+1)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		// Drop entire line when it IS the doc-level caveat. Match by
		// the system-private tag (round-4) so an LLM-written caveat
		// starting with the public prefix is preserved (only OUR
		// caveat carries the tag).
		if strings.Contains(trimmed, authorityCaveatTag) {
			continue
		}
		// Strip leading markers + their terse reason from the line.
		line = stripLeadingMarkerAndReason(line)
		// Mid-line marker + adjacent terse reason: marker
		// ReplaceAll first, then sweep adjacent reason brackets.
		for _, m := range []string{HedgeMarkerIllustrative, HedgeMarkerHistorical, HedgeMarkerConditional} {
			line = stripInlineMarkerWithReason(line, m)
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// stripInlineMarkerWithReason removes every occurrence of `marker`
// AND any immediately-following bracketed reason phrase (English `()`
// or Chinese `（）`). Loop-based replace handles multiple inline
// occurrences in the same line.
func stripInlineMarkerWithReason(line, marker string) string {
	for {
		idx := strings.Index(line, marker)
		if idx < 0 {
			return line
		}
		head := line[:idx]
		tail := strings.TrimLeft(line[idx+len(marker):], " \t")
		// Drop adjacent reason bracket if present.
		if strings.HasPrefix(tail, "(") {
			if end := strings.Index(tail, ")"); end > 0 && end < 200 {
				tail = strings.TrimSpace(tail[end+1:])
			}
		} else if strings.HasPrefix(tail, "（") {
			if end := strings.Index(tail, "）"); end > 0 && end < 400 {
				tail = strings.TrimSpace(tail[end+len("）"):])
			}
		}
		// Stitch with a single space when both sides have content.
		separator := ""
		if strings.TrimRight(head, " \t") != "" && tail != "" {
			separator = " "
		}
		line = strings.TrimRight(head, " \t") + separator + tail
	}
}

// stripLeadingMarkerAndReason drops a leading marker AND any
// immediately-following bracketed reason phrase. Whitespace-tolerant.
func stripLeadingMarkerAndReason(line string) string {
	rest := strings.TrimLeft(line, " \t")
	leadingSpaces := line[:len(line)-len(rest)]
	hasMarker, marker := leadingHedgeMarker(rest)
	if !hasMarker {
		return line
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, marker))
	// Byte limits raised in round-4: Chinese terse reason is up to ~80
	// bytes (UTF-8 3 bytes/char); long-form caveat body is even longer
	// when the markers travel inline (defense in depth — the caveat
	// itself is dropped via the line-prefix rule above).
	if strings.HasPrefix(rest, "(") {
		if end := strings.Index(rest, ")"); end > 0 && end < 200 {
			rest = strings.TrimSpace(rest[end+1:])
		}
	} else if strings.HasPrefix(rest, "（") {
		if end := strings.Index(rest, "）"); end > 0 && end < 400 {
			rest = strings.TrimSpace(rest[end+len("）"):])
		}
	}
	return leadingSpaces + rest
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

// AuthorityCaveatTag returns the system-private tag embedded inside
// every system-generated authority caveat. Consumers (the orchestrator
// gate, downstream Strip) use this for grep so they can distinguish
// system-injected caveats from LLM-written ones that happen to share
// the public "Authority: " prefix.
func AuthorityCaveatTag() string { return authorityCaveatTag }

// isAuthorityCaveat reports whether s carries the SYSTEM-injected
// authority-caveat sentinel. Round-4 refinement: dedup looks for
// authorityCaveatTag (a private internal token the LLM cannot
// guess). Pre-round-4 the dedup matched any caveat starting with
// "Authority: " — but the answer-document-skill teaches the LLM to
// write Caveats[] with descriptive prefixes, and a user-written
// "Authority: <something>" caveat would either get overwritten by
// the system or shadow the system's own caveat. Tagging makes the
// system's caveat unambiguously identifiable; the public-facing
// AuthorityCaveatPrefix is what the user sees, the tag is what the
// system uses for plumbing.
func isAuthorityCaveat(s string) bool {
	return strings.Contains(s, authorityCaveatTag)
}

// Sentinels — chosen to be visible in the rendered output but
// distinctive enough that the contract checker (commit 6) can grep
// for them as proof that hedge injection ran. Exported so
// orchestrator.runAuthorityOverreachCheck can grep without
// duplicating string literals (single source of truth).
//
// authorityCaveatTag is a private invisible-ish token embedded INSIDE
// the system-generated caveat string. The dedup path
// (isAuthorityCaveat) looks for the tag rather than the public
// "Authority: " prefix, so an LLM-written caveat that happens to
// start with "Authority: " is not mistaken for the system's caveat.
// The tag uses zero-width space + ASCII brackets so it's visually
// invisible in user-facing prose but unambiguously machine-grep-able.
const (
	HedgeMarkerConditional  = "[hedged]"
	HedgeMarkerHistorical   = "[historical]"
	HedgeMarkerIllustrative = "[illustrative]"
	AuthorityCaveatPrefix   = "Authority: "
	authorityCaveatTag      = "​[auth-axis]​"
)

// Backwards-compatible lowercase aliases used inside this file's
// hedge-injection branches. Keeping them avoids touching the per-
// branch string concatenations during the export refactor.
const (
	hedgeMarkerConditional  = HedgeMarkerConditional
	hedgeMarkerHistorical   = HedgeMarkerHistorical
	hedgeMarkerIllustrative = HedgeMarkerIllustrative
)
