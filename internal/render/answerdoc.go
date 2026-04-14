package render

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answerdoc.go — P2.2 deterministic AnswerDocument → prose renderer.
//
// This file is independent of the CLI Renderer struct (renderer.go).
// Both live in package render for organizational reasons: the CLI
// Renderer handles streaming event output, and RenderAnswerDocument
// handles one-shot final-answer composition. They share no state and
// can be read, tested, and modified in isolation.
//
// Design contract (docs/architecture-root-cause-remediation.md §6
// P2.2): the finalizer LLM emits a typed AnswerDocument; this
// renderer is the ONLY path by which that struct becomes user-visible
// prose. Every per-shape template is a Go string builder — no Go
// text/template or html/template, no locale library, no plugin
// system. The goal is that a reviewer can grep `shape=step_list`
// here and read the exact prose the user will see.
//
// Language selection:
//
//   - "zh" / "zh-CN" / "zh-cn" / "cn" / "chinese" → Simplified Chinese
//   - "en" / "en-US" / "english" / "" / "off" / "none" → English
//   - any other value → English (safe fallback)
//
// The language selector mirrors orchestrator.languageDirective so a
// flag=on session's answers come out in the same language the
// legacy finalizer would have produced.

// RenderAnswerDocument converts an AnswerDocument into user-visible
// prose in the specified language. The returned string is the full
// final-answer body that the finalizer's ParseOutput stores in
// StageOutput.FinalAnswer.
//
// A nil document returns an empty string — the caller is responsible
// for treating that as a retry or fallback condition.
//
// The renderer never reads any state outside the document itself;
// all per-shape content is assembled from doc.Steps / doc.Symbols /
// etc. and citations are resolved through doc.Citations[CitationRef].
// This is the structural invariant that makes Patterns 1-4
// impossible: a step without a cited line references a Citation that
// was itself validated by emit_answer_document, and the renderer
// cannot emit a line that was not in the pool.
func RenderAnswerDocument(doc *types.AnswerDocument, lang string) string {
	if doc == nil {
		return ""
	}
	l := normalizeAnswerDocLang(lang)

	var b strings.Builder

	// Optional summary lead-in. Rendered for every shape when set,
	// since it is the one LLM-authored prose slot and carries the
	// natural-language framing the user expects.
	if s := strings.TrimSpace(doc.Summary); s != "" {
		b.WriteString(s)
		b.WriteString("\n\n")
	}

	switch doc.Shape {
	case types.ShapeListOfSymbols:
		renderAnswerDocListOfSymbols(&b, doc, l)
	case types.ShapeStepList:
		renderAnswerDocStepList(&b, doc, l)
	case types.ShapeValue:
		renderAnswerDocValue(&b, doc, l, false)
	case types.ShapeConfigValue:
		renderAnswerDocValue(&b, doc, l, true)
	case types.ShapeBoolean:
		renderAnswerDocBoolean(&b, doc, l)
	case types.ShapeExplanation:
		// Summary already rendered above; explanation shape has no
		// further structural content beyond the citation pool.
		renderAnswerDocCitationPool(&b, doc, l)
	default:
		// ShapeNone / empty / unknown: degrade gracefully to the
		// explanation path so the user at least sees Summary and
		// citations if any.
		renderAnswerDocCitationPool(&b, doc, l)
	}

	if len(doc.Caveats) > 0 {
		b.WriteString("\n")
		b.WriteString(answerDocCaveatsHeader(l))
		b.WriteString("\n")
		for _, c := range doc.Caveats {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(c))
		}
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// answerDocLang is the normalized internal locale identifier. Only
// two values exist today; adding a new language means adding a case
// here and an entry in every answerDoc* helper below. Doing both at
// the same grep site is the point of the enum.
type answerDocLang int

const (
	answerDocLangEN answerDocLang = iota
	answerDocLangZH
)

func normalizeAnswerDocLang(lang string) answerDocLang {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese", "简体中文":
		return answerDocLangZH
	}
	return answerDocLangEN
}

// -------- Shape: list_of_symbols --------

func renderAnswerDocListOfSymbols(b *strings.Builder, doc *types.AnswerDocument, lang answerDocLang) {
	// Header varies by completeness claim so the reader can see at a
	// glance whether the slate is authoritative, a floor, or tentative.
	switch doc.SymbolsCompleteness {
	case types.CompletenessComplete:
		switch lang {
		case answerDocLangZH:
			b.WriteString("**完整答案**：\n\n")
		default:
			b.WriteString("**Complete answer:**\n\n")
		}
	case types.CompletenessLowerBound:
		switch lang {
		case answerDocLangZH:
			b.WriteString("**至少包含以下符号**（可能还有其他符合条件的符号）：\n\n")
		default:
			b.WriteString("**At least the following symbols** (more may exist):\n\n")
		}
	default:
		// CompletenessUnknown or empty: the finalizer should not
		// normally reach this path — the evaluator's cardinality
		// cross-check would have downgraded to lower_bound — but if
		// it does, render the symbols without an authority claim so
		// the user sees the data instead of a blank.
		switch lang {
		case answerDocLangZH:
			b.WriteString("**候选符号**（非权威列表）：\n\n")
		default:
			b.WriteString("**Candidate symbols** (non-authoritative):\n\n")
		}
	}

	for _, s := range doc.Symbols {
		if s.File != "" && s.Line > 0 {
			fmt.Fprintf(b, "- **%s** (`%s:%d`)", s.Name, s.File, s.Line)
		} else if s.File != "" {
			fmt.Fprintf(b, "- **%s** (`%s`)", s.Name, s.File)
		} else {
			fmt.Fprintf(b, "- **%s**", s.Name)
		}
		if r := strings.TrimSpace(s.Rationale); r != "" {
			fmt.Fprintf(b, " — %s", r)
		}
		b.WriteString("\n")
	}
}

