# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T00:44:16Z
- sweep_start_ts: 20260815-174414
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260815-174416 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 194s | 46 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | Explicit-window causal projection is present; the main ranking is on-chain only; occupancy/business spans and rule-based eliminable directions are both preserved; adjacent/background rows stay non-causal. Unlike r539, the answer publishes no cross-direction total and repeatedly states physical overlap is unresolved and directions cannot be added. Opening phrase “four independent candidates” is mildly ambiguous but is immediately bounded by the exact unresolved/non-additive statement, so retain as model wording observation rather than a prose hard gate. |
| 1 | github_issue_zod_prefault_symptom | FAIL | eval/results/github_issue_zod_prefault_symptom-20260815-174416 | write_apply,answer_regex | none | 198s | 25 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | pass-with-caveat | First `make check` genuinely failed because the initial model-authored tests did not match the fixture contract; typed failure handoff caused a legitimate replan and the second check passed. After that pass, B866 prevented any proof-repair replay: no proof-followup batch was appended and normalization finished directly as honest `production_verification_source_static_only`. Correct patch retained; machine FAIL reflects absent TypeScript runtime behavior authority, not code failure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
