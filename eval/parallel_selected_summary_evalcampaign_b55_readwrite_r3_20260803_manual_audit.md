# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T02:28:14Z
- sweep_start_ts: 20260803-192813
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260803-192814 | answer_regex,answer_contains | none | 385s | 31 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=12,inv=4/2,fin_reject=6,unavail=0,prune=0 | fail | Runner false green. Source proves `buildAnalysisIR -> gate.RunWith` and reverse wrapper `gate.Run -> gate.RunWith`; a principal aggregate member_set bypassed the new exact directed-path gate. Finalizer then exhausted six precise call-edge rejects and degraded by publishing the rejected retry-state draft, whose list and diagram still rename the sink `gate.Run`. |
| 2 | github_issue_commons_lang_random_ascii_symptom | FAIL | eval/results/github_issue_commons_lang_random_ascii_symptom-20260803-192814 | write_apply,answer_regex | none | 535s | 19 | read=12,repo_map=4,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | Product patch satisfies the fixture's static oracle and `make check` passes after replan. Final unverified is nevertheless honest: `javac`/Maven are unavailable, the Make/Python checker is `source_static`, and no Java runner executed either changed path. Do not promote the static checker merely to make the eval green. The new proof-followup refs gate is not expected to fire because workflow terminates on this environment boundary instead of opening another replan. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `EVAL-B56-CALLMEM1` (P0/red-line, confirmed): exact directed endpoint authority ran only inside the branch where a principal aggregate member_set was absent. A grounded unordered roster therefore skipped directionality entirely. Fix: run typed source→sink reachability before the member-set interior-span shortcut; member_set may close span completeness only after reachability succeeds or the model declares typed `no_directed_path`.
- `EVAL-B56-EVALGREEN1` (P1, open): the eval runner reports PASS even when the runtime explicitly says `answer_document_retry_state_recovered`, structured answer checks were skipped, and a rejected draft shipped. Human audit caught it; the declared regex oracle did not. This is an eval-verdict authority gap, not permission to weaken answer contracts.
- The Java write result is not a verifier gap: `make check` proves source shape, not Java execution/behavior. The correct local outcome without `javac`/Maven is `unverified`; environment installation or a runner-backed fixture is required for a verified green.
