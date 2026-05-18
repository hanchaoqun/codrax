package orchestrator

import (
	"fmt"
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
	return appendSystemCaveatBullets(answer, caveats, lang)
}

// AppendSoftContractCaveatsToAnswer appends user-facing caveats only
// for soft root violations under the current runtime policy.
//
// This is the accept-path companion to FilterActionableRootViolations:
// default-soft contract concerns should not burn another finalizer
// retry, but if the registry has a CaveatFamilyID the shipped answer
// should still tell the user about the residual boundary. Operator
// promotion via pipeline_contract_strict_kinds removes the kind from
// this soft set, keeping promoted gates actionable.
func AppendSoftContractCaveatsToAnswer(answer string, violations []types.Violation, lang string) string {
	return AppendUserCaveatsToAnswer(answer, softContractCaveatViolations(violations), lang)
}

func softContractCaveatViolations(violations []types.Violation) []types.Violation {
	roots := FilterDerivedViolations(violations)
	if len(roots) == 0 {
		return nil
	}
	out := make([]types.Violation, 0, len(roots))
	for _, v := range roots {
		if !isSoftViolationKind(v.Kind) {
			continue
		}
		out = append(out, v)
	}
	return out
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
	return appendSystemCaveatBullets(answer, []string{caveat}, lang)
}

// AppendResidualConcernDetailsToAnswer renders the finalize-repair
// hard-cap disclosure. Unlike AppendUserCaveatsToAnswer, this path
// intentionally preserves per-violation detail from the typed
// contract/reviewer results so "N residual concern(s)" is followed
// by the actual unresolved items. It still stays in system-caveat
// vocabulary: no ViolKind names, no IR fields, no confidence scores.
func AppendResidualConcernDetailsToAnswer(answer string, violations []types.Violation, lang string) string {
	bullets := MaterializeResidualConcernDetails(violations, lang)
	if len(bullets) == 0 {
		return answer
	}
	return appendSystemCaveatBullets(answer, bullets, lang)
}

// MaterializeResidualConcernDetails converts unresolved violations
// into the detailed variant used only when the finalize-stage repair
// hard cap is reached. Inputs are typed system/model outputs; this
// helper does not infer answer facts or repair missing answer content.
func MaterializeResidualConcernDetails(violations []types.Violation, lang string) []string {
	if len(violations) == 0 {
		return nil
	}
	useChinese := isChineseLang(lang)
	limit := len(violations)
	if limit > MaxMaterializedCaveats {
		limit = MaxMaterializedCaveats
	}
	out := make([]string, 0, limit+1)
	if useChinese {
		if len(violations) > limit {
			out = append(out, fmt.Sprintf("质量审阅仍有 %d 项未完全解决，以下列出前 %d 项可见边界。", len(violations), limit))
		} else {
			out = append(out, fmt.Sprintf("质量审阅仍有 %d 项未完全解决，以下列出可见边界。", len(violations)))
		}
	} else {
		if len(violations) > limit {
			out = append(out, fmt.Sprintf("Quality review still has %d unresolved concern(s); showing the first %d visible boundary item(s).", len(violations), limit))
		} else {
			out = append(out, fmt.Sprintf("Quality review still has %d unresolved concern(s); the visible boundary item(s) are listed below.", len(violations)))
		}
	}

	seen := map[string]bool{}
	for _, v := range violations {
		if len(out) > limit {
			break
		}
		line := residualConcernLine(v, useChinese)
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	if len(out) == 1 {
		for _, c := range MaterializeUnresolvedViolationsAsCaveats(violations, lang) {
			c = strings.TrimSpace(c)
			if c == "" || seen[c] {
				continue
			}
			seen[c] = true
			out = append(out, c)
			if len(out) > limit {
				break
			}
		}
	}
	if len(out) == 1 {
		return nil
	}
	return out
}

func residualConcernLine(v types.Violation, useChinese bool) string {
	switch v.Kind {
	case types.ViolMustInclude:
		if term := residualClusterValue(v.ClusterKey, "term"); term != "" {
			if useChinese {
				return "答案仍需明确提及 " + inlineCode(term) + "。"
			}
			return "The answer still needs to explicitly mention " + inlineCode(term) + "."
		}
	case types.ViolAnswerSemanticUnderfilled, types.ViolAnswerTopicMismatch:
		topic := residualClusterValue(v.ClusterKey, "topic")
		observation := residualObservation(v.Detail)
		suggestion := residualSuggestion(v.Repair)
		if topic != "" || observation != "" || suggestion != "" {
			return residualTopicLine(topic, observation, suggestion, useChinese)
		}
	}

	subject := firstResidualClusterValue(v.ClusterKey, "term", "symbol", "topic", "label", "file", "path", "block", "block_kind", "relation")
	if subject != "" {
		if useChinese {
			return "与 " + inlineCode(subject) + " 相关的检查仍需补充验证。"
		}
		return "The check related to " + inlineCode(subject) + " still needs additional verification."
	}
	if caveats := MaterializeUnresolvedViolationsAsCaveats([]types.Violation{v}, langFromChinese(useChinese)); len(caveats) > 0 {
		return caveats[0]
	}
	return ""
}

func residualTopicLine(topic, observation, suggestion string, useChinese bool) string {
	subject := strings.TrimSpace(topic)
	body := strings.TrimSpace(suggestion)
	if observation != "" {
		body = observation
	}
	if subject == "" {
		if useChinese {
			return "仍有一处答案覆盖问题：" + trimSentence(body)
		}
		return "One answer-coverage concern remains: " + trimSentence(body)
	}
	if body == "" {
		if useChinese {
			return "关于 " + inlineCode(subject) + " 的覆盖仍需补充。"
		}
		return "Coverage for " + inlineCode(subject) + " still needs to be completed."
	}
	if useChinese {
		return "关于 " + inlineCode(subject) + "：" + trimSentence(body)
	}
	return "For " + inlineCode(subject) + ": " + trimSentence(body)
}

func residualClusterValue(clusterKey, key string) string {
	prefix := strings.TrimSpace(key) + ":"
	for _, part := range strings.Split(clusterKey, "|") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(part, prefix))
		}
	}
	return ""
}

