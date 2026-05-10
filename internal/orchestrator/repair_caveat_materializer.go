package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// repair_caveat_materializer.go — Plan W1.2 (2026-05-05).
//
// Translates unresolved Violation lists into user-facing natural-
// language caveats by way of the ViolKindSpec.CaveatFamilyID +
// CaveatFamilyTemplate registry. The output goes into
// AnswerDocumentV2.Caveats[] so the user sees a polite "the system
// could not fully verify X" sentence — never the orchestration's
// internal vocabulary (ViolKind names, IR fields, confidence
// scores, yield-kill markers, etc.).
//
// Replaces the prependFailLoudWarning code path that pre-2026-05-05
// dumped closure ledger contents directly into the user-visible
// answer string. See docs/design/iteration_inflation_remediation.md
// W1 for design rationale.
//
// The function is pure: input violations + language → output
// caveat strings. No state, no side effects. Called from the
// orchestrator at the points where retry has exhausted and the
// answer is shipping with unresolved gaps.

// MaxMaterializedCaveats caps the number of system-generated caveats
// per answer to avoid drowning the user. Beyond this, additional
// families are dropped (the LLM-authored caveats already in the
// answer document take precedence as a separate channel).
const MaxMaterializedCaveats = 3

// MaterializeUnresolvedViolationsAsCaveats groups violations by
// CaveatFamilyID, looks up the user-facing template per language,
// and returns up to MaxMaterializedCaveats deduplicated caveat
// strings ready to append to AnswerDocumentV2.Caveats.
//
// Empty / nil input returns nil. Violations whose ViolKindSpec
// either is unregistered or has empty CaveatFamilyID are skipped
// silently — those are operator-only telemetry signals.
//
// lang: "zh" / "zh-cn" / "cn" / "chinese" → ZH template; anything
// else → EN. Empty defaults to ZH (default project language).
//
// Output stability: family declaration order via
// AllCaveatFamilies, then alphabetical by family ID for any
// families whose declaration order is undefined.
func MaterializeUnresolvedViolationsAsCaveats(violations []types.Violation, lang string) []string {
	if len(violations) == 0 {
		return nil
	}
	hit := make(map[string]bool, len(violations))
	for _, v := range violations {
		spec, ok := types.ViolKindSpecFor(v.Kind)
		if !ok {
			continue
		}
		if spec.CaveatFamilyID == "" {
			continue
		}
		hit[spec.CaveatFamilyID] = true
	}
	if len(hit) == 0 {
		return nil
	}
	useChinese := isChineseLang(lang)
	out := make([]string, 0, len(hit))
	// AllCaveatFamilies gives stable order via the registry's
	// declaration sequence (map iteration in Go is randomized, but
	// the registry's All accessor returns in registration order via
	// its slice mirror — see types/violation_registry.go).
	for _, fam := range types.AllCaveatFamilies() {
		if !hit[fam.ID] {
			continue
		}
		body := fam.EN
		if useChinese {
			body = fam.ZH
		}
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		out = append(out, body)
		if len(out) >= MaxMaterializedCaveats {
			break
		}
	}
	return out
}

// AppendUserCaveatsToAnswer renders materialized caveats as a
// trailing markdown section appended to the answer text. Replaces
// prependFailLoudWarning's hostile header pattern: caveats sit at
// the END of the answer (after the body the user just read), in
// natural language, with no internal jargon.
//
// The heading text is "补充说明：" (ZH) / "Additional notes:" (EN) —
// deliberately different from the answer document's own LLM-
// authored "**说明**：" / "**Caveats:**" section so the two channels
// remain visually distinguishable. LLM-authored caveats describe
// content gaps the LLM identified; system caveats describe
// orchestration constraints the retry loop could not resolve.
//
// If MaterializeUnresolvedViolationsAsCaveats returns no caveats
// (all violations are operator-only telemetry, or the input is
// empty), the answer is returned unchanged.
func AppendUserCaveatsToAnswer(answer string, violations []types.Violation, lang string) string {
	caveats := MaterializeUnresolvedViolationsAsCaveats(violations, lang)
	if len(caveats) == 0 {
		return answer
	}
	heading := "**Additional notes:**"
	if isChineseLang(lang) {
		heading = "**补充说明：**"
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(answer, "\n"))
	b.WriteString("\n\n")
	b.WriteString(heading)
	b.WriteString("\n\n")
	for _, c := range caveats {
		b.WriteString("- ")
		b.WriteString(c)
		b.WriteString("\n")
	}
	return b.String()
}

// AppendSystemCaveatString renders ONE pre-formatted system caveat
// as a trailing markdown section appended to the answer text. Same
// channel as AppendUserCaveatsToAnswer (the "**补充说明：**" /
// "**Additional notes:**" heading) but bypasses the
// MaterializeUnresolvedViolationsAsCaveats Violation→template
// path: callers that already have a final-form caveat string
// (e.g. P6's softFinalizeRepairCapMessage) write it directly.
//
// Why this matters (P7, 2026-05-10): the AnswerDocumentV2.Caveats[]
// channel is for LLM-authored caveats — content gaps the LLM
// itself identified. System-injected caveats (orchestrator decisions
// about repair-loop termination, scope boundaries, etc.) MUST NOT
// pollute the LLM-authored slot — that violates
// feedback_no_system_backfill_to_user_panel. They go through this
// system-side helper instead, keeping the two channels visually
// distinguishable in the rendered answer.
//
// Empty / whitespace-only caveat returns the answer unchanged. Use
// AppendUserCaveatsToAnswer when the input is a list of
// types.Violation; use this when the input is a single already-
// composed user-facing string.
func AppendSystemCaveatString(answer, caveat, lang string) string {
	caveat = strings.TrimSpace(caveat)
	if caveat == "" {
		return answer
	}
	heading := "**Additional notes:**"
	if isChineseLang(lang) {
		heading = "**补充说明：**"
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(answer, "\n"))
	b.WriteString("\n\n")
	b.WriteString(heading)
	b.WriteString("\n\n")
	b.WriteString("- ")
	b.WriteString(caveat)
	b.WriteString("\n")
	return b.String()
}

// isChineseLang accepts the same set of variants the renderer
// already recognises in answerDocumentRequiresChinese (the canonical
// gate is in tool/emit_answer_document.go). Duplicated here as a
// pure helper so the materializer has no cross-package import on
// the tool layer.
func isChineseLang(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese":
		return true
	case "":
		// Default project language is zh — see config defaultLang.
		// Empty input most commonly means BusContext.Language was
		// never populated, which currently corresponds to the zh
		// default.
		return true
	}
	return false
}
