package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_post_emit_advisory.go — V3-3 (§40.51): the finalizer's
// post-emit advisory lanes are ONE table, ONE latch, ONE round.
//
// EVOLUTION RECORD: Observe used to evaluate four independent one-shot lanes
// (requested dimensions / dimension order / external-observation selectors /
// trace primary-cause entity) in fixed order and returned on the FIRST lane
// with findings, so a document missing all four paid four sequential
// emit_answer_document_patch rounds — each a full finalizer ReAct iteration
// — and received four separate "your emit landed, but…" disclosures for the
// same document. The lanes are now rows of postEmitAdvisoryLanes: every row
// runs in the same observation, the findings are merged into one disclosure
// behind one preamble, one latch (postEmitAdvisoryDelivered) closes the
// pass, and the pass never touches hard-reject accounting (retriesUsed,
// rejectHintsUsed, emitFullDocFailStreak, the empty-blocks breaker, the
// patch/full-emit preference). All rows are soft guidance by red line:
// none may become a hard reject, and a rejected advisory patch ships the
// previously accepted document (Observe's Stop branch — the typed escape).

// postEmitAdvisoryLaneKind is the closed set of post-emit advisory rows.
// Internal routing / telemetry only — never rendered into a prompt.
type postEmitAdvisoryLaneKind string

const (
	postEmitAdvisoryRequestedDimensions          postEmitAdvisoryLaneKind = "requested_dimensions"
	postEmitAdvisoryRequestedDimensionOrder      postEmitAdvisoryLaneKind = "requested_dimension_order"
	postEmitAdvisoryExternalObservationSelectors postEmitAdvisoryLaneKind = "external_observation_selectors"
	postEmitAdvisoryTracePrimaryCauseEntity      postEmitAdvisoryLaneKind = "trace_primary_cause_entity"
	postEmitAdvisoryTraceRootCauseSelection      postEmitAdvisoryLaneKind = "trace_root_cause_selection"
)

// postEmitAdvisoryLaneClass records why a row is soft: precise_repair rows
// read typed structure (analyzer dimensions, schema-published selectors) and
// ask for a bounded surface repair; advisory rows offer a model-owned choice
// or read a NOISY signal (G13 substring entity matching) and may only suggest.
// Both classes are hints —
// the class exists so a future row cannot be promoted to a reject without
// changing the table.
type postEmitAdvisoryLaneClass string

const (
	postEmitAdvisoryPreciseRepair postEmitAdvisoryLaneClass = "precise_repair"
	postEmitAdvisoryAdvisory      postEmitAdvisoryLaneClass = "advisory"
)

// postEmitAdvisoryLane is one row: detect runs the lane's detector on the
// accepted document and renders the lane's executable section body (without
// preamble) when it has findings.
type postEmitAdvisoryLane struct {
	kind   postEmitAdvisoryLaneKind
	class  postEmitAdvisoryLaneClass
	detect func(ctx *types.AgentContext, doc *types.AnswerDocumentV2, lang string) (body string, ok bool)
}

