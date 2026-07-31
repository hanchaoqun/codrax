# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T22:22:38Z
- sweep_start_ts: 20260731-152237
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260731-152238 | write_apply,answer_regex | none | 233s | 18 | read=15,repo_map=3,list=2,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail-system | AF1 生效：risk=medium、自动执行；补丁正确且 `make check` 通过，但 changed-path gate 把 typed root `make@.` 错当 C/C++ runner，拒绝授权同根 Rust 路径，最终 write_report_failed。 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-152238 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 266s | 47 | read=5,repo_map=0,list=0,trace=6,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=1,prune=3 | fail-human | AE 生效：5 sync/10 oneway、transaction 12145859 `code=0x19`、1.409ms occurrence、on-chain rank board 均正确；但 blocking authority 明示 lower_bound，正文仍写“总计/全部/唯一”，并把 50 条 blocked-reason（Σ16.358ms）外推成全部 65 段/70.338ms 睡眠。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed generalized gaps

1. `EVAL-B12-AG1`（P1，cross-language project verification）：Make 是
   language-agnostic meta runner；filesystem-derived `make@.` 明确存在
   `check` test signal 且 `make check` 成功，但 changed-path coverage 仍按
   `make -> C/C++` family 相交，导致真实跨语言 behavioral oracle 无权覆盖
   同 project root 的 Rust 改动。修复必须要求执行命令精确匹配 typed
   TestSurface candidate；裸模型选择的任意 Make target 继续 fail-closed。
2. `EVAL-B12-AG2`（P1，typed lower-bound answer authority）：blocking
   authority 已给出 `coverage_status=lower_bound_capacity_truncated`，soft
   prompt 仍不能稳定阻止模型写 exhaustive wording。应由 typed authority
   无条件生成用户可见 coverage caveat，不读取或扫描模型答案。
3. `EVAL-B12-AG3`（P1，census-to-duration caliber）：blocked-reason census
   是 caller-linked record census；50 条 caller 记录及其自报 delay 总和不能
   自动覆盖 65 段 scheduler sleep/70.338ms。需要 typed caliber disclosure，
   禁止从 occurrence census 外推完整 state-duration partition。

## Preserved positive controls

- H1 的显式 233.190ms 用户窗、Trace 因果投影、自动补采、目标四态、
  wakeup chain、根因排序和窗内可消除量均在。
- IPC request census 是 complete，允许完整发布 5 sync + 10 oneway；
  target blocking wall-clock 是 lower bound，两者必须继续分型。
- PyO3 的 high-risk 模型波动已由 AF1 soft rubric 消除；审批门未放宽。
