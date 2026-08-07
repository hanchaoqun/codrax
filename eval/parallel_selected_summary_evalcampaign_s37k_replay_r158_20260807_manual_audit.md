# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T10:09:13Z
- sweep_start_ts: 20260807-030911
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260807-030913 | write_apply,answer_regex | none | 196s | 21 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | Patch is correct and the repo-owned Python oracle passed. Node was unavailable, so six TypeScript behavior cases did not run and `production_verification_source_static_only` is an honest unverified verdict. Native JSON object/array teaching worked on the first emit; a later exact-contract quality rejection caused a full analyzer retry, and the retry deleted all model-authored behavior contracts instead of repairing only the offending row. |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260807-030913 | answer_regex,answer_contains | none | 248s | 27 | read=5,repo_map=0,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=5,unavail=0,prune=0 | uncertain | S37k worked in production: the requested comparison table and Mermaid survived. However two explorer rows merely selected tool-schema names inside condition branches but were typed as direct calls, so the strict diagram gate correctly rejected the same false edges four times before the model changed them to precedence. Mechanical citation repair then replaced two correct guard/callsite refs with weaker `Name()` definition refs. The final answer is useful but still contains wording drift (`forceFullEmitNext` described as discarding an existing patch, and the summary overstates patch-base selection), so runner PASS is not a full human pass. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- S37k production result: `EVAL-B251-PERMEMBERTABLE1`, `EVAL-B252-COMPMATRIXAXIS1`, and `EVAL-B253-WRITEJSONCARRIER1` are production-closed by r158.
- New deterministic system conflict: `EVAL-B255-CALLANCHORAUTH1`. A lexical name mention was published as typed call authority, while the downstream hard gate required an actual invocation. The correct fix is to downgrade the upstream relation authority from exact typed source facts, not weaken the downstream visible-edge gate.
- New monotonicity bug: `EVAL-B256-CITATIONMONOTONE1`. A later label/definition fallback may not overwrite a current citation whose source quote corroborates an explicit item-local code surface.
- New P1 write-analysis issue: `EVAL-B257-WRITEEXACTRETRY1`. One under-grounded exact behavior row currently triggers a full LLM re-emit, allowing all otherwise valid typed contracts to disappear. The next batch should deterministically calibrate only rejected contract authority and preserve the rest.
- `EVAL-B254-COMPINVROUTE1` remains open: r158 removed broad source-lens calls but the bounded comparison still took 248 seconds and five finalizer retries.