// -------- Shape: step_list --------

func renderAnswerDocStepList(b *strings.Builder, doc *types.AnswerDocument, lang answerDocLang) {
	switch lang {
	case answerDocLangZH:
		b.WriteString("**步骤**：\n\n")
	default:
		b.WriteString("**Steps:**\n\n")
	}
	for _, step := range doc.Steps {
		fmt.Fprintf(b, "%d. %s", step.Index, strings.TrimSpace(step.Description))
		if cite := lookupCitation(doc, step.CitationRef); cite != nil {
			fmt.Fprintf(b, " [`%s:%d`]", cite.File, cite.Line)
		}
		b.WriteString("\n")
	}
}

// -------- Shape: value + config_value --------

func renderAnswerDocValue(b *strings.Builder, doc *types.AnswerDocument, lang answerDocLang, isConfig bool) {
	if doc.Value == nil {
		return
	}
	v := doc.Value
	switch lang {
	case answerDocLangZH:
		if isConfig && v.Key != "" {
			fmt.Fprintf(b, "配置项 `%s` 的值为 `%s`", v.Key, v.Literal)
		} else {
			fmt.Fprintf(b, "值为 `%s`", v.Literal)
		}
	default:
		if isConfig && v.Key != "" {
			fmt.Fprintf(b, "The config key `%s` resolves to `%s`", v.Key, v.Literal)
		} else {
			fmt.Fprintf(b, "The value is `%s`", v.Literal)
		}
	}
	if cite := lookupCitation(doc, v.CitationRef); cite != nil {
		fmt.Fprintf(b, " (`%s:%d`)", cite.File, cite.Line)
	}
	b.WriteString(".\n")
}

// -------- Shape: boolean --------

func renderAnswerDocBoolean(b *strings.Builder, doc *types.AnswerDocument, lang answerDocLang) {
	if doc.Boolean == nil {
		return
	}
	bl := doc.Boolean
	var lead string
	switch lang {
	case answerDocLangZH:
		if bl.Decision {
			lead = "**是**"
		} else {
			lead = "**否**"
		}
	default:
		if bl.Decision {
			lead = "**YES**"
		} else {
			lead = "**NO**"
		}
	}
	fmt.Fprintf(b, "%s — %s", lead, strings.TrimSpace(bl.Rationale))
	if cite := lookupCitation(doc, bl.CitationRef); cite != nil {
		fmt.Fprintf(b, " (`%s:%d`)", cite.File, cite.Line)
	}
	b.WriteString("\n")
}

// -------- Shared: citation pool footer --------

// renderAnswerDocCitationPool emits a bulleted list of every citation
// in the pool. Called by the explanation / fallback path. Shape-
// specific renderers do NOT call this — they resolve citations
// inline via lookupCitation — so there is exactly one source of
// citation rendering per shape.
func renderAnswerDocCitationPool(b *strings.Builder, doc *types.AnswerDocument, lang answerDocLang) {
	if len(doc.Citations) == 0 {
		return
	}
	switch lang {
	case answerDocLangZH:
		b.WriteString("\n**引用**：\n\n")
	default:
		b.WriteString("\n**Citations:**\n\n")
	}
	for _, c := range doc.Citations {
		if q := strings.TrimSpace(c.Quote); q != "" {
			fmt.Fprintf(b, "- `%s:%d` — %s\n", c.File, c.Line, q)
		} else {
			fmt.Fprintf(b, "- `%s:%d`\n", c.File, c.Line)
		}
	}
}

// answerDocCaveatsHeader returns the per-language header for the
// caveats footer. Isolated so adding a new language means adding one
// case here, not scanning Render* bodies.
func answerDocCaveatsHeader(lang answerDocLang) string {
	switch lang {
	case answerDocLangZH:
		return "**注意事项**："
	default:
		return "**Caveats:**"
	}
}

// lookupCitation resolves a CitationRef integer against the
// document's citation pool. Returns nil when ref == CitationRefUnset
// (-1), when ref is out of range, or when the pool is empty. The
// tool-layer schema validator guarantees that a non-unset ref is
// always in range, so out-of-range here only happens under programmer
// error and degrades silently (the step / value / boolean line is
// rendered without a citation suffix).
func lookupCitation(doc *types.AnswerDocument, ref int) *types.Citation {
	if ref == types.CitationRefUnset {
		return nil
	}
	if ref < 0 || ref >= len(doc.Citations) {
		return nil
	}
	c := doc.Citations[ref]
	return &c
}
