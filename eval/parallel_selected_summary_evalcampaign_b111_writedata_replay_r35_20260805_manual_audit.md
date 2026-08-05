# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T13:31:31Z
- sweep_start_ts: 20260805-063130
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_memoclaw_text_search_multirepo_ts | PASS | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260805-063131 | log_regex,write_apply,write_patch_oracle | none | 179s | 20 | read=9,repo_map=1,list=4,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Patch correctly changes the client to POST `/v1/search`, emits a JSON body with query/limit and optional namespace, preserves unrelated APIs/tests, and `make check` passes. Delivery is correctly verified. B109's hard behavior-contract proof bridge was not exercised: all four planner outcomes were soft `operator=satisfies` entries from `expected_outcome_fallback`, so this is not production proof of that arm. Keep contract-strength quality on heterogeneous replay watch; do not promote model prose to a hard contract via keyword scanning. |
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260805-063131 | log_regex,answer_regex | none | 220s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final source-domain contribution ledger is GroupA=10/7, GroupB=4, GroupC=5; reconciliation is 17/4/5; final projection is 17/0/5. The ordinary semicolon/cardinality output mismatch correctly stayed assemble-only, separate from reference-domain repair. This run started with the correct domain, so B110 replacement generation was not production-exercised. Five failed actions remain: the planner emitted unsupported `read_instructions`, staging let it reach execution, and a later missing-input error masked it. B112 adds first-precedence typed action-kind admission. Intermediate model thoughts invented values/labels, but typed ledgers contained them; treat as contained model fluctuation, not an answer-text hard-gate target. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generic findings

- `EVAL-B111-WRITEPROOF-REPLAY`: human pass; B109 hard-contract proof-follow-up arm remains structurally pinned but production replay-inconclusive because the emitted contracts were soft.
- `EVAL-B111-DATAREF-REPLAY`: human pass; source contribution, reconcile, and final projection domains are audit-correct. B110's replacement arm remains structurally pinned but was unnecessary in this run.
- `EVAL-B111-ACTIONKIND1` (P1): confirmed generic admission-order gap. An unknown typed action enum was not rejected before field/input/stage checks and execution, burning a repair round. B112 rejects it at staging from the capability registry and supplies current typed legal actions.
- Model-only intermediate value/name hallucinations did not enter typed artifacts or the final answer. No prose scanner, normalizer, or system-authored conclusion is warranted from this one run.
