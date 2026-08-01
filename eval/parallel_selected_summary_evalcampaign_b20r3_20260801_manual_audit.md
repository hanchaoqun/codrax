# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T12:26:38Z
- sweep_start_ts: 20260801-052636
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260801-052638 | write_apply,write_patch_oracle | none | 225s | 18 | read=8,repo_map=2,list=1,trace=0,source_lens=2 | midloop=2,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Correct one-file production patch; the unchanged project suite executes 4/4 PASS. Three newly model-authored Python probes contain an extra closing parenthesis and correctly become `verification_probe_syntax_error`, so final proof remains weak. W5 works: stdout ends with “最终交付状态：未完全验证”, names the cumulative batch and `verification_proof_incomplete`, and matches final JSON. Safe model variance; no language/case hard gate. |
| 1 | github_issue_gson_lazy_number_symptom | PASS | eval/results/github_issue_gson_lazy_number_symptom-20260801-052638 | write_apply,write_patch_oracle | none | 231s | 19 | read=12,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Correct one-file `equals`/`hashCode` patch; `make check` source oracle passes. Java probes and Maven are unavailable, so cumulative proof remains honestly unverified. W5 works: the last stdout card is the terminal unverified verdict with the exact batch/reason, after both generation-local pass cards, and matches final JSON. No false verified claim remains. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
