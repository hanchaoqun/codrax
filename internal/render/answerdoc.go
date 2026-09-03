package render

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
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
	structuredDiagramBodies := answerDocumentStructuredDiagramBodyKeys(doc)
	duplicatedSectionItems := answerDocumentDuplicatedSectionItemBlocks(doc)
	var b strings.Builder

	renderAnswerDocV2ExactResolution(&b, doc.ExactResolution, docLang)
	renderedScopeDisclosures := map[types.ScopeDisclosureKind]bool{}
	for _, blk := range doc.Blocks {
		blk = stripDuplicateStructuredDiagramFencesFromBlock(blk, structuredDiagramBodies)
		if duplicatedSectionItems[strings.TrimSpace(blk.ID)] {
			blk.Items = nil
		}
		renderAnswerDocV2Block(&b, blk, doc, docLang)
		renderAnswerDocV2ScopeDisclosure(&b, blk.ScopeDisclosure, docLang, renderedScopeDisclosures)
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

func answerDocumentDuplicatedSectionItemBlocks(doc *types.AnswerDocumentV2) map[string]bool {
	out := map[string]bool{}
	if doc == nil || len(doc.Blocks) == 0 {
		return out
	}
	carrierItems := map[string]bool{}
	for _, blk := range doc.Blocks {
		switch blk.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		for _, it := range blk.Items {
			if key := answerDocumentVisibleItemKey(it); key != "" {
				carrierItems[key] = true
			}
		}
	}
	if len(carrierItems) == 0 {
		return out
	}
	for _, blk := range doc.Blocks {
		if blk.Kind != types.BlockSection || strings.TrimSpace(blk.ID) == "" || len(blk.Items) == 0 {
			continue
		}
		visible := 0
		covered := 0
		for _, it := range blk.Items {
			key := answerDocumentVisibleItemKey(it)
			if key == "" {
				continue
			}
			visible++
			if carrierItems[key] {
				covered++
			}
		}
		if visible > 0 && covered == visible {
			out[strings.TrimSpace(blk.ID)] = true
		}
	}
	return out
}

func answerDocumentVisibleItemKey(it types.AnswerBlockItem) string {
	refParts := make([]string, 0, 1+len(it.CitationRefs))
	for _, ref := range types.AnswerBlockItemCitationRefs(it) {
		refParts = append(refParts, fmt.Sprintf("%d", ref))
	}
	parts := []string{
		strings.TrimSpace(it.Label),
		strings.TrimSpace(it.Text),
		strings.Join(refParts, ","),
	}
	for _, cell := range it.Cells {
		parts = append(parts, strings.TrimSpace(cell))
	}
	joined := strings.Join(parts, "\x00")
	if strings.Trim(joined, "\x00 \t\r\n") == "" {
		return ""
	}
	return strings.ToLower(joined)
}

// RenderAnswerDocumentWithAttachments renders the structured V2 answer
// plus best-effort visible fragments recovered from malformed model
// output. Attachments are appended after the validated answer so they
// never masquerade as citation-checked blocks, but model-authored
// content that would otherwise be lost remains inspectable.
func RenderAnswerDocumentWithAttachments(doc *types.AnswerDocumentV2, attachments []types.AnswerDisplayAttachment, lang string) string {
	base := RenderAnswerDocument(doc, lang)
	extra := renderAnswerDisplayAttachments(doc, attachments, normalizeAnswerDocLang(lang))
	if strings.TrimSpace(extra) == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return extra
	}
	return strings.TrimRight(base, "\n") + "\n\n" + extra
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
		renderV2BlockSection(b, blk, doc, lang)
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
	renderV2StandaloneTypedRelations(b, blk, doc)
	renderV2RuntimeWorkRelationReceipt(b, blk, lang)
	renderV2ConceptualTerminalResolutionReceipt(b, blk, lang)
}

func renderV2RuntimeWorkRelationReceipt(b *strings.Builder, blk types.AnswerBlock, lang answerDocLang) {
	receipt := blk.RuntimeWorkRelation
	if b == nil || receipt == nil || !receipt.IsBound() || blk.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown {
		return
	}
	row := receipt.BoundRow
	if lang == answerDocLangZH {
		fmt.Fprintf(b, "**运行时工作关系判断**：`%s`", row.WorkLabel)
		if row.Subject != "" {
			fmt.Fprintf(b, "（线程 `%s`）", row.Subject)
		}
		fmt.Fprintf(b, "实测 %.3fms；%s\n\n", row.MeasuredDurationMS, runtimeWorkRelationConclusionZH(receipt.Conclusion, row.Credential, row.FrameCausalityApplicable))
		return
	}
	fmt.Fprintf(b, "**Runtime-work relation conclusion**: `%s`", row.WorkLabel)
	if row.Subject != "" {
		fmt.Fprintf(b, " (thread `%s`)", row.Subject)
	}
	fmt.Fprintf(b, " measured %.3fms; %s\n\n", row.MeasuredDurationMS, runtimeWorkRelationConclusionEN(receipt.Conclusion, row.Credential, row.FrameCausalityApplicable))
}

// runtimeWorkRelationConclusionZH — RECEIPT-1 (§40.32): the unproven-mechanism
// clause names a dropped frame / frame deadline only on a typed frame
// question (frame=true); otherwise it names the generic target-wait /
// completion mechanism. Credential facts are identical on both forks.
func runtimeWorkRelationConclusionZH(conclusion types.RuntimeWorkRelationConclusion, credential string, frame bool) string {
	unprovenTail := "目标等待其完成或它构成因果贡献"
	hostTail := "工作完成触发唤醒、目标等待该工作或该工作构成因果贡献"
	selfTail := "是否构成因果贡献仍需独立的等待或截止期证据"
	selfTailShort := "是否构成因果贡献仍未证"
	if frame {
		unprovenTail = "目标等待其完成或它造成丢帧"
		hostTail = "工作完成触发唤醒、目标等待该工作或该工作造成丢帧"
		selfTail = "是否造成该帧超时仍需独立帧或截止期证据"
		selfTailShort = "是否造成该帧超时仍未证"
	}
	switch conclusion {
	case types.RuntimeWorkRelationConclusionRelatedCausalityUnproven:
		if credential == "host_direct_wakeup_edge" {
			return "已证实该宿主线程随后直接唤醒目标，但尚未证明" + hostTail
		}
		return "已证实它与链上区间存在关系，但尚未证明" + unprovenTail
	case types.RuntimeWorkRelationConclusionTargetSelfWorkObserved:
		return "已证实这是目标自身执行的工作；" + selfTail
	case types.RuntimeWorkRelationConclusionCausalContributionSupported:
		return "现有链上证据支持其构成因果贡献，结论范围受该证据链约束"
	case types.RuntimeWorkRelationConclusionRelationUnproven:
		switch credential {
		case "host_direct_wakeup_edge":
			return "已证实宿主线程随后直接唤醒目标；但尚未证明该" + hostTail + "，因此工作到目标的因果关系仍未证"
		case "typed_chain_interval_overlap":
			return "已证实该工作与链上区间存在关系；但尚未证明" + unprovenTail + "，因此工作到目标的因果关系仍未证"
		case "target_self_execution":
			return "已证实这是目标自身执行的工作；" + selfTailShort
		default:
			return "已观测到该工作，但当前证据尚未建立它与目标的关系"
		}
	default:
		return "已观测到该工作，但当前证据尚未建立它与目标的关系"
	}
}

