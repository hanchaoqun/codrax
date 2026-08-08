# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T09:00:33Z
- sweep_start_ts: 20260808-020032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-020033 | answer_regex,answer_contains | none | 111s | 23 | read=1,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Four main stages and their responsibilities are grounded and rendered in a valid Mermaid flow. Exploration completed once with no completion/finalizer reject. This replay emitted ten complete evidence objects (the tool legitimately enriched them to thirteen); it did not reproduce the prior adjacent metadata-fragment payload, so B338 is a production no-regression result rather than an exact repair-path closure. |
| 2 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260808-020033 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 155s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | The model-authored lead keeps the proven chain `threadpool-400 -> network-300 -> cookie-200 -> app-100`, names only the on-chain 11.000ms IO wait as root, and separates the 2.014..2.015 1.000ms runnable segment. `logger-900` remains off-chain background with 19.500ms measured occupancy; the 7.350ms effective attribution is not called its actual wait. Typed final context supplied the interval/caliber boundary without rewriting the model conclusion. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
