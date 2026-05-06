package render

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RenderAnswerDocument renders an AnswerDocumentV2 (block-only
// carrier) into the user-visible markdown string. B5 落地. The
// renderer iterates blocks in declared order; each block kind has a
// dedicated helper. Citations + Snippets blocks render after the
// main blocks, mirroring V1's RenderAnswerDocument tail.
//
// Per docs/migration/block_only_carrier.md §5.5 design:
//   - NO shape switch — the renderer never reads doc.Shape (V2 has
//     none); it dispatches on per-block Kind.
//   - The renderer NEVER writes to the document — output is the
//     final string only. Per the feedback_no_system_backfill_to_user
//     _panel red line, we cannot mutate doc.Blocks even to upgrade
//     missing block IDs.
//   - Caveat / surface_role / claim_uses are read for display
//     decoration but never modified.
func RenderAnswerDocument(doc *types.AnswerDocumentV2, lang string) string {
	if doc == nil {
		return ""
	}
	docLang := normalizeAnswerDocLang(lang)
	var b strings.Builder

	for _, blk := range doc.Blocks {
		renderAnswerDocV2Block(&b, blk, doc, docLang)
	}

	if len(doc.MissingRequestedRoles) > 0 {
		renderAnswerDocV2MissingRequestedRoles(&b, doc.MissingRequestedRoles, docLang)
	}

	if len(doc.Caveats) > 0 {
		renderAnswerDocV2Caveats(&b, doc.Caveats, docLang)
	}

	// Reuse V1's citation pool + snippet renderers; both already
	// take their input by Citation / CodeSnippet slice — they don't
	// care about V1 vs V2 docs.
	if len(doc.Citations) > 0 {
		renderAnswerDocV2Citations(&b, doc.Citations, docLang)
	}
	if len(doc.Snippets) > 0 {
		renderAnswerDocV2Snippets(&b, doc.Snippets, docLang)
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// renderAnswerDocV2Block dispatches on block.Kind. Unknown / empty
// kind silently skips — schema validation (B3) already guarantees
// every block has a valid kind, so this branch is a defensive
// no-op only.
func renderAnswerDocV2Block(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	switch blk.Kind {
	case types.BlockSummary:
		renderV2BlockSummary(b, blk, lang)
	case types.BlockSection:
		renderV2BlockSection(b, blk, lang)
	case types.BlockOrderedList:
		renderV2BlockOrderedList(b, blk, doc, lang)
	case types.BlockBulletList:
		renderV2BlockBulletList(b, blk, doc, lang)
	case types.BlockScalar:
		renderV2BlockScalar(b, blk, doc, lang)
	case types.BlockDecision:
		renderV2BlockDecision(b, blk, doc, lang)
	case types.BlockTable:
		renderV2BlockTable(b, blk, doc, lang)
	case types.BlockDiagram:
		renderV2BlockDiagram(b, blk, lang)
	case types.BlockCaveat:
		renderV2BlockCaveat(b, blk, lang)
	}
}

func renderV2BlockSummary(b *strings.Builder, blk types.AnswerBlock, _ answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "## %s\n\n", blk.Title)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
}

func renderV2BlockSection(b *strings.Builder, blk types.AnswerBlock, lang answerDocLang) {
	heading := strings.TrimSpace(blk.Title)
	if heading == "" {
		// A Section without an explicit Title is rendered without a
		// heading line; the body still appears.
	} else {
		fmt.Fprintf(b, "### %s\n\n", heading)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	rendered := 0
	for _, it := range blk.Items {
		// Skip items that contribute no user-visible prose. Some LLM
		// emits attach typed claim annotations (items[i].claim_use)
		// without Label/Text — those are contract-layer signals, not
		// answer prose. Without this guard the renderer prints "- "
		// per signal-only item.
		s := renderV2BlockItem(it, nil, lang)
		if strings.TrimSpace(s) == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", s)
		rendered++
	}
	if rendered > 0 {
		b.WriteString("\n")
	}
}

func renderV2BlockOrderedList(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	idx := 0
	for _, it := range blk.Items {
		s := renderV2BlockItem(it, doc, lang)
		if strings.TrimSpace(s) == "" {
			continue
		}
		idx++
		fmt.Fprintf(b, "%d. %s\n", idx, s)
	}
	if idx > 0 {
		b.WriteString("\n")
	}
}

func renderV2BlockBulletList(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	rendered := 0
	for _, it := range blk.Items {
		s := renderV2BlockItem(it, doc, lang)
		if strings.TrimSpace(s) == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", s)
		rendered++
	}
	if rendered > 0 {
		b.WriteString("\n")
	}
}

func renderV2BlockScalar(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	prefix := "Value:"
	if lang == answerDocLangZH {
		prefix = "值："
	}
	literal := renderUserSurfaceText(blk.Text)
	if len(blk.Items) > 0 && literal == "" {
		// Scalar may use first item's Label as literal when Text is
		// empty (B3 schema accepts both shapes).
		literal = renderUserSurfaceText(blk.Items[0].Label)
	}
	if literal == "" {
		return
	}
	fmt.Fprintf(b, "**%s** `%s`", prefix, literal)
	cite := blockTopCitation(blk, doc)
	if cite != "" {
		fmt.Fprintf(b, " (%s)", cite)
	}
	b.WriteString("\n\n")
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "*%s*\n\n", blk.Title)
	}
}

func renderV2BlockDecision(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	prefix := "Decision:"
	if lang == answerDocLangZH {
		prefix = "结论："
	}
	body := renderUserSurfaceText(blk.Text)
	fmt.Fprintf(b, "**%s** %s", prefix, body)
	cite := blockTopCitation(blk, doc)
	if cite != "" {
		fmt.Fprintf(b, " (%s)", cite)
	}
	b.WriteString("\n\n")
}

func renderV2BlockTable(b *strings.Builder, blk types.AnswerBlock, _ *types.AnswerDocumentV2, _ answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	if len(blk.Items) == 0 {
		return
	}
	// Two-column rendering: Label | Text. More elaborate column
	// shapes are postponed to a later refinement; the V2 schema
	// already gives Label + Text + CitationRef on each item.
	b.WriteString("| Item | Detail |\n|---|---|\n")
	for _, it := range blk.Items {
		label := strings.TrimSpace(it.Label)
		text := strings.TrimSpace(it.Text)
		fmt.Fprintf(b, "| %s | %s |\n", escapePipe(label), escapePipe(text))
	}
	b.WriteString("\n")
}

func renderV2BlockDiagram(b *strings.Builder, blk types.AnswerBlock, _ answerDocLang) {
	if blk.Diagram == nil {
		return
	}
	d := blk.Diagram
	body := strings.TrimSpace(d.Body)
	if body == "" {
		return
	}
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	lang := strings.TrimSpace(d.Language)
	if lang == "" {
		lang = "mermaid"
	}
	fmt.Fprintf(b, "```%s\n%s\n```\n\n", lang, body)
}

func renderV2BlockCaveat(b *strings.Builder, blk types.AnswerBlock, _ answerDocLang) {
	body := renderUserSurfaceText(blk.Text)
	if body == "" {
		return
	}
	// Caveat blocks are rendered with a leading marker so the user
	// can spot them at a glance. Mirror the docs/architecture.md
	// guidance: caveats are out-of-band notes, not principal answer.
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "> **%s** %s\n\n", blk.Title, body)
		return
	}
	fmt.Fprintf(b, "> %s\n\n", body)
}