func runtimeWorkRelationConclusionEN(conclusion types.RuntimeWorkRelationConclusion, credential string, frame bool) string {
	hostTail := "work-completion, target-wait, and causal-contribution mechanisms remain unproved"
	overlapTail := "target wait/completion and causal contribution remain unproved"
	selfTail := "whether it constitutes a causal contribution needs separate wait or deadline evidence"
	selfTailShort := "whether it constitutes a causal contribution remains unproved"
	if frame {
		hostTail = "work-completion, target-wait, and dropped-frame causality remain unproved"
		overlapTail = "target wait/completion and dropped-frame causality remain unproved"
		selfTail = "whether it caused the frame or deadline miss needs separate frame evidence"
		selfTailShort = "whether it caused the frame or deadline miss remains unproved"
	}
	switch conclusion {
	case types.RuntimeWorkRelationConclusionRelatedCausalityUnproven:
		if credential == "host_direct_wakeup_edge" {
			return "the host thread is proved to wake the target directly afterward, but " + hostTail
		}
		return "a typed chain-interval relation is proved, but " + overlapTail
	case types.RuntimeWorkRelationConclusionTargetSelfWorkObserved:
		return "this is proved target-self work; " + selfTail
	case types.RuntimeWorkRelationConclusionCausalContributionSupported:
		return "typed on-chain evidence supports a causal contribution within that evidence boundary"
	case types.RuntimeWorkRelationConclusionRelationUnproven:
		switch credential {
		case "host_direct_wakeup_edge":
			return "the host thread is proved to wake the target directly afterward; " + hostTail + ", so the work-to-target causal relation is still unproved"
		case "typed_chain_interval_overlap":
			return "a typed chain-interval relation is proved; " + overlapTail + ", so the work-to-target causal relation is still unproved"
		case "target_self_execution":
			return "this is proved target-self work; " + selfTailShort
		default:
			return "the work is observed, but its relation to the target is not established by current evidence"
		}
	default:
		return "the work is observed, but its relation to the target is not established by current evidence"
	}
}

func renderV2ConceptualTerminalResolutionReceipt(b *strings.Builder, blk types.AnswerBlock, lang answerDocLang) {
	receipt := blk.ConceptualTerminalResolution
	if b == nil || receipt == nil || !receipt.IsBound() || blk.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown {
		return
	}
	row := receipt.BoundRow
	if lang == answerDocLangZH {
		b.WriteString("**概念目标核对**：")
		if row.TerminalCallable != "" && row.ExactOperation != "" {
			fmt.Fprintf(b, "当前已证终点操作为 `%s` 调用 `%s`", row.TerminalCallable, row.ExactOperation)
			if row.Source != "" {
				fmt.Fprintf(b, "（`%s`）", row.Source)
			}
			b.WriteString("；")
		}
		b.WriteString(conceptualTerminalResolutionConclusionZH(receipt.Conclusion, row.TerminalCallable != ""))
		b.WriteString("\n\n")
		return
	}
	b.WriteString("**Conceptual-destination check**: ")
	if row.TerminalCallable != "" && row.ExactOperation != "" {
		fmt.Fprintf(b, "the grounded terminal operation is `%s` calling `%s`", row.TerminalCallable, row.ExactOperation)
		if row.Source != "" {
			fmt.Fprintf(b, " (`%s`)", row.Source)
		}
		b.WriteString("; ")
	}
	b.WriteString(conceptualTerminalResolutionConclusionEN(receipt.Conclusion, row.TerminalCallable != ""))
	b.WriteString("\n\n")
}

func conceptualTerminalResolutionConclusionZH(conclusion types.ConceptualTerminalResolutionConclusion, hasOperation bool) string {
	switch conclusion {
	case types.ConceptualTerminalResolutionDestinationSupported:
		return "模型判断该精确操作支持用户要求的概念目标；结论范围只到这条已证操作，不额外证明未观测的下游效果"
	case types.ConceptualTerminalResolutionCurrentTerminalDiffers:
		return "模型判断当前实现终止于该精确操作，并未达到用户所述的概念目标"
	case types.ConceptualTerminalResolutionDestinationUnproven:
		if hasOperation {
			return "模型判断这条精确操作不足以确认用户所述的概念目标已经达到"
		}
		return "模型判断当前没有已证终点操作，无法确认用户所述的概念目标已经达到"
	default:
		return "模型判断现有证据不足以确认用户所述的概念目标已经达到"
	}
}

func conceptualTerminalResolutionConclusionEN(conclusion types.ConceptualTerminalResolutionConclusion, hasOperation bool) string {
	switch conclusion {
	case types.ConceptualTerminalResolutionDestinationSupported:
		return "the model concludes that this exact operation supports the requested conceptual destination; the conclusion extends only to this grounded operation and does not prove unobserved downstream effects"
	case types.ConceptualTerminalResolutionCurrentTerminalDiffers:
		return "the model concludes that the current implementation terminates at this exact operation and does not reach the requested conceptual destination"
	case types.ConceptualTerminalResolutionDestinationUnproven:
		if hasOperation {
			return "the model concludes that this exact operation is insufficient to establish that the requested conceptual destination was reached"
		}
		return "the model concludes that no grounded terminal operation is available, so the requested conceptual destination remains unproven"
	default:
		return "the model concludes that the current evidence is insufficient to establish the requested conceptual destination"
	}
}

// renderV2StandaloneTypedRelations makes model-authored relation metadata
// visible when it intentionally lives on a principal list/table without a
// Mermaid block. The system contributes only fixed Markdown punctuation. Both
// endpoints and the reader-facing label are copied from model-authored
// surfaces; an exact sibling-diagram alias may resolve to that diagram's
// unique visible label. RelationKind is checked for ownership but never
// translated into prose.
func renderV2StandaloneTypedRelations(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2) {
	if b == nil || blk.SurfaceRole != types.SurfacePrincipal || blk.Diagram != nil {
		return
	}
	switch blk.Kind {
	case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
	default:
		return
	}
	forms := make(map[types.ClaimForm]bool, len(blk.ClaimUses))
	for _, use := range blk.ClaimUses {
		if use.ClaimForm != types.ClaimUnknown {
			forms[use.ClaimForm] = true
		}
	}
	rendered := 0
	for _, anchor := range blk.EdgeAnchors {
		form := types.ClaimFormForRelation(anchor.RelationKind)
		if form == types.ClaimUnknown || !forms[form] {
			continue
		}
		fromNode, toNode := standaloneRelationReaderEndpoints(doc, anchor)
		from := renderUserSurfaceText(fromNode)
		to := renderUserSurfaceText(toNode)
		label := renderUserSurfaceText(anchor.VisibleLabel)
		if from == "" || to == "" || label == "" {
			continue
		}
		// A structured item can already carry the same model-authored relation
		// as a visible `source -> target` row. In that shape the anchor is
		// authority metadata, not a request for a second prose bullet. Keep the
		// fallback only for anchors whose relation is otherwise invisible.
		if standaloneRelationAlreadyVisibleInItems(blk, anchor) {
			continue
		}
		fmt.Fprintf(b, "- **%s → %s** — %s\n", from, to, label)
		rendered++
	}
	if rendered > 0 {
		b.WriteString("\n")
	}
}

func standaloneRelationAlreadyVisibleInItems(blk types.AnswerBlock, anchor types.DiagramEdgeAnchor) bool {
	for _, item := range blk.Items {
		surfaces := make([]string, 0, 1+len(item.Cells))
		surfaces = append(surfaces, item.Label)
		surfaces = append(surfaces, item.Cells...)
		for _, surface := range surfaces {
			left, right, ok := types.AnswerAggregateMemberRelationParts(surface)
			if !ok {
				continue
			}
			fromMatches := types.AnswerCodeIdentitySurfacesCompatible(left, anchor.FromIdentity) ||
				types.AnswerCodeIdentitySurfacesCompatible(left, anchor.FromNode)
			toMatches := types.AnswerCodeIdentitySurfacesCompatible(right, anchor.ToIdentity) ||
				types.AnswerCodeIdentitySurfacesCompatible(right, anchor.ToNode)
			if fromMatches && toMatches {
				return true
			}
		}
	}
	return false
}

