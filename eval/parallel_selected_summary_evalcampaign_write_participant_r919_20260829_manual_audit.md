# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T03:06:22Z
- sweep_start_ts: 20260828-200620
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260828-200622 | write_apply,write_patch_oracle | none | 181s | 27 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单文件实现修复、probe+unittest 5/5、未改测试；未触发 failed-read，B1431 仅无回归。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-200622 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 310s | 37 | read=12,repo_map=1,list=1,trace=0,source_lens=0 | midloop=7,inv=5/0,fin_reject=2,unavail=0,prune=0 | uncertain | 关系与语法正确但新增隐式端点泄漏内部 hash 名；确认 B1434。B1432/B1433 未自然触发。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

详细人工审计与施工判据见 `eval/manual_audit_evalcampaign_write_participant_r919_20260829.md`。
