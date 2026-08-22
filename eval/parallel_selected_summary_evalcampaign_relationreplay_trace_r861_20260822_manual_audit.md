# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T13:01:44Z
- sweep_start_ts: 20260822-060142
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-060144 | answer_regex,answer_contains | none | 168s | 28 | read=2,repo_map=1,list=1,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=0 | partial | No recovered stale draft and no relation-repair loop: final answer correctly identifies JsonPlugin, separates import-time @register binding from runtime resolve, explains REGISTRY lookup/cls instantiation/executor callback, follows the TimestampMixin -> ValidationMixin -> BasePlugin MRO, and renders five explicit reader-facing relations in the principal list. The model chose no Mermaid block, so B1344/B1345 did not naturally trigger. Two item citations are materially imprecise despite correct prose: “resolve 查找 REGISTRY” cites runner.py:15 rather than registry.py:31, and “resolve 调用 cls” cites register definition/assignment at registry.py:11/17 rather than cls() at line 34. The pre-emit checker found candidate locations but accepted them as a soft advisory and did not expose executable candidate evidence IDs. |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-060144 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 202s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 2.000000..2.020000s main window, complete threadpool-400 -> network-300 -> cookie-200 -> app-100 typed wakeup chain, 11.000ms on-chain iowait first seat, three independent 1.000ms scheduling/priority candidates, actual-time/removable double account, business drilldown, background separation, and full Trace causal projection all survive with zero finalizer rejects. Model prose again slightly overstates the fscache wait plus sequence as stalling the entire dependency chain; keep as a soft wording observation, never a prose hard gate or system-authored conclusion. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