// standaloneRelationReaderEndpoints replaces diagram-local aliases on a
// sibling structured relation carrier with the model-authored labels already
// visible in that exact diagram. Resolution is deliberately structural: both
// aliases must form one visible directed edge, and each alias must have one
// unambiguous label in that same diagram. It never guesses from alias spelling,
// endpoint identities, request text, or answer prose, and it never mutates the
// typed carrier. Ambiguous/missing mappings preserve the authored endpoints.
func standaloneRelationReaderEndpoints(doc *types.AnswerDocumentV2, anchor types.DiagramEdgeAnchor) (string, string) {
	fromNode := strings.TrimSpace(anchor.FromNode)
	toNode := strings.TrimSpace(anchor.ToNode)
	if doc == nil || fromNode == "" || toNode == "" {
		return fromNode, toNode
	}

	var resolvedFrom, resolvedTo string
	matches := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		if !mermaidBodyHasDirectedEdge(block.Diagram.Body, fromNode, toNode) {
			continue
		}
		labels := mermaidUniqueReaderLabels(block.Diagram.Body)
		fromLabel := strings.TrimSpace(labels[strings.ToLower(fromNode)])
		toLabel := strings.TrimSpace(labels[strings.ToLower(toNode)])
		if fromLabel == "" || toLabel == "" {
			continue
		}
		matches++
		if matches > 1 {
			return fromNode, toNode
		}
		resolvedFrom, resolvedTo = fromLabel, toLabel
	}
	if matches == 1 {
		return resolvedFrom, resolvedTo
	}
	return fromNode, toNode
}

func mermaidBodyHasDirectedEdge(body, from, to string) bool {
	for _, edge := range mermaidcompat.ParseEdges(body) {
		if strings.EqualFold(strings.TrimSpace(edge.From), strings.TrimSpace(from)) &&
			strings.EqualFold(strings.TrimSpace(edge.To), strings.TrimSpace(to)) {
			return true
		}
	}
	return false
}

func mermaidUniqueReaderLabels(body string) map[string]string {
	candidates := make(map[string]map[string]bool)
	add := func(decl mermaidcompat.NodeDecl) {
		id := strings.ToLower(strings.TrimSpace(decl.Ident))
		label := strings.TrimSpace(decl.Label)
		if id == "" || label == "" {
			return
		}
		if candidates[id] == nil {
			candidates[id] = make(map[string]bool)
		}
		candidates[id][label] = true
	}
	sequence := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		sequence = mermaidcompat.FirstKeywordIn(line) == "sequenceDiagram"
		break
	}
	for _, line := range strings.Split(body, "\n") {
		for _, decl := range mermaidcompat.SequenceParticipantDeclarations(line) {
			add(decl)
		}
		if sequence {
			continue
		}
		for _, decl := range mermaidcompat.NodeDeclarationsAll(line) {
			add(decl)
		}
	}
	out := make(map[string]string)
	for id, labels := range candidates {
		if len(labels) != 1 {
			continue
		}
		for label := range labels {
			out[id] = label
		}
	}
	return out
}

func renderV2BlockSummary(b *strings.Builder, blk types.AnswerBlock, _ answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "## %s\n\n", blk.Title)
	}
	if text := renderUserSurfaceProseText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
}

