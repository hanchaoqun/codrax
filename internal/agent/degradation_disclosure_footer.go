package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EVALFIX-2E (CLASS 5, 2026-07-30,
// docs/design/evalfix2_class_designs_20260730.md CLASS 5) — the
// user-facing degradation-disclosure footer.
//
// The typed degradation ledger (internal/types/degradation_ledger.go)
// accounts every deliberate fail-open action the system performed on
// the answer's semantics (citation-quote rewrites, richness hard→soft
// facet softening, completeness downgrade). This footer is the single
// user-facing outlet: AT MOST ONE grouped system line, rendered ONLY at
// the last-mile supplement chokepoint
// (renderAnswerDocumentWithLastMileSupplements — TRUNC family pin), so
// every FinalAnswer path (first draft, auto-repair, recovery) carries
// it identically. 0 entries → 0 bytes: healthy runs stay byte-stable.
//
// 系统不可代替 LLM 写用户面板答案 red line: this is a SYSTEM disclosure
// line in the same precedent family as the degraded footer / authority
// caveat — a render-time transform that never backfills answer content
// and never touches the archived document on Mutable.

// degradationFooterEnabled gates the user-facing footer. Function
// indirection mirrors orchestrator.SetRichnessSofteningWarnEnabled so
// this package takes no import on cmd. Default true; the
// codrax.yaml :: pipeline_degradation_footer knob threads through
// SetDegradationFooterEnabled. Turning the footer off does NOT stop the
// ledger or the WARN logs — telemetry is always recorded.
var degradationFooterEnabled = func() bool { return true }

// SetDegradationFooterEnabled is wired by cmd/root.go to thread the
// pipeline_degradation_footer yaml knob.
func SetDegradationFooterEnabled(enabled bool) {
	degradationFooterEnabled = func() bool { return enabled }
}

// Generic fallback words for a ledger lane missing from
// DegradationLaneRegistry (defensive arm). degradedSectionDisplayNames
// discipline: internal tokens never ride a user surface, and the
// renderer never invents a specific name for an unknown lane — the
// count is kept, the wording falls open to the generic pair below.
const (
	degradationLaneGenericZH = "确定性降级处理"
	degradationLaneGenericEN = "deterministic degradation"
)

// renderDegradationDisclosureFooter builds the single grouped footer
// line from the merged ledger view. Deterministic rules:
//   - only Class==answer_semantics entries with Count>0 participate;
//     plumbing lanes NEVER render here (log-only forever);
//   - grouping is per lane (registry declaration order), never
//     per-event; unregistered lanes merge into one generic group;
//   - 0 participating entries → "" (caller appends nothing, so the
//     healthy path stays byte-identical);
//   - wording comes ONLY from the registry ZH/EN pairs, chosen by the
//     session language (zh default, en on "en" prefix — same predicate
//     as degradedDeterministicSectionsCaveat).
func renderDegradationDisclosureFooter(ctx *types.AgentContext, lang string) string {
	if !degradationFooterEnabled() || ctx == nil || ctx.Mutable == nil {
		return ""
	}
	entries := types.BuildDegradationLedgerView(ctx.Mutable)
	if len(entries) == 0 {
		return ""
	}
	en := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "en")
	type footerGroup struct {
		label string
		count int
	}
	var groups []footerGroup
	genericIdx := -1
	for _, entry := range entries {
		if entry.Count <= 0 {
			continue
		}
		spec, registered := types.DegradationLaneRegistry[entry.Lane]
		if registered {
			if spec.Class != types.ClassAnswerSemantics {
				continue // plumbing: registered, permanently log-only
			}
			label := spec.ZH
			if en {
				label = spec.EN
			}
			groups = append(groups, footerGroup{label: label, count: entry.Count})
			continue
		}
		// Defensive arm: unregistered lane → generic word, count kept,
		// all unregistered lanes merged into one group.
		if genericIdx >= 0 {
			groups[genericIdx].count += entry.Count
			continue
		}
		label := degradationLaneGenericZH
		if en {
			label = degradationLaneGenericEN
		}
		genericIdx = len(groups)
		groups = append(groups, footerGroup{label: label, count: entry.Count})
	}
	if len(groups) == 0 {
		return ""
	}
	parts := make([]string, 0, len(groups))
	for _, g := range groups {
		parts = append(parts, fmt.Sprintf("%s ×%d", g.label, g.count))
	}
	if en {
		return "System degradation disclosure: " + strings.Join(parts, ", ") + " (full detail in run logs)"
	}
	return "系统降级披露：" + strings.Join(parts, "、") + "（完整明细见运行日志）"
}