// renderV2BlockItem returns the inline string for one item: Label
// + optional Text + optional citation marker. Used by ordered /
// bullet / section lists.
func renderV2BlockItem(it types.AnswerBlockItem, doc *types.AnswerDocumentV2, _ answerDocLang) string {
	parts := make([]string, 0, 3)
	if l := renderUserSurfaceText(it.Label); l != "" {
		parts = append(parts, "**"+l+"**")
	}
	if t := renderUserSurfaceText(it.Text); t != "" {
		parts = append(parts, t)
	}
	out := strings.Join(parts, " — ")
	if doc != nil && it.CitationRef >= 0 && it.CitationRef < len(doc.Citations) {
		cite := renderCitationDisplay(doc.Citations[it.CitationRef])
		if cite != "" {
			out = out + " (" + cite + ")"
		}
	}
	return out
}

// blockTopCitation pulls the first valid citation reference from a
// block's items[] / claim_uses[]. Returns "" when no usable cite
// exists.
func blockTopCitation(blk types.AnswerBlock, doc *types.AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	for _, it := range blk.Items {
		if it.CitationRef >= 0 && it.CitationRef < len(doc.Citations) {
			return renderCitationDisplay(doc.Citations[it.CitationRef])
		}
	}
	return ""
}

func renderAnswerDocV2Caveats(b *strings.Builder, caveats []string, lang answerDocLang) {
	heading := "**Caveats:**"
	if lang == answerDocLangZH {
		heading = "**说明**："
	}
	fmt.Fprintf(b, "\n%s\n\n", heading)
	for _, c := range caveats {
		fmt.Fprintf(b, "- %s\n", strings.TrimSpace(c))
	}
}