func renderV2BlockSection(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	heading := renderV2AuthoredOrSourceInventoryHeading(blk)
	if heading == "" {
		// A Section without an explicit Title is rendered without a
		// heading line; the body still appears. Source-inventory sections
		// have a typed family fallback, so their row ownership stays visible.
	} else if blk.SystemGeneratedKind.IsRuntimeTraceSupplement() {
		// Runtime-trace supplements are independently navigable report
		// chapters, not subheadings of whichever model-authored section happened
		// to precede them.  The authority bit is json:"-", so model prose cannot
		// opt itself into this structural promotion.
		fmt.Fprintf(b, "## %s\n\n", heading)
	} else {
		fmt.Fprintf(b, "### %s\n\n", heading)
	}
	if text := renderUserSurfaceProseText(blk.Text); text != "" {
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

func renderV2BlockOrderedList(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	if heading := renderV2ListHeading(blk, lang); heading != "" {
		renderV2ListOrTableHeading(b, blk, heading)
	}
	if text := renderUserSurfaceProseText(blk.Text); text != "" {
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
	if heading := renderV2ListHeading(blk, lang); heading != "" {
		renderV2ListOrTableHeading(b, blk, heading)
	}
	if text := renderUserSurfaceProseText(blk.Text); text != "" {
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

func renderV2ListHeading(blk types.AnswerBlock, lang answerDocLang) string {
	if heading := renderV2AuthoredOrSourceInventoryHeading(blk); heading != "" {
		return heading
	}
	if !answerBlockIDIsNextStepCarrier(blk.ID) {
		return ""
	}
	if lang == answerDocLangZH {
		return "下一步"
	}
	return "Next steps"
}

// renderV2AuthoredOrSourceInventoryHeading keeps authored presentation in charge while
// ensuring an exact typed source-inventory partition never becomes invisible.
// SourceInventoryFamily has already been validated against the admitted row
// registry; unlike Title/Text/item prose, it is the block's family authority.
// Rendering it as a fallback label does not infer, move, merge, or rewrite any
// model-owned member or conclusion.
func renderV2AuthoredOrSourceInventoryHeading(blk types.AnswerBlock) string {
	if title := strings.TrimSpace(blk.Title); title != "" {
		return title
	}
	if family := strings.TrimSpace(blk.SourceInventoryFamily); family != "" {
		return family
	}
	return ""
}

// renderV2ListOrTableHeading keeps ordinary model-authored list/table titles
// byte-compatible while giving authenticated runtime-trace decision and audit
// surfaces real chapter headings.  This fixes the HTML information hierarchy
// without trusting a title string or reserved ID supplied by the model.
func renderV2ListOrTableHeading(b *strings.Builder, blk types.AnswerBlock, heading string) {
	if b == nil || strings.TrimSpace(heading) == "" {
		return
	}
	if blk.SystemGeneratedKind.IsRuntimeTraceSupplement() {
		fmt.Fprintf(b, "## %s\n\n", heading)
		return
	}
	fmt.Fprintf(b, "**%s**\n\n", heading)
}

func answerBlockIDIsNextStepCarrier(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.NewReplacer("-", "_", " ", "_").Replace(id)
	switch id {
	case "next_step", "next_steps":
		return true
	default:
		return false
	}
}

func renderV2BlockScalar(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	literal := renderUserSurfaceText(blk.Text)
	if len(blk.Items) > 0 && literal == "" {
		// Scalar may use first item's Label as literal when Text is
		// empty (B3 schema accepts both shapes).
		literal = renderUserSurfaceText(blk.Items[0].Label)
	}
	if literal == "" {
		return
	}
	label := renderUserSurfaceText(blk.Title)
	cite := blockTopCitation(blk, doc)
	code := renderScalarLiteralAsCodeSpan(literal)
	if label == "" && answerDocumentHasVisibleNonScalarBlock(doc) {
		// In mixed explanatory answers, an untitled scalar is usually a
		// supporting value that the model already names in surrounding prose.
		// Rendering a synthetic "Value/值" heading makes system-authored
		// wording look like part of the answer contract. Keep the literal
		// visible without inventing a label.
		if code {
			fmt.Fprintf(b, "`%s`", literal)
		} else {
			b.WriteString(literal)
		}
		if cite != "" {
			fmt.Fprintf(b, " (%s)", cite)
		}
		b.WriteString("\n\n")
		return
	}
	if label == "" {
		label = "Value"
		if lang == answerDocLangZH {
			label = "值"
		}
	}
	sep := ":"
	if lang == answerDocLangZH {
		sep = "："
	}
	if code {
		fmt.Fprintf(b, "**%s%s** `%s`", label, sep, literal)
	} else {
		fmt.Fprintf(b, "**%s%s** %s", label, sep, literal)
	}
	if cite != "" {
		fmt.Fprintf(b, " (%s)", cite)
	}
	b.WriteString("\n\n")
}

// renderScalarLiteralAsCodeSpan reports whether a scalar block's literal may
// render inside a single-backtick code span.
//
// DISP-3 item8 (§29.8 P3 "opendir 反引号整段落 metric 块形+嵌套反引号破损",
// real_trace_campaign_20260705.md, 2026-07-09; witness
// cust_trace_opendir_792.txt lines 57/59): the model can put a whole
// metric-explanation PARAGRAPH into a scalar block — the unconditional wrap
// rendered a paragraph-sized code span, and a literal that itself contains a
// backtick (the line-59 form: `AssetManager.getResourceValue` inside the
// paragraph) produced broken nested code spans. Such literals now render as
// plain text — no system wording is added, the model's bytes pass through
// unchanged; only the broken/absurd markup is withheld.
//
// Verbatim glyph checks only (a display-form fork, never a reject gate):
//   - a backtick cannot nest inside a single-backtick code span;
//   - a newline cannot live inside one either;
//   - CJK sentence enders (。！？) and the ASCII ". " sentence break mark the
//     literal as prose, not a single literal value (a decimal "3.14" never
//     matches — its '.' is not followed by a space).
//
// True scalars (numbers, identifiers, paths, config values) contain none of
// these and keep the code span byte-identically.
func renderScalarLiteralAsCodeSpan(literal string) bool {
	if strings.ContainsAny(literal, "`\n") {
		return false
	}
	if strings.ContainsAny(literal, "。！？") {
		return false
	}
	return !strings.Contains(literal, ". ")
}

func answerDocumentHasVisibleNonScalarBlock(doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if block.Kind == types.BlockScalar {
			continue
		}
		if strings.TrimSpace(block.Title) != "" ||
			strings.TrimSpace(block.Text) != "" ||
			len(block.Items) > 0 ||
			block.Diagram != nil {
			return true
		}
	}
	return false
}

func renderV2BlockDecision(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	prefix := "Decision:"
	if lang == answerDocLangZH {
		prefix = "结论："
	}
	body := renderUserSurfaceProseText(blk.Text)
	if blk.CurrentStatusVerdict != "" {
		rawVerdict := string(blk.CurrentStatusVerdict)
		body = stripLeadingDecisionVerdict(body, rawVerdict)
		var verdict string
		if types.CurrentStatusDowngradeForBlock(doc, blk) != nil {
			// SPR #72 (RTC §8.3): run-level evidence downgrade — the
			// origin-lane ledger holds zero current_source evidence, so
			// the side-picked verdict renders as a caveat disclosure
			// instead of an asserted conclusion. The model's prose body
			// is preserved verbatim below.
			verdict = decisionVerdictDowngradeDisplay(rawVerdict, lang)
		} else {
			verdict = decisionVerdictDisplay(rawVerdict, lang)
		}
		if body != "" {
			body = verdict + " — " + body
		} else {
			body = verdict
		}
	}
	if blk.ErrorGranularityVerdict != "" {
		rawVerdict := string(blk.ErrorGranularityVerdict)
		body = stripLeadingDecisionVerdict(body, rawVerdict)
		verdict := decisionVerdictDisplay(rawVerdict, lang)
		if body != "" {
			body = verdict + " — " + body
		} else {
			body = verdict
		}
	}
	fmt.Fprintf(b, "**%s** %s", prefix, body)
	cite := blockTopCitation(blk, doc)
	if cite != "" {
		fmt.Fprintf(b, " (%s)", cite)
	}
	b.WriteString("\n\n")
}

func stripLeadingDecisionVerdict(body, verdict string) string {
	body = strings.TrimSpace(body)
	verdict = strings.TrimSpace(verdict)
	if body == "" || verdict == "" {
		return body
	}
	candidates := []string{"`" + verdict + "`", verdict}
	for _, candidate := range candidates {
		if len(body) < len(candidate) || !strings.EqualFold(body[:len(candidate)], candidate) {
			continue
		}
		if len(body) > len(candidate) {
			next, _ := utf8.DecodeRuneInString(body[len(candidate):])
			if !isDecisionVerdictBoundary(next) {
				continue
			}
		}
		rest := strings.TrimSpace(body[len(candidate):])
		rest = strings.TrimLeft(rest, " \t\r\n-:：—–，,。.;；")
		return strings.TrimSpace(rest)
	}
	return body
}

func isDecisionVerdictBoundary(r rune) bool {
	switch r {
	case ' ', '\t', '\r', '\n', '-', ':', '：', '—', '–', '，', ',', '。', '.', ';', '；':
		return true
	default:
		return false
	}
}

func renderAnswerDocV2ExactResolution(b *strings.Builder, exact *types.AnswerExactResolution, lang answerDocLang) {
	if exact == nil || exact.Status == "" {
		return
	}
	line := exactResolutionDisplayLine(exact, lang)
	if strings.TrimSpace(line) == "" {
		return
	}
	fmt.Fprintf(b, "> %s\n\n", line)
}

func exactResolutionDisplayLine(exact *types.AnswerExactResolution, lang answerDocLang) string {
	if exact == nil {
		return ""
	}
	anchor := strings.TrimSpace(exact.Anchor)
	context := exact.ContextMode == types.AnswerExactResolutionContextGroundedOnly
	if lang == answerDocLangZH {
		var line string
		switch exact.Status {
		case types.AnswerExactResolutionExactMatch:
			if anchor != "" {
				line = fmt.Sprintf("精确目标已命中：`%s`。", anchor)
			} else {
				line = "精确目标已命中。"
			}
		case types.AnswerExactResolutionAliasMatch:
			if anchor != "" {
				line = fmt.Sprintf("未找到完全一致的精确目标，但已验证等价/别名锚点：`%s`。", anchor)
			} else {
				line = "未找到完全一致的精确目标，但已验证等价/别名锚点。"
			}
		case types.AnswerExactResolutionAbsent:
			line = "当前已验证范围内未找到完全一致的精确目标。"
		default:
			return ""
		}
		if context {
			line += " 下文的相关内容仅作为已落地上下文，不代表精确命中。"
		}
		return line
	}
	var line string
	switch exact.Status {
	case types.AnswerExactResolutionExactMatch:
		if anchor != "" {
			line = fmt.Sprintf("Exact target resolved: `%s`.", anchor)
		} else {
			line = "Exact target resolved."
		}
	case types.AnswerExactResolutionAliasMatch:
		if anchor != "" {
			line = fmt.Sprintf("No exact target match was found, but a verified alias/equivalent anchor was resolved: `%s`.", anchor)
		} else {
			line = "No exact target match was found, but a verified alias/equivalent anchor was resolved."
		}
	case types.AnswerExactResolutionAbsent:
		line = "No exact target match was found within the verified scope."
	default:
		return ""
	}
	if context {
		line += " Related content below is grounded context only, not an exact match."
	}
	return line
}

func renderAnswerDocV2ScopeDisclosure(b *strings.Builder, disclosure types.ScopeDisclosureKind, lang answerDocLang, seen map[types.ScopeDisclosureKind]bool) {
	if disclosure == types.ScopeDisclosureUnknown || !disclosure.IsValid() {
		return
	}
	if seen != nil {
		if seen[disclosure] {
			return
		}
		seen[disclosure] = true
	}
	line := scopeDisclosureDisplayLine(disclosure, lang)
	if strings.TrimSpace(line) == "" {
		return
	}
	fmt.Fprintf(b, "> %s\n\n", line)
}

func scopeDisclosureDisplayLine(disclosure types.ScopeDisclosureKind, lang answerDocLang) string {
	if lang == answerDocLangZH {
		switch disclosure {
		case types.ScopeDisclosureInactiveScopeNamed:
			return "范围说明：答案已显式标注当前活跃仓范围之外的相关子仓。"
		case types.ScopeDisclosureOutOfActiveScope:
			return "范围说明：目标不在当前活跃仓范围内，本回答只覆盖本次已激活的仓库集合。"
		case types.ScopeDisclosureRequiresWorkspaceAdjust:
			return "范围说明：需要调整活跃仓库范围后再验证这个目标。"
		default:
			return ""
		}
	}
	switch disclosure {
	case types.ScopeDisclosureInactiveScopeNamed:
		return "Scope note: the answer explicitly names relevant sub-repositories outside the current active set."
	case types.ScopeDisclosureOutOfActiveScope:
		return "Scope note: the target is outside the current active repository set; this answer only covers the repositories active in this run."
	case types.ScopeDisclosureRequiresWorkspaceAdjust:
		return "Scope note: adjust the active repository set before verifying this target."
	default:
		return ""
	}
}

func renderV2BlockTable(b *strings.Builder, blk types.AnswerBlock, _ *types.AnswerDocumentV2, lang answerDocLang) {
	if heading := renderV2AuthoredOrSourceInventoryHeading(blk); heading != "" {
		renderV2ListOrTableHeading(b, blk, heading)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
		if types.AnswerTextLooksLikeMarkdownTable(text) {
			return
		}
	}
	if text := renderV2TableItemMarkdownText(blk); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
		return
	}
	if len(blk.Items) == 0 {
		return
	}
	if renderV2StructuredTable(b, blk, lang) {
		return
	}
	rendered := 0
	for _, it := range blk.Items {
		if strings.TrimSpace(it.Label) == "" && strings.TrimSpace(it.Text) == "" {
			continue
		}
		rendered++
	}
	if rendered == 0 {
		return
	}
	labelHeader, detailHeader := renderV2TableFallbackHeaders(lang)
	fmt.Fprintf(b, "| %s | %s |\n|---|---|\n", labelHeader, detailHeader)
	for _, it := range blk.Items {
		label := strings.TrimSpace(it.Label)
		text := strings.TrimSpace(it.Text)
		if label == "" && text == "" {
			continue
		}
		fmt.Fprintf(b, "| %s | %s |\n", escapePipe(label), escapePipe(text))
	}
	b.WriteString("\n")
}

func renderV2TableText(blk types.AnswerBlock) string {
	text := renderUserSurfaceText(blk.Text)
	if text != "" && types.AnswerTextLooksLikeMarkdownTable(text) {
		return text
	}
	return renderV2TableItemMarkdownText(blk)
}

func renderV2TableItemMarkdownText(blk types.AnswerBlock) string {
	var parts []string
	seen := make(map[string]bool)
	for _, it := range blk.Items {
		for _, raw := range []string{it.Label, it.Text} {
			candidate := renderUserSurfaceText(raw)
			if candidate == "" || !types.AnswerTextLooksLikeMarkdownTable(candidate) {
				continue
			}
			key := strings.TrimSpace(candidate)
			if seen[key] {
				continue
			}
			seen[key] = true
			parts = append(parts, candidate)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

func renderV2StructuredTable(b *strings.Builder, blk types.AnswerBlock, lang answerDocLang) bool {
	columns := renderV2NormalizeTableStrings(blk.Columns)
	hasStructuredCarrier := len(columns) > 0
	rows := make([]renderV2TableRow, 0, len(blk.Items))
	hasLabel := false
	maxCells := 0
	for _, it := range blk.Items {
		label := strings.TrimSpace(it.Label)
		if len(renderV2NormalizeTableStrings(it.Cells)) > 0 {
			hasStructuredCarrier = true
		}
		cells := renderV2TableItemCells(it)
		if label == "" && len(cells) == 0 {
			continue
		}
		if label != "" {
			hasLabel = true
		}
		if len(cells) > maxCells {
			maxCells = len(cells)
		}
		rows = append(rows, renderV2TableRow{label: label, cells: cells})
	}
	if !hasStructuredCarrier || len(rows) == 0 || (len(columns) == 0 && maxCells == 0) {
		return false
	}
	headers := renderV2StructuredTableHeaders(columns, hasLabel, maxCells, lang)
	if len(headers) == 0 {
		return false
	}
	renderRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(headers))
		if hasLabel {
			cells = append(cells, row.label)
		}
		cells = append(cells, row.cells...)
		for len(cells) < len(headers) {
			cells = append(cells, "")
		}
		if len(cells) > len(headers) {
			cells = cells[:len(headers)]
		}
		renderRows = append(renderRows, cells)
	}
	headers, renderRows = renderV2CompactEmptyStructuredColumns(headers, renderRows)
	if len(headers) == 0 {
		return false
	}
	fmt.Fprintf(b, "| %s |\n", strings.Join(renderV2EscapedTableCells(headers), " | "))
	b.WriteString("|")
	for range headers {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, cells := range renderRows {
		fmt.Fprintf(b, "| %s |\n", strings.Join(renderV2EscapedTableCells(cells), " | "))
	}
	b.WriteString("\n")
	return true
}

type renderV2TableRow struct {
	label string
	cells []string
}

func renderV2TableItemCells(it types.AnswerBlockItem) []string {
	cells := renderV2NormalizeTableStrings(it.Cells)
	text := strings.TrimSpace(it.Text)
	if len(cells) == 0 {
		if text == "" {
			return nil
		}
		return []string{text}
	}
	if text != "" && !renderV2TableCellsContain(cells, text) {
		cells = append(cells, text)
	}
	return cells
}

func renderV2TableCellsContain(cells []string, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	for _, cell := range cells {
		if strings.TrimSpace(cell) == text {
			return true
		}
	}
	return false
}

func renderV2StructuredTableHeaders(columns []string, hasLabel bool, maxCells int, lang answerDocLang) []string {
	if hasLabel {
		switch {
		case len(columns) == maxCells+1:
			return renderV2FillTableHeaders(columns, len(columns), lang, true)
		case len(columns) >= maxCells+1 && maxCells > 0:
			return renderV2FillTableHeaders(columns[:maxCells+1], maxCells+1, lang, true)
		case maxCells == 0 && len(columns) > 0:
			return renderV2FillTableHeaders(columns[:1], 1, lang, true)
		case len(columns) > 0:
			headers := append([]string{renderV2TableRowHeader(lang)}, columns...)
			return renderV2FillTableHeaders(headers, maxCells+1, lang, true)
		default:
			return renderV2FillTableHeaders([]string{renderV2TableRowHeader(lang)}, maxCells+1, lang, true)
		}
	}
	width := maxCells
	if len(columns) > width {
		width = len(columns)
	}
	return renderV2FillTableHeaders(columns, width, lang, false)
}

func renderV2CompactEmptyStructuredColumns(headers []string, rows [][]string) ([]string, [][]string) {
	if len(headers) == 0 || len(rows) == 0 {
		return headers, rows
	}
	keep := make([]bool, len(headers))
	kept := 0
	for col := range headers {
		for _, row := range rows {
			if col < len(row) && strings.TrimSpace(row[col]) != "" {
				keep[col] = true
				kept++
				break
			}
		}
	}
	if kept == 0 {
		keep[0] = true
		kept = 1
	}
	if kept == len(headers) {
		return headers, rows
	}
	outHeaders := make([]string, 0, kept)
	for col, h := range headers {
		if keep[col] {
			outHeaders = append(outHeaders, h)
		}
	}
	outRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		out := make([]string, 0, kept)
		for col := range headers {
			if !keep[col] {
				continue
			}
			if col < len(row) {
				out = append(out, row[col])
			} else {
				out = append(out, "")
			}
		}
		outRows = append(outRows, out)
	}
	return outHeaders, outRows
}

func renderV2FillTableHeaders(headers []string, width int, lang answerDocLang, hasLabel bool) []string {
	if width <= 0 {
		return nil
	}
	out := append([]string(nil), headers...)
	for len(out) < width {
		out = append(out, renderV2SyntheticTableHeader(len(out), width, lang, hasLabel))
	}
	for i, h := range out {
		if strings.TrimSpace(h) == "" {
			out[i] = renderV2SyntheticTableHeader(i, width, lang, hasLabel)
		}
	}
	return out
}

func renderV2SyntheticTableHeader(index, width int, lang answerDocLang, hasLabel bool) string {
	if hasLabel && index == 0 {
		return renderV2TableRowHeader(lang)
	}
	if width == 2 && hasLabel && index == 1 {
		_, detail := renderV2TableFallbackHeaders(lang)
		return detail
	}
	if lang == answerDocLangZH {
		return fmt.Sprintf("列 %d", index+1)
	}
	return fmt.Sprintf("Column %d", index+1)
}

func renderV2TableRowHeader(lang answerDocLang) string {
	label, _ := renderV2TableFallbackHeaders(lang)
	return label
}

func renderV2NormalizeTableStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.TrimSpace(s)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func renderV2EscapedTableCells(cells []string) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = escapePipe(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(cell), "\r\n", "\n"), "\n", "<br>"))
	}
	return out
}

