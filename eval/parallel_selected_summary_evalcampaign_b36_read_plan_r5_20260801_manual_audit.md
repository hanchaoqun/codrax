# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T06:12:14Z
- sweep_start_ts: 20260801-231213
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260801-231215 | write_plan,write_patch_oracle | none | 67s | 19 | read=3,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 正确的一行 typo 修复计划，边界稳定。 |
| 1 | sr_java_config_precedence | PASS | eval/results/sr_java_config_precedence-20260801-231215 | answer_regex | none | 82s | 19 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 错误枚举补充已消失，但仍有系统“精确目标命中”前缀、无效 missing-runtime-role 块及通用 caveat；模型核心结论本身正确。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