// postEmitAdvisoryLanes is the single-source lane table (table order = section
// order in the merged disclosure). Any new post-emit lane is a row here —
// never a new sequential arm in Observe (pinned by
// answer_document_post_emit_advisory_census_test.go).
var postEmitAdvisoryLanes = []postEmitAdvisoryLane{
	{
		kind:  postEmitAdvisoryRequestedDimensions,
		class: postEmitAdvisoryPreciseRepair,
		detect: func(ctx *types.AgentContext, doc *types.AnswerDocumentV2, lang string) (string, bool) {
			missing := requestedAnswerDimensionsRequiringPatchRetry(missingRequestedAnswerDimensionsInDocument(ctx, doc))
			if len(missing) == 0 {
				return "", false
			}
			return requestedAnswerDimensionCoverageHint(ctx, missing, lang), true
		},
	},
	{
		kind:  postEmitAdvisoryRequestedDimensionOrder,
		class: postEmitAdvisoryPreciseRepair,
		detect: func(ctx *types.AgentContext, doc *types.AnswerDocumentV2, lang string) (string, bool) {
			violation, ok := requestedAnswerDimensionOrderViolationInDocument(ctx, doc)
			if !ok {
				return "", false
			}
			return requestedAnswerDimensionOrderHint(violation, lang), true
		},
	},
	{
		kind:  postEmitAdvisoryExternalObservationSelectors,
		class: postEmitAdvisoryPreciseRepair,
		detect: func(ctx *types.AgentContext, doc *types.AnswerDocumentV2, lang string) (string, bool) {
			missing := missingExternalObservationSelectorFactsInDocument(ctx, doc)
			if len(missing) == 0 {
				return "", false
			}
			return externalObservationSelectorCoverageHint(missing, lang), true
		},
	},
	{
		kind:  postEmitAdvisoryTracePrimaryCauseEntity,
		class: postEmitAdvisoryAdvisory,
		detect: func(ctx *types.AgentContext, doc *types.AnswerDocumentV2, lang string) (string, bool) {
			missing := missingTraceProjectionHeadlineEntitiesInDocument(ctx, doc)
			if len(missing) == 0 {
				return "", false
			}
			return traceProjectionHeadlineEntityCoverageHint(missing, lang), true
		},
	},
	{
		kind:  postEmitAdvisoryTraceRootCauseSelection,
		class: postEmitAdvisoryAdvisory,
		detect: func(ctx *types.AgentContext, _ *types.AnswerDocumentV2, lang string) (string, bool) {
			return traceRootCauseSelectionOpportunity(ctx, lang)
		},
	},
}

// B1672: an optional selection opportunity, not a missing-answer defect.
// Read only the frozen selectable roster and accepted/staged report state.
// The shared dispatch latch and loop limits own all continuation policy.
func traceRootCauseSelectionOpportunity(ctx *types.AgentContext, lang string) (string, bool) {
	if ctx == nil || ctx.Mutable == nil {
		return "", false
	}
	mu := ctx.Mutable
	if mu.TraceRootCauseReport() != nil || mu.PendingTraceRootCauseReport() != nil {
		return "", false // Includes an explicit empty choice or withdrawal.
	}
	contract := mu.TraceFindingContract()
	if contract == nil || !contract.RootCauseReportEnabled || len(tracefinding.SelectableRootCauseCandidates(contract)) == 0 {
		return "", false
	}
	if postEmitAdvisoryLangIsZH(lang) {
		state := "尚未收到你提交的根因选择。"
		if mu.TraceRootCauseSelectorRejected() {
			state = "之前提交的根因选择未被接收，具体原因见工具反馈。"
		}
		return state + "完整答案已经保留；这是单独 JSON 报告的可选补充，不要求你改变结论。若愿意补充，请在同一个 `emit_answer_document_patch` 中用 `replace_trace_root_causes` 提交完整有序选择：`{\"schema_version\":2,\"root_causes\":[{\"candidate_id\":\"<从前述可选清单复制的准确ID>\"}]}`，不要照抄示例占位符。只选择你判断成立的链上候选，不必选满清单。若明确不选择任何项或撤回原选择，提交 `{\"schema_version\":2,\"root_causes\":[]}`。也可以继续省略该字段：本次报告会如实表示未取得有效选择，不会因此拒绝正文。只补 JSON 时，用 `unchanged_block_ids` 保留已有块，不为补交而改写正文或图。", true
	}
	state := "You have not submitted a root-cause selection."
	if mu.TraceRootCauseSelectorRejected() {
		state = "The previous root-cause selection was not accepted; the tool feedback gives the reason."
	}
	return state + " Your full answer is retained. This is an optional addition for the separate JSON report, not a request to change your conclusion. If useful, include the complete ordered selection in `replace_trace_root_causes` on the same `emit_answer_document_patch`: `{\"schema_version\":2,\"root_causes\":[{\"candidate_id\":\"<exact ID copied from the offered roster>\"}]}`; do not copy the placeholder. Choose only on-chain candidates you endorse, not every offered item. To record no selected cause or withdraw a selection, submit `{\"schema_version\":2,\"root_causes\":[]}`. You may instead keep omitting the field: the report will honestly indicate that no valid selection was obtained, without rejecting the answer. For a JSON-only addition, preserve existing blocks with `unchanged_block_ids`; do not rewrite prose or diagrams just to fill the report.", true
}