func renderV2TableFallbackHeaders(lang answerDocLang) (string, string) {
	if lang == answerDocLangZH {
		return "项目", "说明"
	}
	return "Item", "Details"
}

func renderV2BlockDiagram(b *strings.Builder, blk types.AnswerBlock, lang answerDocLang) {
	if blk.Diagram == nil {
		return
	}
	d := blk.Diagram
	body := normalizeDiagramBodyForRender(d.Body)
	if body == "" {
		return
	}
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	if text := renderUserSurfaceProseText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	diagramLang := strings.TrimSpace(d.Language)
	if diagramLang == "" {
		diagramLang = "mermaid"
	}
	fmt.Fprintf(b, "```%s\n%s\n```\n\n", diagramLang, body)
	if blk.RequestedRelationScope == types.DiagramRelationScopePartialUnproven {
		if lang == answerDocLangZH {
			b.WriteString("**关系覆盖范围：** 当前图仅展示已有证据支持的局部关系；现有证据尚未证明这些片段构成用户所请求的完整端到端关系。\n\n")
		} else {
			b.WriteString("**Relationship coverage:** This diagram shows only locally proved relations; current evidence does not yet prove that these segments form the complete requested end-to-end relation.\n\n")
		}
	}
	if len(blk.ParticipantBoundaries) > 0 {
		participants := make([]string, 0, len(blk.ParticipantBoundaries))
		for _, boundary := range blk.ParticipantBoundaries {
			if boundary.Status != types.DiagramParticipantBoundaryUnproven || strings.TrimSpace(boundary.Participant) == "" {
				continue
			}
			participants = append(participants, "`"+strings.TrimSpace(boundary.Participant)+"`")
		}
		if len(participants) > 0 {
			if lang == answerDocLangZH {
				fmt.Fprintf(b, "**未证关系边界：** %s（图中保留已证局部事实或包含关系，但当前证据未证明用户所请求的有向关系；不得据此补画连接）。\n\n", strings.Join(participants, "、"))
			} else {
				fmt.Fprintf(b, "**Unproven relation boundaries:** %s (the diagram may retain proved local facts or containment, but current evidence does not prove the requested directed relation; no connecting edge may be inferred).\n\n", strings.Join(participants, ", "))
			}
		}
	}
}

