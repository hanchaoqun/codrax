# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T20:12:49Z
- sweep_start_ts: 20260805-131247
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260805-131249 | log_regex,answer_regex | none | 54s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final artifact is byte-exact `{"ids":["u1","u3"]}`. Prebound-helper teaching worked: no helper import/redefinition failure. Initial plan still omitted the typed required `instructions.md`; the runtime guard rejected it and the model repaired once. Because the precise required-path contract was already present and the final execution was correct, retain this as model-variance telemetry rather than add another gate. |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260805-131249 | answer_regex,answer_contains | none | 212s | 20 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=5,unavail=0,prune=0 | fail | Correct final class and declared MRO roster, but the answer says `JsonPlugin.handle` is inherited from/finally defined by `BasePlugin`; Python lookup actually selects `TimestampMixin.handle`, which delegates through `ValidationMixin` to `BasePlugin`. Finalizer also spent five rejects on an optional graph and repeated removal bookkeeping. The typed relation capsule still omitted all three `JsonPlugin` base edges despite exact source reads; the recovered rejected-diagram attachment reappeared after the accepted no-diagram patch. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
