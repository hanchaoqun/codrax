# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T13:53:39Z
- sweep_start_ts: 20260819-065338
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_nlohmann_long_double_symptom | FAIL | eval/results/github_issue_nlohmann_long_double_symptom-20260819-065340 | write_apply,answer_regex | none | 191s | 26 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | partial | 两份头文件均正确从 `%.*lg` 改为 `%.*Lg`，`make check` 通过；Make 回执为 `observation_scope=aggregate`，确定性终验拒绝 controller 两次 `all_verified` 并诚实保留 `verification_proof_incomplete`。但本轮不能给 B1165 的 assertion join 签生产正证：模型把 `acceptance_tests` 与 `project_test_observations` 塞进 string-wrapped `changes` 尾部，selected compat 只恢复 changes 并静默丢掉两个 sibling 数组，落盘计划显示 tests=0 且无 observation。这是新 P0 B1168 lossless-recovery gap。两次 unavailable tool 是 planner 在预算后尝试 `read_file`，未改变计划。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-065340 | answer_regex,answer_contains,mermaid_edge_count | none | 727s | 64 | read=27,repo_map=4,list=0,trace=0,source_lens=0 | midloop=24,inv=11/0,fin_reject=7,unavail=0,prune=0 | partial | B1166 获生产正证：第一稿 raw `precedence` 被 exact anchor/body gate 拒绝，最终全部改成模型自写的“顺序进入/调用/传入参数/派发”，系统没有改图。关系图仍分成阶段顺序、Mutable 局部调用、BusContext 参数传递、Orchestrator 派发四个孤岛，未形成请求中的跨阶段数据流；B1167 继续开放。7 次拒绝、24 次 midloop、最大 128,562 tokens（64%），重复注入约 28KB 关系说明，确认 B1169 repair payload/churn gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
