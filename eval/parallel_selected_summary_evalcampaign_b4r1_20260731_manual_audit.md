# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T11:34:07Z
- sweep_start_ts: 20260731-043407
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_gson_lazy_number_symptom | PASS | eval/results/github_issue_gson_lazy_number_symptom-20260731-043407 | write_apply,write_patch_oracle | none | 202s | 18 | read=12,repo_map=2,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Applied tree correctly adds value-based `equals` and matching `hashCode` while preserving every Number conversion method; the fixture oracle passes. Control flow is not clean: after the first green verify, a cumulative verify-only batch inherited the old ChangeReport, filtered `run_tests` out of the callable tool surface, recorded one unavailable `run_tests` attempt, then reused the stale green report and rendered a duplicate “测试通过”. Correct patch, open generalized verification-authority gap. |
| 1 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260731-043407 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 232s | 42 | read=8,repo_map=0,list=0,trace=2,source_lens=0 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit 11.000–11.008s window, two bounded trace queries, and full causal projection were preserved. Engine truth is subtler than the answer: fragmented runnable state does compete, then its exact 5ms account is absorbed into the single formal runnable_wait seat (`rank_family_key=runnable_wait>fragmented_runnable_wait`, `absorbed_rank_rows=1`) to prevent double counting. The answer instead says state_churn is context-only and never competes because the tool contract contains contradictory adjacent rules. It also publishes duplicate state snapshots (19 switches/20 segments vs 20/21) although both raw query payloads contain only 19/20. Route was typed optional, but completion forced eight source reads because ToolBusContext dropped TurnRouteHint at the agent→tool projection boundary. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Automatic: 2/2 PASS.
- Human correctness: 1/2 PASS.
- The Trace failure is a product/system semantics and wiring failure, not an oracle wording preference.
- The Gson patch is correct, but the write verifier’s stale-report reuse is filed separately because a correct result must not conceal a false second verification event.
