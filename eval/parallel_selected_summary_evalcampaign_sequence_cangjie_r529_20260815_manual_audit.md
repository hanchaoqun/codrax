# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T20:42:01Z
- sweep_start_ts: 20260815-134200
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260815-134201 | typed_inventory_rowset,dimension_substring,answer_contains | none | 146s | 26 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 最终 2 个 extend、2 个 foreign func、8 个 public class 的 rowset、文件、符号和 package 全部正确；同名 `native_add` 按文件/package 保持两行，package 来自 typed attribute 而非路径推断。首稿其实已逐行带 package，但 analyzer 把“文件路径”和“包路径”都标为 `source_location`，维度检查误报第 3 维缺失；模型重写时丢失枚举 metadata，patch 被拒，最终保留完整首稿并追加内部化“输出维度核对”。确认 B854，内容未丢但产生无效重试。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260815-134201 | answer_regex,answer_contains | none | 196s | 30 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 结论仍正确：不存在 `buildAnalysisIR -> gate.Run` 有向路径，真实拓扑是 `buildAnalysisIR -> gate.RunWith <- gate.Run`。首稿图已带两条 required edge anchors，B853 无损补锚车道本轮未触发；唯一拒绝是把本地调用混进 principal path facet，修补后把 endpoint boundary 与 supporting local calls 分栏，图关系、方向和 Mermaid 语法保持。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- 两案 runner 与人工均通过。sequence 本轮没有关系丢失，但也没有生产命中 B853；B853 保持“实现与集成 pin 已闭、生产回放待命中”，不再连续复跑同一 case。
- 新确认 `B854-SOURCEATTRIBUTELOCATIONROLE1/P1`：`source_location` 同时承载文件位置与 package 声明，导致软维度检查把已完整答案误判缺列，再诱发模型整块重写和元数据丢失。根修新增 `source_attribute`，覆盖 package/module/namespace；旧模型兼容只依据 typed `source_inventory.requested_fields` 和席位基数纠正，不扫描原问题或答案关键词。
- `source_attribute` 覆盖只消费 exact `source_inventory_row_id`、principal enumeration row 和精确 attribute value；缺失只触发展示软提示，不拒绝答案、不修改模型正文、不代写结论。文件路径继续独立使用 `source_location`。
- 相关 `internal/types`、`internal/tool`、`internal/agent` 全包测试与 `go build ./...` 通过。Trace 路径零改动，显式窗、因果投影、自动补齐与链上-only 根因不受影响。
- 两案在 active bytes 持续期间跨过 4ms/4s 后正常完成；没有固定年龄降级。Cangjie 的旧稿恢复来自第二轮 patch 结构失败，而不是流式超时，完整首稿被保留是正确的保底行为。
