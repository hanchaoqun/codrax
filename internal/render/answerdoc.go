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
// Design contract (P2.2 remediation): the finalizer LLM emits a
// typed AnswerDocument; this
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
		// Summary already rendered above. 2026-04-17: explanation
		// shape now optionally carries an anchor-symbol skeleton for
		// multi-topic answers (one symbol per sub-topic). When
		// present, render it as a Key Anchors block between the
		// summary and the citation pool so the reader sees the
		// load-bearing identifiers each topic hangs on.
		if len(doc.Symbols) > 0 {
			renderAnswerDocExplanationSkeleton(&b, doc, l)
		}
	default:
		// ShapeNone / empty / unknown: degrade gracefully to the
		// explanation path so the user at least sees Summary and
		// citations if any.
	}

	// Render-only Key snippets block (deterministic, built by
	// emit_answer_document from read_file gutter at each cited line).
	// Rendered between shape-specific body and citation pool so the
	// order is: prose → structured payload → code snippets → citations.
	//
	// Skipped for scalar shapes (value / boolean / config_value) —
	// those shapes have a hard character cap in contract.checkShape
	// ("value answer too long" etc.) and appending snippets blows
	// the cap, retrying into the fail-loud banner even when the
	// literal answer is correct. Snippets belong to shapes whose
	// contract allows multi-paragraph prose (list_of_symbols /
	// step_list / explanation). Also skipped when empty
	// (non-read-backed answers).
	switch doc.Shape {
	case types.ShapeValue, types.ShapeBoolean, types.ShapeConfigValue:
		// scalar shapes: render nothing extra
	default:
		if len(doc.Snippets) > 0 {
			renderAnswerDocSnippets(&b, doc, l)
		}
	}

	// Citation pool renders last for explanation / unknown-shape
	// fall-throughs. Structured shapes (list_of_symbols / step_list
	// / value / config_value / boolean) inline their citations at
	// the payload level.
	switch doc.Shape {
	case types.ShapeListOfSymbols, types.ShapeStepList,
		types.ShapeValue, types.ShapeConfigValue, types.ShapeBoolean:
		// payload renderer already inlined citations
	default:
		renderAnswerDocCitationPool(&b, doc, l)
	}

	// Completeness and caveats: render as a quiet footer separated
	// from the main answer by a blank line. Keeps the answer body
	// clean and puts metadata where it belongs — at the bottom.
	var footer []string
	if doc.Shape == types.ShapeListOfSymbols {
		if tag := completenessTag(doc.SymbolsCompleteness, l); tag != "" {
			footer = append(footer, tag)
		}
	}
	for _, c := range doc.Caveats {
		if t := strings.TrimSpace(c); t != "" {
			footer = append(footer, t)
		}
	}
	if len(footer) > 0 {
		b.WriteString("\n---\n")
		for _, f := range footer {
			fmt.Fprintf(&b, "*%s*\n", f)
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
	if len(doc.Symbols) == 0 {
		return
	}
	// Render as a markdown table for scanability. Tables are
	// visually denser than bullet lists and let the reader quickly
	// find a symbol by scanning the Name column.
	switch lang {
	case answerDocLangZH:
		b.WriteString("| 名称 | 位置 | 说明 |\n")
	default:
		b.WriteString("| Name | Location | Description |\n")
	}
	b.WriteString("|---|---|---|\n")
	for _, s := range doc.Symbols {
		name := tableCell("**" + s.Name + "**")
		loc := ""
		if s.File != "" && s.Line > 0 {
			loc = tableCell(fmt.Sprintf("`%s:%d`", s.File, s.Line))
		} else if s.File != "" {
			loc = tableCell(fmt.Sprintf("`%s`", s.File))
		}
		desc := tableCell(s.Rationale)
		fmt.Fprintf(b, "| %s | %s | %s |\n", name, loc, desc)
	}
}

// -------- Shape: explanation (anchor skeleton) --------

// renderAnswerDocExplanationSkeleton renders the optional Key Anchors
// block for multi-topic explanation answers. Each anchor is one
// symbol + file:line + rationale, laid out bullet-style so the reader
// can jump from the summary prose to the concrete code location.
// Invoked only when doc.Symbols is non-empty on ShapeExplanation.
func renderAnswerDocExplanationSkeleton(b *strings.Builder, doc *types.AnswerDocument, lang answerDocLang) {
	if len(doc.Symbols) == 0 {
		return
	}
	// Render key anchors as a compact table for quick reference.
	switch lang {
	case answerDocLangZH:
		b.WriteString("\n**关键锚点**：\n\n")
		b.WriteString("| 组件 | 位置 | 作用 |\n")
	default:
		b.WriteString("\n**Key anchors:**\n\n")
		b.WriteString("| Component | Location | Role |\n")
	}
	b.WriteString("|---|---|---|\n")
	for _, s := range doc.Symbols {
		name := tableCell("`" + s.Name + "`")
		loc := ""
		if s.File != "" && s.Line > 0 {
			loc = tableCell(fmt.Sprintf("`%s:%d`", s.File, s.Line))
		} else if s.File != "" {
			loc = tableCell(fmt.Sprintf("`%s`", s.File))
		}
		desc := tableCell(s.Rationale)
		fmt.Fprintf(b, "| %s | %s | %s |\n", name, loc, desc)
	}
}

// -------- Shape: step_list --------

func renderAnswerDocStepList(b *strings.Builder, doc *types.AnswerDocument, lang answerDocLang) {
	if len(doc.Steps) == 0 {
		return
	}
	// Detailed numbered list with file:line citations. Previously the
	// renderer also auto-generated a "Flow overview" ASCII chain from
	// steps[], but that shape assumed steps are linear — when the
	// answer's step topology is a DAG (branches / parallel / fan-out),
	// the auto-chain misrepresented the structure. The LLM now owns
	// non-linear diagrams via the answer-document-skill, placing them
	// in Summary where the full Unicode palette survives.
	switch lang {
	case answerDocLangZH:
		b.WriteString("**详细步骤**：\n\n")
	default:
		b.WriteString("**Detailed steps:**\n\n")
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
			fmt.Fprintf(b, "答案是 `%s`", v.Literal)
		}
	default:
		if isConfig && v.Key != "" {
			fmt.Fprintf(b, "The config key `%s` resolves to `%s`", v.Key, v.Literal)
		} else {
			fmt.Fprintf(b, "The answer is `%s`", v.Literal)
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

// -------- Code snippets (render-only, added session 8) --------

// renderAnswerDocSnippets emits a "Key snippets" / "关键代码" block
// showing the clustered code excerpts doc.Snippets carries. Each
// snippet is a language-tagged markdown fence header-lined with
// `file:line-line`. Empty when no snippets — caller already gated
// on len(doc.Snippets) > 0 before calling.
func renderAnswerDocSnippets(b *strings.Builder, doc *types.AnswerDocument, lang answerDocLang) {
	switch lang {
	case answerDocLangZH:
		b.WriteString("\n**关键代码**：\n\n")
	default:
		b.WriteString("\n**Key snippets:**\n\n")
	}
	for _, s := range doc.Snippets {
		if s.StartLine == s.EndLine {
			fmt.Fprintf(b, "`%s:%d`\n", s.File, s.StartLine)
		} else {
			fmt.Fprintf(b, "`%s:%d-%d`\n", s.File, s.StartLine, s.EndLine)
		}
		if tag := strings.TrimSpace(s.Language); tag != "" {
			fmt.Fprintf(b, "```%s\n", tag)
		} else {
			b.WriteString("```\n")
		}
		b.WriteString(s.Code)
		if !strings.HasSuffix(s.Code, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString("```\n\n")
	}
}

// completenessTag returns a short italic tag for the list_of_symbols
// completeness claim, rendered in the footer. Returns empty for
// CompletenessComplete (no annotation needed — the answer speaks for
// itself).
func completenessTag(claim types.CompletenessClaim, lang answerDocLang) string {
	switch claim {
	case types.CompletenessLowerBound:
		switch lang {
		case answerDocLangZH:
			return "以上为已确认信息，可能还有其他符合条件的信息"
		default:
			return "confirmed items listed above; more may exist"
		}
	case types.CompletenessUnknown:
		switch lang {
		case answerDocLangZH:
			return "以上为候选信息，非权威列表"
		default:
			return "candidate items above; non-authoritative"
		}
	}
	// CompletenessComplete: no tag — a complete answer needs no qualifier.
	return ""
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

// tableCell sanitises a string for safe use inside a markdown table
// cell. The three things that break a table row are embedded
// newlines (split the row), literal `|` characters (split a column
// mid-cell), and leading/trailing whitespace (break the header
// separator alignment). Newlines become spaces, `|` becomes `\|`
// (markdown escape), and consecutive whitespace is collapsed so a
// long rationale with internal line breaks doesn't blow the row
// up. Empty input returns empty.
func tableCell(s string) string {
	if s == "" {
		return ""
	}
	// Normalise every whitespace variant (newline, tab, CR, NBSP) to
	// a single space before collapsing duplicates. Avoids an LLM-
	// emitted tab or U+00A0 leaking through strings.Fields.
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch r {
		case '\n', '\r', '\t', '\v', '\f', '\u00A0':
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		case '|':
			b.WriteString(`\|`)
			prevSpace = false
		case ' ':
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

