# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T20:09:08Z
- sweep_start_ts: 20260808-130907
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260808-130908 | answer_regex,answer_contains,mermaid_edge_count | none | 590s | 37 | read=9,repo_map=4,list=1,trace=0,source_lens=1 | midloop=10,inv=2/0,fin_reject=11,unavail=0,prune=0 | fail | Explorer 仍没有形成请求组件间的已证完整数据流。Finalizer 先画无证组件流，随后在 `call`/`return`/`precedence` 间反复改写同一 `Orchestrator -> runAnalyzePhase` 假关系，11 次 reject 后降级为零边节点图；正文却继续宣称 Orchestrator 顺序调用所有 agents，证据不支撑。B378 的旧无条件 presentation-only 逃逸句已从实际错误中消失，但模型没有消费 typed recipe。FRCAP 最终又附上一份几乎相同的第一稿整页，B369 需按结构语义 identity 继续收窄。 |
| 2 | qf_diagram_pipeline | FAIL | eval/results/qf_diagram_pipeline-20260808-130908 | answer_regex,answer_contains,mermaid_edge_count | none | 835s | 41 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=8,inv=1/0,fin_reject=20,unavail=1,prune=0 | fail | 首稿四阶段文字和图形顺序正确，但节点写成 `A[analyze<br/>StageAnalyze]` 等“展示值 + 代码身份”。validator 固定取第一段 `analyze` 作为 endpoint，因而无法与 `StageAnalyze` typed evidence 对齐；20 次 reject 在删 anchor、错引 citation、错误 call/precedence 之间震荡，最终删光箭头。确认 `EVAL-B379-DIAGRAMMULTISURFACEIDENTITY1=P0/HIGH`：endpoint 解析应在标签的多个 exact identity 中，只接受 citable evidence 唯一命中的一个；多个 distinct typed 命中必须 fail-closed，不能按第一段、大小写或语言猜。Explorer 还把四项 ordered carrier 手写成一条非相邻 `StageAnalyze -> StageExtract`，但严格 read-context 已足够让 pre-emit 确定性恢复相邻关系，B379 阻断了该恢复。最终又展示 alias-only 不同但可见内容重复的第一稿，B369 重新记 partial。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: `0/2`; human full correctness: `0/2`。
- B378 的词面单源修复已生效：实际错误不再宣称 typed-flow 可以通过删 `edge_anchors` 进入 presentation-only；模型仍尝试旧修法属于未遵循精确提示，不是合同再次自相矛盾。
- 新最高杠杆项是 B379：Mermaid label 同时含展示值与 canonical identity 时，首段固定投影污染 typed endpoint。它使原本可由严格已读 source carrier 证明的正确首稿被系统错拒。
- B369 从“exact typed carrier 同一”闭环退回 partial：alias/ID 变化使结构字节不同，但用户可见答案仍整页重复；后续只能做 typed diagram semantic canonicalization，不能靠正文相似度或关键词去重。
- 两案共 31 次 reject，性能瓶颈是关系身份错拒和修复震荡，不是 JSON decode、Mermaid renderer、SQL 或 trace 转换。
- 无 malformed JSON（logic 的 blocks JSON-string 被既有容错恢复且不是失败根因），无用户/模型原文关键词硬门，无系统替写模型结论，无 Trace 查询。后续继续保持 Trace 显式窗、自动补采、链上根因和邻近/背景边界。