func renderAnswerDocV2MissingRequestedRoles(b *strings.Builder, roles []types.AnswerMissingRequestedRole, lang answerDocLang) {
	if len(roles) == 0 {
		return
	}
	if lang == answerDocLangZH {
		b.WriteString("\n**缺失的请求层**：\n\n")
	} else {
		b.WriteString("\n**Missing requested layers:**\n\n")
	}
	for _, role := range roles {
		line := renderMissingRequestedRoleLine(role, lang)
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", line)
	}
	b.WriteString("\n")
}

func renderMissingRequestedRoleLine(role types.AnswerMissingRequestedRole, lang answerDocLang) string {
	label := strings.TrimSpace(role.Label)
	switch role.Role {
	case types.EvidenceDiagramRoleConfig:
		if lang == answerDocLangZH {
			if label != "" {
				return fmt.Sprintf("%s 层没有为这个精确目标提供匹配的配置项。", label)
			}
			return "配置文件层没有为这个精确目标提供匹配的配置项。"
		}
		if label != "" {
			return fmt.Sprintf("No %s entry binds this exact target.", label)
		}
		return "No config-file entry binds this exact target."
	case types.EvidenceDiagramRoleOverride:
		if strings.EqualFold(label, "cli") {
			if lang == answerDocLangZH {
				return "CLI 标志或命令行覆盖层未绑定这个精确目标。"
			}
			return "No CLI flag or command-line override binding exists for this exact target."
		}
		if lang == answerDocLangZH {
			if label != "" {
				return fmt.Sprintf("%s 覆盖层未绑定这个精确目标。", label)
			}
			return "覆盖层未绑定这个精确目标。"
		}
		if label != "" {
			return fmt.Sprintf("No %s override binding exists for this exact target.", label)
		}
		return "No override binding exists for this exact target."
	case types.EvidenceDiagramRoleDefault:
		if lang == answerDocLangZH {
			if label != "" {
				return fmt.Sprintf("%s 层没有为这个精确目标提供默认绑定。", label)
			}
			return "代码默认层没有为这个精确目标提供绑定。"
		}
		if label != "" {
			return fmt.Sprintf("No %s binding exists for this exact target.", label)
		}
		return "No code-default binding exists for this exact target."
	case types.EvidenceDiagramRoleRuntime:
		if lang == answerDocLangZH {
			if label != "" {
				return fmt.Sprintf("%s 层没有为这个精确目标提供运行时绑定。", label)
			}
			return "运行时层没有为这个精确目标提供绑定。"
		}
		if label != "" {
			return fmt.Sprintf("No %s binding exists for this exact target.", label)
		}
		return "No runtime binding exists for this exact target."
	default:
		return ""
	}
}

func renderAnswerDocV2Citations(b *strings.Builder, citations []types.Citation, lang answerDocLang) {
	if lang == answerDocLangZH {
		b.WriteString("\n**引用**：\n\n")
	} else {
		b.WriteString("\n**Citations:**\n\n")
	}
	for _, c := range citations {
		fmt.Fprintf(b, "- %s\n", renderCitationDisplay(c))
	}
}

func renderAnswerDocV2Snippets(b *strings.Builder, snippets []types.CodeSnippet, lang answerDocLang) {
	if lang == answerDocLangZH {
		b.WriteString("\n**关键代码**：\n\n")
	} else {
		b.WriteString("\n**Key snippets:**\n\n")
	}
	for _, s := range snippets {
		header := s.File
		if s.StartLine > 0 {
			header = fmt.Sprintf("%s:%d", s.File, s.StartLine)
			if s.EndLine > s.StartLine {
				header = fmt.Sprintf("%s-%d", header, s.EndLine)
			}
		}
		fmt.Fprintf(b, "📄 **`%s`**\n\n```%s\n%s\n```\n\n", header, s.Language, s.Code)
	}
}

// escapePipe replaces unescaped pipe characters in markdown table
// cells so a Label / Text containing "|" doesn't break the table.
func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func renderUserSurfaceText(s string) string {
	s = StripAuthorityArtifactsForRender(s)
	return strings.TrimSpace(s)
}
