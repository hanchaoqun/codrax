package render

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Common renderer helpers shared by V2 (and previously V1) answer
// document rendering. B8-T2 (block_only_carrier.md §5.8, 2026-05-03)
// extracted these out of the deleted answerdoc.go (V1 renderer) so
// the citation display + lang normalisation stay available to V2.

// answerDocLang is the bilingual switch the renderer uses to pick
// English vs Chinese labels.
type answerDocLang int

const (
	answerDocLangEN answerDocLang = iota
	answerDocLangZH
)

// normalizeAnswerDocLang maps user lang strings to the renderer enum.
// Empty / unknown values fall back to English.
func normalizeAnswerDocLang(lang string) answerDocLang {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese", "简体中文":
		return answerDocLangZH
	}
	return answerDocLangEN
}

// renderCitationDisplay produces the per-scope display string for a
// single Citation. Centralised so the V2 citation pool, V2 inline
// references, and any future rendering surface (markdown table, dock
// summary) share one definition.
func renderCitationDisplay(c types.Citation) string {
	var head string
	switch c.Scope {
	case types.ScopeLineRange:
		if c.LineEnd > c.Line {
			head = fmt.Sprintf("`%s:%d-%d`", c.File, c.Line, c.LineEnd)
		} else {
			head = fmt.Sprintf("`%s:%d`", c.File, c.Line)
		}
	case types.ScopeSection:
		if c.Line > 0 {
			head = fmt.Sprintf("`%s:%d` [section: `%s`]", c.File, c.Line, c.SectionPath)
		} else {
			head = fmt.Sprintf("`%s` [section: `%s`]", c.File, c.SectionPath)
		}
	case types.ScopeFile:
		head = fmt.Sprintf("`%s` [layer: %s]", c.File, c.FileRoleLabel)
	case types.ScopeCrossfile:
		summary := strings.TrimSpace(c.CrossfileSummary)
		if summary == "" {
			summary = "cross-file contract"
		}
		head = fmt.Sprintf("cross-file contract: %s", summary)
		if c.File != "" {
			head = fmt.Sprintf("%s (in `%s`)", head, c.File)
		}
	case types.ScopeNegative:
		pattern := strings.TrimSpace(c.NegativePattern)
		if pattern != "" {
			head = fmt.Sprintf("`%s` [absence: `%s`]", c.File, pattern)
		} else {
			head = fmt.Sprintf("`%s` [absence]", c.File)
		}
	default:
		// ScopeLine or empty (transitional).
		if c.Line > 0 {
			head = fmt.Sprintf("`%s:%d`", c.File, c.Line)
		} else {
			head = fmt.Sprintf("`%s`", c.File)
		}
	}
	// Append the auto-derived enclosing-function suffix when the cite
	// resolved to a function-internal line. EnclosingFunction is set
	// at emit time by enrichCitationsWithEnclosingFunction (graph
	// lookup) so the renderer can rely on it without re-querying
	// the graph here. Empty for module-level / unresolved cites —
	// nothing is appended in that case.
	if fn := strings.TrimSpace(c.EnclosingFunction); fn != "" {
		head = head + fmt.Sprintf(" (in `%s`)", fn)
	}
	if q := strings.TrimSpace(c.Quote); q != "" {
		return head + " — " + q
	}
	return head
}
