# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T16:47:21Z
- sweep_start_ts: 20260821-094720
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-094721 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 206s | 42 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 精确 2.000000..2.020000 窗、四节点三唤醒边、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双账、邻近/背景隔离、自动补齐及 Trace 因果投影均完整；未按 4ms/4m 或活动流年龄降级。模型仍把 `fscache_page_wait_on_page_bit` 调用点扩写成 fscache 缓存未命中/页面回收方向，且一张摘要表的表头退化为“列 2…列 7”；事实边界段随后正确披露对象/后端/直接阻塞未证，故记呈现与措辞 partial，不增加原文硬门或系统改写。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-094721 | answer_regex,answer_contains | none | 376s | 53 | read=20,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=3/0,fin_reject=3,unavail=0,prune=0 | partial | 最终答案、四阶段输入/输出/状态载体表和合法 sequenceDiagram 均在；图只保留 analyze→explorer→extractor→finalizer 三条 typed precedence，Orchestrator/BusContext 等交互仍只在正文和表中，关系表达偏薄。B1298 本轮只命中装饰成员 support-ref 的纯形债，模型下一轮自行去装饰并给出逐成员 ref，未触发 semantic-grounding 隔离生产形。第二次成文的 thinking 声称会选 `addition_ref`，但实际 JSON 三条 add 均漏掉 ref、只给 block_id+edge，故 legacy add 缺 relation_kind 被拒是正确行为；下一轮提交 live ref 后通过。执行层及生产接线回归也钉住 ref 形可省略 relation_kind，不能误报合同冲突。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