const (
	maxAnswerDisplayAttachments = 4
	// maxAnswerDisplayBodyRunes is a protective ceiling only — it must dwarf
	// any REAL answer shape. TRUNC 批 (P1, §29.10-1, 2026-07-09): the old
	// 16000-rune value silently amputated huadong_792's preserved first
	// draft mid-body with a bare "..." (witness cut byte-exact at rune
	// 16000) — a full multi-window trace draft runs 2-4 万 runes. 200000
	// is pinned as a ratchet by TestTRUNCDisplayBodyCapRatchet; when the
	// ceiling does fire, trimDisplayAttachmentBody now returns a typed
	// truncation signal and the renderer appends an explicit disclosure
	// (total + shown rune counts) — customer-facing answers never
	// silently truncate.
	maxAnswerDisplayBodyRunes = 200000
)

func renderAnswerDisplayAttachments(doc *types.AnswerDocumentV2, attachments []types.AnswerDisplayAttachment, lang answerDocLang) string {
	if len(attachments) == 0 {
		return ""
	}
	seen := answerDocumentVisibleAttachmentIndex(doc, lang)
	var b strings.Builder
	rendered := 0
	skipped := 0
	// P2-1 (§29.47.1 follow-up, 2026-07-12): the panel lead-in forks on the
	// typed Source class — preserved MODEL output keeps the incumbent
	// model-provenance wording; SYSTEM-authored deterministic content (the
	// cross-check appendix, review notes) opens under the system's own
	// voice. Two passes, model panel first (the historical order), one
	// shared dedupe index and one shared display cap across both.
	renderGroup := func(systemAuthored bool, panelStart string) {
		groupRendered := 0
		for _, att := range attachments {
			if att.SystemAuthored() != systemAuthored {
				continue
			}
			body, totalRunes, truncated := trimDisplayAttachmentBody(att.Body)
			if body == "" {
				continue
			}
			if seen.Contains(att.Kind, body) {
				continue
			}
			seen.Add(att.Kind, body)
			if rendered >= maxAnswerDisplayAttachments {
				skipped++
				continue
			}
			if groupRendered == 0 {
				start := panelStart
				// DISPLAY-HYG 二轮 catalog C6 (§29.104.18.1, 2026-07-17):
				// consecutive horizontal rules collapse — when the previous
				// group's closing rule already separates the panels, the next
				// panel's leading rule is redundant (witness L960/L962 double
				// `---`). Single-panel renders keep their bytes.
				if strings.HasSuffix(b.String(), "---\n\n") {
					start = strings.TrimPrefix(start, "---\n\n")
				}
				b.WriteString(start)
			}
			renderDisplayAttachment(&b, att, body, lang)
			if truncated {
				renderDisplayAttachmentTruncationNote(&b, totalRunes, lang)
			}
			groupRendered++
			rendered++
		}
		if groupRendered > 0 {
			b.WriteString(displayAttachmentPanelEnd())
		}
	}
	renderGroup(false, displayAttachmentPanelStart(lang))
	renderGroup(true, displaySystemAttachmentPanelStart(lang))
	if rendered == 0 {
		return ""
	}
	if skipped > 0 {
		renderDisplayAttachmentOmissionNote(&b, skipped, lang)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

type displayAttachmentIndex struct {
	exact     map[string]bool
	plainText []string
}

func answerDocumentVisibleAttachmentIndex(doc *types.AnswerDocumentV2, lang answerDocLang) displayAttachmentIndex {
	seen := displayAttachmentIndex{exact: map[string]bool{}}
	if doc == nil {
		return seen
	}
	if full := strings.TrimSpace(RenderAnswerDocument(doc, answerDocLangCode(lang))); full != "" {
		seen.Add(types.AnswerDisplayAttachmentMarkdown, full)
		seen.Add(types.AnswerDisplayAttachmentText, full)
	}
	for _, blk := range doc.Blocks {
		if blk.Diagram != nil {
			body := normalizeDiagramBodyForRender(blk.Diagram.Body)
			if body != "" {
				seen.Add(types.AnswerDisplayAttachmentDiagram, body)
			}
		}
		if text := strings.TrimSpace(blk.Text); text != "" {
			seen.Add(types.AnswerDisplayAttachmentMarkdown, text)
			seen.Add(types.AnswerDisplayAttachmentText, text)
		}
	}
	return seen
}

func (idx *displayAttachmentIndex) Contains(kind, body string) bool {
	if idx == nil {
		return false
	}
	if idx.exact[displayAttachmentKey(kind, body)] {
		return true
	}
	if !displayAttachmentKindCanUsePlainTextDedup(kind) {
		return false
	}
	key := displayAttachmentPlainTextDedupKey(body)
	if key == "" {
		return false
	}
	for _, existing := range idx.plainText {
		if existing == key || strings.Contains(existing, key) {
			return true
		}
	}
	return false
}

func (idx *displayAttachmentIndex) Add(kind, body string) {
	if idx == nil {
		return
	}
	if idx.exact == nil {
		idx.exact = map[string]bool{}
	}
	idx.exact[displayAttachmentKey(kind, body)] = true
	if !displayAttachmentKindCanUsePlainTextDedup(kind) {
		return
	}
	if key := displayAttachmentPlainTextDedupKey(body); key != "" {
		idx.plainText = append(idx.plainText, key)
	}
}

func displayAttachmentKindCanUsePlainTextDedup(kind string) bool {
	return kind == types.AnswerDisplayAttachmentMarkdown || kind == types.AnswerDisplayAttachmentText || strings.TrimSpace(kind) == ""
}

func displayAttachmentPlainTextDedupKey(s string) string {
	s = renderUserSurfaceText(s)
	if strings.TrimSpace(s) == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	parts := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence && answerDocMarkdownTableSeparatorLine(trimmed) {
			continue
		}
		parts = append(parts, displayAttachmentPlainLineKey(trimmed))
	}
	key := strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
	if utf8.RuneCountInString(key) < 16 {
		return ""
	}
	return key
}

func answerDocMarkdownTableSeparatorLine(s string) bool {
	if s == "" || !strings.Contains(s, "|") {
		return false
	}
	for _, r := range s {
		switch r {
		case '|', '-', ':', ' ', '\t':
			continue
		default:
			return false
		}
	}
	return true
}

func displayAttachmentPlainLineKey(line string) string {
	line = strings.TrimSpace(line)
	for strings.HasPrefix(line, "#") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	}
	for strings.HasPrefix(line, ">") {
		line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
	}
	for strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		line = strings.TrimSpace(line[2:])
	}
	if dot := strings.Index(line, ". "); dot > 0 {
		prefix := line[:dot]
		allDigits := true
		for _, r := range prefix {
			if !unicode.IsDigit(r) {
				allDigits = false
				break
			}
		}
		if allDigits {
			line = strings.TrimSpace(line[dot+2:])
		}
	}
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")
	line = strings.ReplaceAll(line, "`", "")
	var b strings.Builder
	lastSpace := true
	for _, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func answerDocLangCode(lang answerDocLang) string {
	if lang == answerDocLangZH {
		return "zh"
	}
	return "en"
}

