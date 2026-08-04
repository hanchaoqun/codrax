# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T07:17:54Z
- sweep_start_ts: 20260804-001752
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_command_current_source_explanation | PASS | eval/results/read_combo_command_current_source_explanation-20260804-001754 | answer_regex | none | 86s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Runner 的宽松 regex 把“失败说明”签成 PASS。分类器发出 raw_route=hybrid、needs_repo=true、current_source=required、source=mixed，但主 operation 漂成 computer_operation；guard 又归一到 operation。独立 command planner 连续三轮生成未正确 quote/缺少 -name 的 find 命令并全部失败，pipeline_dispatches=0，既没有统计值也没有读取源码，更没有 command_measurement typed carrier 或 B64 evidence-path handoff。 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260804-001754 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=3/2,fin_reject=0,unavail=0,prune=0 | fail | B66 确定性窗修复生效：exact-window root/rank 与 critical 补采、projection anchor、principal_state、因果投影均为 2.000..2.020 / 20.000ms，真实占时与现规则可消双轴也正确。人工仍判 FAIL，因为模型摘要首句把窗后 2.020020 switch-in 混成“窗内睡眠 20.020ms”，与同页 typed 20.000ms 冲突；系统没有改写正文。属于上下文显著性/模型取值残余，不得靠答案文本扫描硬改。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