// postEmitAdvisoryHintKey is the ONE hint key of the merged pass (one key,
// one shot: LoopPolicy's per-key cap is irrelevant).
const postEmitAdvisoryHintKey = "answer_doc.post_emit_advisory"

// postEmitAdvisorySignal runs EVERY lane on the accepted document in the same
// observation and returns at most one HintRequested signal per dispatch. It
// reads and writes no hard-reject counter; BypassBudget keeps LoopPolicy's
// mid-loop inject budget untouched.
func (e *answerDocumentEvaluator) postEmitAdvisorySignal(ctx *types.AgentContext, doc *types.AnswerDocumentV2) LoopSignal {
	if e == nil || e.postEmitAdvisoryDelivered || doc == nil {
		return LoopSignal{}
	}
	lang := e.language
	if strings.TrimSpace(lang) == "" {
		lang = extractAnswerDocLang(ctx)
	}
	var kinds []postEmitAdvisoryLaneKind
	var sections []string
	for _, lane := range postEmitAdvisoryLanes {
		body, ok := lane.detect(ctx, doc, lang)
		if !ok || strings.TrimSpace(body) == "" {
			continue
		}
		kinds = append(kinds, lane.kind)
		sections = append(sections, body)
	}
	if len(sections) == 0 {
		return LoopSignal{}
	}
	e.postEmitAdvisoryDelivered = true
	logging.Debug("[diag finalizer] post_emit_advisory lanes=%v", kinds)
	return LoopSignal{
		HintRequested:  true,
		HintKey:        postEmitAdvisoryHintKey,
		Hint:           renderPostEmitAdvisoryHint(sections, kinds, lang),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func postEmitAdvisoryLangIsZH(lang string) bool {
	lang = strings.ToLower(strings.TrimSpace(lang))
	return lang == "zh" || strings.HasPrefix(lang, "zh-")
}

// renderPostEmitAdvisoryHint renders the merged disclosure: one preamble,
// numbered items in table order (each lane's executable body byte-stable),
// one closing rule. When the ordering item shares the disclosure with other
// items the cross-item constraint is stated once: model_block_order cannot
// ride a patch that adds or removes blocks, so the other items should prefer
// replace/local edits over add/remove where possible.
func renderPostEmitAdvisoryHint(sections []string, kinds []postEmitAdvisoryLaneKind, lang string) string {
	zh := postEmitAdvisoryLangIsZH(lang)
	hasOrder, hasSelection := false, false
	for _, kind := range kinds {
		if kind == postEmitAdvisoryRequestedDimensionOrder {
			hasOrder = true
		}
		if kind == postEmitAdvisoryTraceRootCauseSelection {
			hasSelection = true
		}
	}
	var b strings.Builder
	if zh {
		if hasSelection {
			b.WriteString("你的 `emit_answer_document` 已经落地。以下建议合并在本次可选补充机会中：最多只调用一次 `emit_answer_document_patch`，合并你决定采纳的补充；根因选择仍由你决定，可以继续省略。不要重新搜索、编造内容或为填满 JSON 改写已有结论、正文、图和引用；不采纳任何补充时，可用 `unchanged_block_ids` 原样保留已有块。不要写工具外散文。\n")
		} else {
			b.WriteString("你的 `emit_answer_document` 已经落地。下面列出的每一项都只是答案展示面的修订：请只调用一次 `emit_answer_document_patch`，在同一个 patch 里覆盖全部修订项；不要重新搜索或编造没有证据的内容；保留已有结论和引用；不要写工具外散文。\n")
		}
		if hasOrder && len(sections) > 1 {
			b.WriteString("其中排序项使用 `model_block_order`，它不能与 `add_blocks` / `remove_block_ids` 同用：其余修订项能用 `replace_blocks` 或 `block_field_edits_v1` 完成的，优先这样做，让排序能随同一个 patch 提交；只有确需新增或删除块时才使用增删操作，并在本次 patch 中省略 `model_block_order`。\n")
		}
		for i, section := range sections {
			fmt.Fprintf(&b, "\n### 修订项 %d\n\n%s\n", i+1, strings.TrimSpace(section))
		}
		return b.String()
	}
	if hasSelection {
		b.WriteString("Your `emit_answer_document` call landed. These suggestions share this optional follow-up: submit at most ONE `emit_answer_document_patch` combining the additions you choose to make; root-cause selection remains yours and may still be omitted. Do not re-open searches, invent content, or rewrite existing conclusions, prose, diagrams or citations just to fill the JSON report. If you decline every addition, preserve the existing blocks with `unchanged_block_ids`. Do not write prose outside the tool call.\n")
	} else {
		b.WriteString("Your `emit_answer_document` call landed. Every item below is a surface-only revision: submit ONE `emit_answer_document_patch` that covers all of the items together; do not re-open searches or invent unsupported content; preserve existing conclusions and citations; do not write prose outside the tool call.\n")
	}
	if hasOrder && len(sections) > 1 {
		b.WriteString("The ordering item uses `model_block_order`, which cannot be combined with `add_blocks` / `remove_block_ids`: prefer `replace_blocks` or `block_field_edits_v1` for the other items so the ordering can ride the same patch; only when a block truly must be added or removed, use those operations and omit `model_block_order` from this patch.\n")
	}
	for i, section := range sections {
		fmt.Fprintf(&b, "\n### Item %d\n\n%s\n", i+1, strings.TrimSpace(section))
	}
	return b.String()
}

// requestedAnswerDimensionOrderHint renders the layout-only ordering item.
// The "keep every block byte-identical / no add or remove" constraint is
// scoped to THIS item so the merged disclosure never forbids the content
// repairs a sibling item asks for.
func requestedAnswerDimensionOrderHint(violation requestedAnswerDimensionOrderViolation, lang string) string {
	idsJSON, _ := json.Marshal(violation.ModelBlockIDs)
	if postEmitAdvisoryLangIsZH(lang) {
		return fmt.Sprintf("typed 展示顺序要求第 %d 维对应块 `%s` 位于第 %d 维对应块 `%s` 之前，但当前模型块顺序相反。对于本项：在 `emit_answer_document_patch` 的 `model_block_order` 中将当前全部模型自有块 ID 各列一次；其余模型块的完整阅读顺序仍由你选择。当前模型块 ID：`%s`。本项本身不增删块，不要改写正文或关系，也不要移动系统补充块。", violation.EarlierDimensionIndex, violation.EarlierBlockID, violation.LaterDimensionIndex, violation.LaterBlockID, string(idsJSON))
	}
	return fmt.Sprintf("The typed presentation order requires dimension #%d block `%s` to appear before dimension #%d block `%s`, but the current model-authored block order is reversed. For this item: use `model_block_order` in `emit_answer_document_patch` containing every current model-authored block id exactly once; choose the complete reader-facing placement of all other model blocks yourself. Current model block ids: `%s`. This item itself adds or removes no block, does not rewrite prose or relations, and does not move system-generated supplements.", violation.EarlierDimensionIndex, violation.EarlierBlockID, violation.LaterDimensionIndex, violation.LaterBlockID, string(idsJSON))
}