func answerDocumentStructuredDiagramBodyKeys(doc *types.AnswerDocumentV2) map[string]bool {
	seen := map[string]bool{}
	if doc == nil {
		return seen
	}
	for _, blk := range doc.Blocks {
		if blk.Kind != types.BlockDiagram || blk.Diagram == nil {
			continue
		}
		if key := diagramBodyDedupKey(blk.Diagram.Body); key != "" {
			seen[key] = true
		}
	}
	return seen
}

func stripDuplicateStructuredDiagramFencesFromBlock(blk types.AnswerBlock, diagramBodies map[string]bool) types.AnswerBlock {
	if len(diagramBodies) == 0 {
		return blk
	}
	blk.Text = stripDuplicateStructuredDiagramFences(blk.Text, diagramBodies)
	for i := range blk.Items {
		blk.Items[i].Label = stripDuplicateStructuredDiagramFences(blk.Items[i].Label, diagramBodies)
		blk.Items[i].Text = stripDuplicateStructuredDiagramFences(blk.Items[i].Text, diagramBodies)
	}
	return blk
}

func stripDuplicateStructuredDiagramFences(text string, diagramBodies map[string]bool) string {
	if strings.TrimSpace(text) == "" || len(diagramBodies) == 0 || !strings.Contains(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "```") {
			out = append(out, line)
			i++
			continue
		}
		info := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "```" {
				end = j
				break
			}
		}
		if end < 0 {
			out = append(out, line)
			i++
			continue
		}
		body := strings.Join(lines[i+1:end], "\n")
		if answerDocFenceCanDedupStructuredDiagram(info) && diagramBodies[diagramBodyDedupKey(body)] {
			i = end + 1
			continue
		}
		out = append(out, lines[i:end+1]...)
		i = end + 1
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func answerDocFenceCanDedupStructuredDiagram(info string) bool {
	info = strings.TrimSpace(info)
	if info == "" {
		return true
	}
	fields := strings.Fields(info)
	if len(fields) == 0 {
		return true
	}
	return strings.EqualFold(fields[0], "mermaid")
}

func renderDisplayAttachment(b *strings.Builder, att types.AnswerDisplayAttachment, body string, lang answerDocLang) {
	title := strings.TrimSpace(att.Title)
	switch att.Kind {
	case types.AnswerDisplayAttachmentDiagram:
		body = normalizeDiagramBodyForRender(body)
		if body == "" {
			return
		}
		if title == "" {
			if lang == answerDocLangZH {
				title = "保留的图"
			} else {
				title = "Preserved diagram"
			}
		}
		fmt.Fprintf(b, "#### %s\n\n", title)
		fenceLang := strings.TrimSpace(att.Language)
		if fenceLang == "" {
			fenceLang = "mermaid"
		}
		fmt.Fprintf(b, "```%s\n%s\n```\n\n", fenceLang, body)
	default:
		if title == "" {
			if lang == answerDocLangZH {
				title = "保留的原文"
			} else {
				title = "Preserved text"
			}
		}
		fmt.Fprintf(b, "#### %s\n\n%s\n\n", title, renderUserSurfaceText(body))
	}
}

func renderDisplayAttachmentOmissionNote(b *strings.Builder, skipped int, lang answerDocLang) {
	if skipped <= 0 {
		return
	}
	if lang == answerDocLangZH {
		fmt.Fprintf(b, "> 另有 %d 条保留内容未显示，以避免答案面板过长；结构化答案主体不受影响。\n\n", skipped)
		return
	}
	fmt.Fprintf(b, "> %d additional preserved item(s) were not shown to keep the answer panel readable; the structured answer body is unaffected.\n\n", skipped)
}

func displayAttachmentPanelStart(lang answerDocLang) string {
	if lang == answerDocLangZH {
		return "---\n\n> **系统保留内容**\n>\n> 下面内容来自模型已生成但未能完整进入结构化答案的输出，系统按原文保留展示；这部分不是上方已校验结构化答案的主体，请按补充参考阅读。\n\n"
	}
	return "---\n\n> **System-preserved content**\n>\n> The following content was generated by the model but could not be fully preserved in the structured answer. It is shown verbatim as supplemental reference, not as part of the validated structured answer above.\n\n"
}

// displaySystemAttachmentPanelStart is the SYSTEM-authored group's lead-in
// (P2-1): deterministic system-voice content is never introduced as
// preserved model output.
func displaySystemAttachmentPanelStart(lang answerDocLang) string {
	if lang == answerDocLangZH {
		return "---\n\n> **系统生成内容**\n>\n> 以下为系统生成的确定性内容（校验附注、审阅备注等），不属于模型撰写的正文；请与上方结构化答案对照参考。\n\n"
	}
	return "---\n\n> **System-generated content**\n>\n> The following is deterministic content generated by the system (cross-check notes, review notes); it is not part of the model-authored answer body and is provided for reference alongside the structured answer above.\n\n"
}

func displayAttachmentPanelEnd() string {
	return "---\n\n"
}

// trimDisplayAttachmentBody bounds one attachment body at the protective
// ceiling. TRUNC 批 (§29.10-1): the legacy shape returned the cut body with a
// bare "\n..." — the huadong_792 witness's silent mid-draft amputation. The
// truncation is now a typed return (trimmed body, total rune count, fired
// flag) so the caller renders an EXPLICIT disclosure instead of a bare
// ellipsis; the trimmed body itself carries no marker.
func trimDisplayAttachmentBody(body string) (string, int, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", 0, false
	}
	rs := []rune(body)
	if len(rs) <= maxAnswerDisplayBodyRunes {
		return body, len(rs), false
	}
	return strings.TrimRight(string(rs[:maxAnswerDisplayBodyRunes]), "\n"), len(rs), true
}

