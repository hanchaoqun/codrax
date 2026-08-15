# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T23:33:52Z
- sweep_start_ts: 20260815-163350
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260815-163352 | answer_regex,answer_contains | none | 271s | 29 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=14,inv=5/0,fin_reject=2,unavail=0,prune=0 | pass | 源码不存在 buildAnalysisIR→gate.Run 的有向路径；答案诚实声明两者分别调用 gate.RunWith，并以两条同向 typed call 边画出汇合边界，未捏造顺序。第 1 稿把未逐边认证的中间调用画入图，第 2 稿存在 endpoint identity 不一致，第 3 稿收敛。相较 r535 的 1201s 超时，本轮 271s 完成，宽对称图未再触发同构枚举失控；14 次 explore midloop 偏高，但既有 no_directed_path 逃生合同已在场，当前归为模型收敛波动，不加 case 硬门。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260815-163352 | answer_regex,answer_contains | none | 347s | 38 | read=12,repo_map=6,list=0,trace=0,source_lens=1 | partial | Mermaid 保留 Analyze→Explore→Extract→Finalize 全阶段 precedence，表格完整列出四阶段输入/输出/载体；仅一次拒绝后 patch 成功，r535 的 6 次 cardinality 瀑布与 3 图降级均消失。正文声称 AnswerDocumentV2 自身携带 AnswerContract 校验结果，与类型定义不符；图展示的是阶段 precedence 与 dispatch 支撑关系，不是每个载体的具体 handoff。精确关系门正确阻止模型把未证载体 incidence 画成边；错误位于模型概括，先记异构观察，不扫描或改写正文。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
