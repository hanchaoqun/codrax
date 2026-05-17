# 最终答案系统附属内容 UX 审计（2026-05-17）

## 范围

本次审计聚焦最终答案面板里所有追加在主体答案前后、由系统派生或兜底保留的用户可见内容：best-effort、fallback、recovered、diagnostic、系统补充说明等。目标是避免非主体内容在视觉上混入已校验的最终回答正文。

## 发现记录

1. `AnswerDisplayAttachment` 保留的原文 / 保留的图
   - 状态：本批已修复。
   - 位置：`internal/render/answerdoc.go`。
   - 问题：说明句是普通段落，文本附件只有较弱的 `--- + bold title`，图附件没有同等强度的上下隔离。长答案里会像最终回答的自然延续。
   - 处理：统一渲染为“系统保留内容”面板：上下分界线、引用样式的醒目系统标签、明确的补充参考说明，以及 `####` 小标题。

2. 文档级 caveats（`doc.Caveats`）
   - 状态：记录到下一批。
   - 位置：`renderAnswerDocV2Caveats`，`internal/render/answerdoc.go`。
   - 风险：当前只渲染为 `**说明**：` / `**Caveats:**` 加列表。模型主动给出的 caveat 可以接受，但系统合成的 caveat 应该考虑使用更清晰的系统说明样式。

3. Missing requested roles / layers
   - 状态：记录到下一批。
   - 位置：`renderAnswerDocV2MissingRequestedRoles`，`internal/render/answerdoc.go`。
   - 风险：这是确定性的系统披露，但现在只是简单粗体标题加列表。在密集的 config-trace 答案里可能和正文混在一起。建议后续做成附录式或引用式说明块。

4. 引用与关键代码附录
   - 状态：记录到下一批。
   - 位置：`renderAnswerDocV2Citations`、`renderAnswerDocV2Snippets`，`internal/render/answerdoc.go`。
   - 风险：这些是常规附录，已有明确标题，优先级低于 recovered output。但当正文以列表/表格收尾时，引用/代码片段仍可能像正文的新章节。后续评估是否需要统一附录边界。

5. Mermaid fallback fence
   - 状态：记录到下一批。
   - 位置：`internal/render/mermaid_render.go`。
   - 风险：解析失败时会保留源码到 fenced block，结构上安全，但如果出现在最终答案正文中，仍可能和用户期望的代码块竞争视觉注意力。后续评估是否只在最终答案上下文里加系统说明前缀。

6. 状态行 / orchestrator notices
   - 状态：本批不处理最终答案层。
   - 位置：`internal/render/orchestrator_notice.go`、status/dock renderers。
   - 观察：这些属于运行中进度面，不是最终答案正文附属内容，已有 glyph/color class。除非后续日志显示它们泄漏进最终答案面板，否则不放入本批整改。

## 分批计划

Batch A：已完成 recovered-output 面板隔离。

Batch B：统一确定性系统附属区块（`doc.Caveats`、`MissingRequestedRoles`）的视觉语言，同时保留模型主动 caveat 的轻量呈现。

Batch C：评估引用/关键代码附录是否需要统一边界，避免正常答案增加噪音。

Batch D：审视 Mermaid fallback 在最终答案面板里的呈现，决定是否只对 fallback fence 增加系统说明包装。
