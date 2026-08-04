# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T06:30:52Z
- sweep_start_ts: 20260803-233051
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_command_current_source_explanation | PASS | eval/results/read_combo_command_current_source_explanation-20260803-233053 | answer_regex | none | 130s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | classifier 输出 raw_route=hybrid、operation=investigate、source=mixed、needs_repo=true、current_source=required，却因 needs_operation=true/target_surface=desktop 被 guard 转成 operation lane，analyze→explore→finalize 全部为 0，刚修的 typed command-measurement evidence path 未获得执行机会。operation planner 首个 find 还因 glob 引号丢失失败，重排后虽得到 253，却只用全仓 grep 预览推断链路；最终仍缺 ObservationLedgerInputFromAgentContext/CompileObservationLedger/compileToolResultObservations 真实喉部，并错误加入 answer_evidence_origin/explore_lane_plan 为值传输节点。机器 answer_regex 无法识别整条证据管线被绕过。 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260803-233053 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 34 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、3 次 trace_query、threadpool→network→cookie→app 链、#1 fscache IO、真实占时/规则可消双轴、Trace 因果投影与系统补采均保持。finalizer prompt 已明确 principal_state=20.000ms 且 attachment extent=20.020ms 不得替代，但模型仍把 20.020ms 写成“窗口内总阻塞/完整周期”，表格与投影则正确为20.000ms，暂记模型口径波动而不做答案扫描硬改。确定性系统 gap 是发布器又把 investigator 的历史/repairable aggregate `app-100总阻塞时长=20.020ms, window=2.000000..2.020020` 自动追加为“系统补充：结构化指标摘录 app=20.020ms”，与 deterministic target_window_states 冲突并扩大错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
