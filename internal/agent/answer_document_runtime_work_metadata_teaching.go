package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Teaching and coverage inspect the same compiled supply used by the tool
// schema. An empty supply is not a measured work row or a causal verdict.
func runtimeWorkRelationTeachingForContext(ctx *types.AgentContext, lang string) string {
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view != nil && view.RuntimeWorkRelationContract.Active() {
		return runtimeWorkRelationMetadataTeaching(lang)
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "en") {
		return "  - 当前证据没有可选择的精确业务工作耗时记录，schema 未发布 `runtime_work_relation` 回执；省略该字段，不要编造 observation_id 或把调度/IO 状态当作业务工作。请自行解释能确认的事实与仍缺的业务关系证据，在可见的主边界块用 `kind:\"caveat\"`、`surface_role:\"principal\"`、`facet_ids:[\"runtime_work_relation\",\"uncertainty_boundary\"]` 标明此子问的缺证说明。无需添加伪观测 claim 或复述内部枚举。该结构只标识你已说明边界，不表示已经证明关系；也不否定已有的链上调度/IO 证据。\n" +
			"  - 如果现有块已经说明此边界，只补确实缺少的归属；保留正文、图和引用。只有当前 schema 发布精确 `add_facet_id` 时才使用它，否则以 `replace_blocks` 提交完整目标块而非字段片段。系统不选择关系、不扫描或改写正文。\n"
	}
	return "  - Current evidence supplies no exact measured business-work row, so the schema does not publish a `runtime_work_relation` receipt. Omit that field; do not invent observation_id or turn scheduler/IO states into business work. Explain the established facts and missing work-to-target evidence yourself in a visible principal boundary block with `kind:\"caveat\"`, `surface_role:\"principal\"`, and `facet_ids:[\"runtime_work_relation\",\"uncertainty_boundary\"]`. Do not add a fabricated observation claim or repeat internal enums. This metadata only identifies your evidence-boundary explanation; it does not prove a relation or invalidate existing on-chain scheduler/IO evidence.\n" +
		"  - If an existing block already explains this boundary, repair only missing ownership and preserve prose, diagrams, and citations. Use an exact `add_facet_id` only when the current schema publishes it; otherwise submit the COMPLETE target block through `replace_blocks`, not a field fragment. The system does not choose a relation or scan or rewrite prose.\n"
}

func runtimeWorkRelationBlockOwnsCoverage(ctx *types.AgentContext, block types.AnswerBlock) bool {
	if block.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown ||
		block.SurfaceRole != types.SurfacePrincipal ||
		strings.TrimSpace(types.AnswerBlockVisibleSurface(block)) == "" ||
		!answerBlockHasFacet(block, string(types.RequestedAnswerDimensionRuntimeWorkRelation)) {
		return false
	}
	if block.RuntimeWorkRelation != nil {
		return block.RuntimeWorkRelation.IsBound() &&
			answerBlockHasFacet(block, string(types.FacetObservedArtifactFact)) &&
			answerBlockHasClaimForm(block, types.ClaimExternalObservation)
	}
	// This is only a presentation owner for a model-authored lack-of-evidence
	// disclosure. It never mints an observation, receipt, root cause or claim.
	if !answerDocTypedRuntimeWorkRelationRequested(ctx) || block.Kind != types.BlockCaveat ||
		!answerBlockHasFacet(block, string(types.FacetUncertaintyBoundary)) {
		return false
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	return view != nil && !view.RuntimeWorkRelationContract.Active()
}

// runtimeWorkRelationMetadataTeaching is shared by the initial dimension /
// profile instructions and the existing post-emit coverage advisory. It is
// an executable metadata example, not an additional acceptance predicate.
// In particular, a valid receipt is not evidence that the block's ownership
// metadata is complete, nor does a metadata gap invalidate that choice.
func runtimeWorkRelationMetadataTeaching(lang string) string {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "en") {
		return "  - 在模型选择、实际承载运行时工作关系结论的可见主块上，可用隐藏结构 `surface_role:\"principal\"`、`facet_ids:[\"runtime_work_relation\",\"observed_artifact_fact\"]` 与 `claim_uses:[{\"claim_form\":\"external_observation\",\"facet_id\":\"observed_artifact_fact\"}]` 表达归属。模型选择工作行和结论；尚未绑定时，从当前 schema 选择一个精确 `runtime_work_relation:{observation_id,conclusion}`。系统仅显示该 typed 行的工作名、实测时长、关系凭证和未证边界，不扫描或改写正文。隐藏字段和结论枚举不要复述到可见正文或 caveat；渲染器会显示读者用语。\n" +
			"  - 已绑定的 `runtime_work_relation` 不代表块的归属元数据齐全。修补前核对现有块：保持已绑定的 `observation_id` 和 `conclusion`，不要重新选择或只重复提交相同 receipt 来修复元数据缺口；只补缺少的元数据，保留其他已有 facet/claim、正文、图和引用。目标块与关系仍由模型选择。\n" +
			"  - 若缺的是 facet 且当前工具 schema 发布精确的 `add_facet_id` 分支，可按其 `block_id` / `value` 单点补齐；不要把 `facet_ids` 数组塞进 `block_field_edits_v1`。否则用 `replace_blocks` 提交完整目标块：复制全部既有字段（包括 id/kind、可见内容、facet_ids、claim_uses、surface_role 和 runtime_work_relation），只修改实际缺失的元数据；`replace_blocks` 不是字段合并，不能只发送缺失字段。\n"
	}
	return "  - On the model-selected visible principal block that actually carries the runtime-work conclusion, ownership can be expressed with hidden metadata `surface_role:\"principal\"`, `facet_ids:[\"runtime_work_relation\",\"observed_artifact_fact\"]`, and `claim_uses:[{\"claim_form\":\"external_observation\",\"facet_id\":\"observed_artifact_fact\"}]`. The model chooses the work row and conclusion; if not yet bound, select one exact `runtime_work_relation:{observation_id,conclusion}` pair from the current schema. The system only displays that typed row's work name, measured duration, relation credential, and unproved boundary without scanning or rewriting prose. Do not repeat hidden fields or conclusion enum tokens in visible prose or caveats; the renderer supplies reader-facing wording.\n" +
		"  - An already bound `runtime_work_relation` does not establish complete block-ownership metadata. Check the existing block before repair: keep its already bound `observation_id` and `conclusion`; do not reselect or merely resubmit the same receipt to fix a metadata gap. Repair only missing metadata and preserve other existing facets/claims, prose, diagrams, and citations. The model still chooses the target block and relation.\n" +
		"  - If a facet is missing and the current tool schema publishes an exact `add_facet_id` branch, add only that facet using its `block_id` / `value`; do not send the `facet_ids` array through `block_field_edits_v1`. Otherwise submit the COMPLETE target block with `replace_blocks`: copy all existing fields (including id/kind, visible content, facet_ids, claim_uses, surface_role, and runtime_work_relation) and change only the actually missing metadata. `replace_blocks` is not a field merge; do not submit only the missing fields.\n"
}
