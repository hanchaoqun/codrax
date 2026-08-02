# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T10:12:07Z
- sweep_start_ts: 20260802-031206
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_analyze_retry_anchor | PASS | eval/results/read_combo_analyze_retry_anchor-20260802-031207 | answer_regex,answer_contains | none | 160s | 22 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 命中所有词面但机制有三处错误：runAnalyzePhase 实际只检查 IR 指针非 nil，未独立检查 QualityGate；`&AnalysisIR{}` 会被旧代码当成功，和注释矛盾；耗尽后的 recovery IR 由外层 Run 安装，不是 runTaskGraph；MaxRetriesPerStage 在该循环是总语义 attempt budget。发现确定性 typed join gap。 |
| 1 | github_issue_dayjs_duration_nan_symptom | PASS | eval/results/github_issue_dayjs_duration_nan_symptom-20260802-031207 | write_apply,answer_regex | none | 190s | 19 | read=8,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确定位 `Number(undefined)`，仅 patch `src/duration.js` 为 missing component→0，未改已有 PT1H 回归期望。npm runner 缺失后 typed escalation 到 `make check`，Python 行为 oracle 通过；patch 1+/1-、目标路径和验证覆盖一致。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
