# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T01:16:34Z
- sweep_start_ts: 20260819-181632
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-181634 | answer_regex,answer_contains,mermaid_edge_count | none | 432s | 39 | read=17,repo_map=1,list=0,trace=0,source_lens=0 | midloop=12,inv=10/0,fin_reject=0,unavail=0,prune=1 | partial | B1204 生产正证：5 条带中文业务标签的边一次成文即保留，0 次 finalizer reject；旧 Extractor→Mutable 伪边未再进入终稿。B1205 的重复 pair 精确拒绝臂本轮未触发，只保留 unit pin。图诚实把 Mutable 标 unproven，但只画 BusContext→BuildAgentContext→AgentContext，未连接 AgentContext 到各阶段，关系表达仍不足；正文又同时说“BusContext 不持有可变状态”和其包含 `Mutable *MutableState`，存在可见矛盾。30 explorer iterations/12 midloop 仍过高，记 B1208 typed relation/context precision residual，不由系统补边。 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-181634 | log_regex,write_apply,answer_regex,answer_contains | none | 1535s | 26 | read=9,repo_map=2,list=4,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=3,prune=0 | partial | B1203 终态目录镜像获生产正证：final report 同步写入 turn-local 与 canonical plans，最终读到 `complete/unverified`，不再是假 `in_progress`；本轮没有 protected-test 拒绝，候选不夺权分支仍由 unit pin。最终源码正确折叠换行 run，原五换行与普通 merge 两项 unittest 均通过且测试未改。失败源是 planner 自带 Python probe 的 f-string 表达式含反斜杠，执行时报 parser_error；后续 exact project tests 已覆盖 changed path/target_behavior，但未覆盖 probe 的 single-newline 合同，系统正确保持 `verification_proof_incomplete`。新建 B1206：inline probe 在计划接纳前缺语言语法预检。首轮 planner 活跃输出 14m52s/约 24 万字符后 provider length 截断、无 plan，记模型波动与心智负担观察，不按固定年龄硬切。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
