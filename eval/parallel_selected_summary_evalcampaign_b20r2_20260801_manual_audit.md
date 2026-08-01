# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T12:09:18Z
- sweep_start_ts: 20260801-050916
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_gson_lazy_number_symptom | PASS | eval/results/github_issue_gson_lazy_number_symptom-20260801-050919 | write_apply,write_patch_oracle | none | 183s | 19 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Production diff is the intended one-file equals/hashCode repair and the independent `make check` source oracle passes. Java probe and Maven are unavailable, so cumulative proof correctly ends `unverified/verification_proof_incomplete` instead of being washed back to verified. However stdout ends with two identical “测试通过” cards and never publishes the terminal unverified verdict or missing-JDK boundary; the user-visible result contradicts the final typed report. General terminal write-status publication gap, not a Java special case. |
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260801-050919 | write_apply,write_patch_oracle | none | 250s | 19 | read=8,repo_map=3,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One-file constructor normalization is correct and tests are unchanged. The bounded probe passes, then W1 explicitly continues to `python3 -m unittest discover -v` with reason `verification_probe_missing_plan_contract_ref`; all 4 tests pass. ChangeReport has project-runner path coverage, no confidence gaps, proof=`strong`, completion=`verified`. Independent replay is also 4/4 PASS. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
