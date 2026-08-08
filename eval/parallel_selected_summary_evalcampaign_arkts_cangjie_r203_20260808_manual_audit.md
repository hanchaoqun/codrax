# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T09:12:34Z
- sweep_start_ts: 20260808-021233
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260808-021234 | typed_inventory_rowset,answer_contains | none | 146s | 22 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Extraction and accepted aggregate handoff are complete: four `@Entry` types and two `@Builder` functions have exact file:line rows. Finalizer context also renders all six under Principal Enumeration Rows. However, the strict source-inventory gate separately uses the coarse global `principal_roles=function`, classifies the four type rows audit-only, and twice orders the model to remove them. The final answer consequently has an empty `@Entry` section. This is a deterministic conflicting-contract failure, not model variance. |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260808-021234 | typed_inventory_rowset,dimension_substring,answer_contains | none | 196s | 22 | read=8,repo_map=2,list=0,trace=0,source_lens=2 | midloop=4,inv=3/1,fin_reject=0,unavail=0,prune=0 | pass | Final inventory is complete: two extend blocks, both same-name `native_add` declarations remain bound to distinct files/packages, and all eight public-class variants have their exact package/file/line. One count/member mismatch was correctly rejected and one member/support-ref carrier needed repair; no finalizer reject or visible loss occurred. Record the structured handoff churn as P2 process observation, not a language-specific hard fix. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
