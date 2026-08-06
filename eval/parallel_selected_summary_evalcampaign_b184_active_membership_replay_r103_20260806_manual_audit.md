# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T15:12:35Z
- sweep_start_ts: 20260806-081233
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_architecture | PASS | eval/results/qf_architecture-20260806-081235 | answer_regex,answer_contains | none | 127s | 25 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | S5 clean replay: model and final answer both distinguish the four unconditional main stages from the two conditional pre-stages. `StageMultiRepoFocus` remains only broad declared capability/background. One completion retry repaired an exact members/support_refs cardinality mismatch; no finalizer rejection or malformed-JSON recovery. |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-081235 | typed_inventory_rowset,dimension_substring,answer_contains | none | 147s | 20 | read=11,repo_map=2,list=1,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Counts, member identities, paths, and packages are correct at 2 extend / 2 foreign func / 8 public class, but the visible citation for `extend Cart` points to `07_foreign_ffi.cj:6` / `native_add`. The exact typed row `Cart.cj:30` was present; bucket-only principal sections skipped the safest exact display-row binder, later fuzzy repairs retained an invalid ref, and the precise alignment warning was soft. Confirmed generalized citation-repair gap, not a Cangjie parser gap. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