func firstResidualClusterValue(clusterKey string, keys ...string) string {
	for _, key := range keys {
		if v := residualClusterValue(clusterKey, key); v != "" {
			return v
		}
	}
	return ""
}

func residualObservation(detail string) string {
	const marker = "observation:"
	idx := strings.Index(detail, marker)
	if idx < 0 {
		return ""
	}
	return trimSentence(detail[idx+len(marker):])
}

func residualSuggestion(repair string) string {
	repair = strings.TrimSpace(repair)
	if repair == "" {
		return ""
	}
	if idx := strings.Index(repair, "Reviewer rationale:"); idx >= 0 {
		repair = strings.TrimSpace(repair[:idx])
	}
	if idx := strings.Index(repair, ". "); idx >= 0 && idx+2 < len(repair) {
		repair = strings.TrimSpace(repair[idx+2:])
	}
	return trimSentence(repair)
}

func trimSentence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, " \t\r\n—-")
	return s
}

func inlineCode(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "`", "")
	return "`" + s + "`"
}

func langFromChinese(useChinese bool) string {
	if useChinese {
		return "zh"
	}
	return "en"
}

func appendSystemCaveatBullets(answer string, caveats []string, lang string) string {
	filtered := make([]string, 0, len(caveats))
	seen := map[string]bool{}
	for _, caveat := range caveats {
		caveat = strings.TrimSpace(caveat)
		if caveat == "" || seen[caveat] {
			continue
		}
		seen[caveat] = true
		filtered = append(filtered, caveat)
	}
	if len(filtered) == 0 {
		return answer
	}
	heading := "**Additional notes:**"
	if isChineseLang(lang) {
		heading = "**补充说明：**"
	}
	base := strings.TrimRight(answer, "\n")
	var b strings.Builder
	b.WriteString(base)
	if strings.Contains(base, heading) {
		b.WriteString("\n")
	} else {
		b.WriteString("\n\n")
		b.WriteString(heading)
		b.WriteString("\n\n")
	}
	for _, c := range filtered {
		b.WriteString("- ")
		b.WriteString(c)
		b.WriteString("\n")
	}
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