// renderDisplayAttachmentTruncationNote appends the explicit truncation
// disclosure for one over-ceiling attachment body. Customer-facing answers
// never silently truncate: the note states the total and shown rune counts
// in the answer language (TRUNC §29.10-1 修根,取代 witness 的裸 "..." 行).
func renderDisplayAttachmentTruncationNote(b *strings.Builder, totalRunes int, lang answerDocLang) {
	if lang == answerDocLangZH {
		fmt.Fprintf(b, "> ⚠ 此条保留内容过长已被截断：原文共 %d 字符，此处仅显示前 %d 字符；上方已校验结构化答案主体不受影响。\n\n", totalRunes, maxAnswerDisplayBodyRunes)
		return
	}
	fmt.Fprintf(b, "> ⚠ This preserved item was truncated for display: %d characters total, only the first %d are shown; the validated structured answer above is unaffected.\n\n", totalRunes, maxAnswerDisplayBodyRunes)
}

func displayAttachmentKey(kind, body string) string {
	if strings.EqualFold(strings.TrimSpace(kind), types.AnswerDisplayAttachmentDiagram) {
		body = diagramBodyDedupKey(body)
	}
	return strings.ToLower(strings.TrimSpace(kind)) + "\x00" + strings.TrimSpace(body)
}

func normalizeDiagramBodyForRender(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if !strings.HasPrefix(body, "```") {
		return mermaidcompat.NormalizeSourceForMarkdown(body)
	}
	lines := strings.Split(body, "\n")
	if len(lines) < 2 {
		return body
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(first, "```") || last != "```" {
		return body
	}
	inner := strings.Join(lines[1:len(lines)-1], "\n")
	return mermaidcompat.NormalizeSourceForMarkdown(strings.TrimSpace(inner))
}

func diagramBodyDedupKey(body string) string {
	body = normalizeDiagramBodyForRender(body)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.TrimSpace(body)
}

func renderV2BlockCaveat(b *strings.Builder, blk types.AnswerBlock, _ answerDocLang) {
	body := renderUserSurfaceProseText(blk.Text)
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
	if t := renderUserSurfaceProseText(it.Text); t != "" {
		parts = append(parts, t)
	}
	if len(parts) == 0 && len(it.Cells) > 0 {
		cells := make([]string, 0, len(it.Cells))
		for _, cell := range it.Cells {
			if visible := renderUserSurfaceText(cell); visible != "" {
				cells = append(cells, visible)
			}
		}
		if len(cells) > 0 {
			// Section/list schemas accept structured cells, so a cell-only item
			// must not disappear merely because the model omitted Label/Text.
			// The renderer contributes only a neutral delimiter; every visible
			// value remains model-authored and in original order.
			parts = append(parts, strings.Join(cells, " | "))
		}
	}
	out := strings.Join(parts, " — ")
	if strings.TrimSpace(out) == "" {
		return ""
	}
	if doc != nil {
		cites := make([]string, 0, 1+len(it.CitationRefs))
		for _, ref := range types.AnswerBlockItemCitationRefs(it) {
			if ref < 0 || ref >= len(doc.Citations) {
				continue
			}
			if cite := renderCitationDisplay(doc.Citations[ref]); cite != "" {
				cites = append(cites, cite)
			}
		}
		if len(cites) > 0 {
			out = out + " (" + strings.Join(cites, "; ") + ")"
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
		for _, ref := range types.AnswerBlockItemCitationRefs(it) {
			if ref >= 0 && ref < len(doc.Citations) {
				return renderCitationDisplay(doc.Citations[ref])
			}
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
	s = normalizeMermaidFencesForAnswerMarkdown(s)
	return strings.TrimSpace(s)
}

// renderUserSurfaceProseText performs one display-only recovery that is safe
// for narrative fields: JSON models occasionally emit the four literal bytes
// `\\n\\n` where they meant a Markdown paragraph break.  The structured value
// remains untouched; only its rendered prose form receives real newlines.
//
// This intentionally is not part of renderUserSurfaceText. Scalar values,
// table cells, relation labels, evidence literals, snippets, and Mermaid bodies
// must remain byte-faithful. Inline and fenced code inside a prose field are
// likewise excluded so a visible escape sequence in an example is preserved.
func renderUserSurfaceProseText(s string) string {
	s = renderUserSurfaceText(s)
	return strings.TrimSpace(normalizeLiteralParagraphEscapesInProse(s))
}

func normalizeLiteralParagraphEscapesInProse(s string) string {
	if !strings.Contains(s, `\n\n`) {
		return s
	}

	var out strings.Builder
	out.Grow(len(s))
	inFence := false
	var fenceChar byte
	fenceWidth := 0
	inlineTicks := 0

	for _, line := range strings.SplitAfter(s, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if width, ch := markdownFenceMarker(trimmed); width >= 3 {
			if !inFence {
				inFence = true
				fenceChar = ch
				fenceWidth = width
			} else if ch == fenceChar && width >= fenceWidth {
				inFence = false
				fenceChar = 0
				fenceWidth = 0
			}
			out.WriteString(line)
			continue
		}
		if inFence {
			out.WriteString(line)
			continue
		}

		for i := 0; i < len(line); {
			if line[i] == '`' {
				j := i + 1
				for j < len(line) && line[j] == '`' {
					j++
				}
				width := j - i
				if inlineTicks == 0 {
					inlineTicks = width
				} else if inlineTicks == width {
					inlineTicks = 0
				}
				out.WriteString(line[i:j])
				i = j
				continue
			}
			if inlineTicks == 0 && strings.HasPrefix(line[i:], `\n\n`) {
				out.WriteString("\n\n")
				i += len(`\n\n`)
				continue
			}
			out.WriteByte(line[i])
			i++
		}
	}
	return out.String()
}

func markdownFenceMarker(line string) (int, byte) {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	ch := line[0]
	i := 1
	for i < len(line) && line[i] == ch {
		i++
	}
	return i, ch
}

func normalizeMermaidFencesForAnswerMarkdown(text string) string {
	if text == "" || !strings.Contains(text, "```") || !mayContainMermaid(text) {
		return text
	}
	return fencedBlockRe.ReplaceAllStringFunc(text, func(match string) string {
		nl := strings.Index(match, "\n")
		if nl < 0 {
			return match
		}
		infoLine := strings.TrimSpace(match[3:nl])
		bodyEnd := strings.LastIndex(match, "\n```")
		if bodyEnd <= nl {
			return match
		}
		body := match[nl+1 : bodyEnd]
		full, ok := answerDocCanonicalMermaidBody(infoLine, body)
		if !ok {
			return match
		}
		full = strings.TrimSpace(mermaidcompat.NormalizeSourceForMarkdown(full))
		if full == "" {
			return match
		}
		return "```mermaid\n" + full + "\n```"
	})
}

func answerDocCanonicalMermaidBody(info, body string) (string, bool) {
	if strings.HasPrefix(info, "mermaid") {
		if directive, _ := mermaidInfoLineDirective(info); directive != "" &&
			firstMermaidKeywordIn(firstNonEmptyTrimmed(body)) == "" {
			if strings.TrimSpace(body) == "" {
				return directive, true
			}
			return directive + "\n" + strings.TrimRight(body, "\n"), true
		}
		return strings.TrimRight(body, "\n"), true
	}
	if kw := firstMermaidKeywordIn(info); kw != "" {
		_ = kw
		if strings.TrimSpace(body) == "" {
			return info, true
		}
		return info + "\n" + strings.TrimRight(body, "\n"), true
	}
	if (info == "" || strings.EqualFold(info, "text")) && looksLikeMermaidBody(body) {
		return strings.TrimRight(body, "\n"), true
	}
	return "", false
}
