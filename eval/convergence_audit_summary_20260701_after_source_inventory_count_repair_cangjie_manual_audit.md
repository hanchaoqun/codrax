# Selected Eval Manual Audit Scaffold

- date: 2026-07-01T06:44:59Z
- sweep_start_ts: 20260701-144459
- total cases: 1
- parallel: 1
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260701-144459 | typed_inventory_rowset,dimension_substring,answer_contains | none | 173s | 23 | read=11,repo_map=3,list=0,trace=0,source_lens=3 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Manual audit confirms the prior visible count drift is fixed: summary renders `extend 块 2 项、foreign func 声明 2 项、public class 8 项`, the public class section renders `public class共 8 项`, and the 8 rows are Bridge, Greeter, Cart, Version, App, Animal, Dog, Service with file/package citations. No `public class 9` / Java supplement pollution remains. Flow is still somewhat read-heavy (`read_file=11`) but uses repo_map/source_inventory first (`repo_map=3`, `list_files=0`, `grep=0`) and closes cleanly (`investigation_complete_calls=1`, `finalizer_rejects=0`). |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
