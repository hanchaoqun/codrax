# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T06:51:00Z
- sweep_start_ts: 20260801-235059
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_member_set_closure_scope | PASS | eval/results/read_combo_member_set_closure_scope-20260801-235101 | answer_regex,answer_contains | none | 152s | 20 | read=3,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 模型原始 Markdown 表已经给出 11 个类型及逐项职责；确定性枚举编译器随后把它改造成「项目/说明」表，11 行说明全部为空。runner 只验名称/数量而误判 PASS；这是系统覆盖模型内容，不是模型漏答。 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260801-235101 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 416s | 51 | read=15,repo_map=0,list=0,trace=6,source_lens=0 | midloop=3,inv=1/0,fin_reject=2,unavail=0,prune=4 | fail | 显式窗、两维占用/可消除、根因榜、唤醒链与 Trace 因果投影均在，自动补采未丢。但模型把策略/频率观测升级为「热限压/热节流」主根因，并同时声称修向有重叠、频率方向又完全独立；typed 边界明确频点/策略上限不能单独证明热机制。属于模型结论越权与内部矛盾，系统不得靠改写答案修。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
