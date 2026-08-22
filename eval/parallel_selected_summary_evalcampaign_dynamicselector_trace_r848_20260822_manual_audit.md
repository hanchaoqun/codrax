# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T08:15:37Z
- sweep_start_ts: 20260822-011536
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-011537 | answer_regex,answer_contains | none | 105s | 27 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | uncertain | B1330-B is production-positive: exact rows for indexed write, indexed lookup and entry argument all emit. The answer is substantially better and converges after one relation-metadata retry, correctly explaining JsonPlugin, lookup and decorator-time binding. However no typed dynamic-selection capsule was emitted because the same registry.py:17 occurrence existed as both a deterministic assignment and a grounded registration row; the compiler counted the corroborating carriers as two binding shapes. The visible `resolve -> JsonPlugin — 返回插件类` also blurs `cls()` instance construction into returning a class. B1330-C collapses only same-coordinate/same-endpoint corroboration and preserves different occurrences as ambiguous. |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-011537 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 225s | 39 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | uncertain | Explicit window, full wakeup chain, 11ms on-chain IO root, dual accounting, background isolation and Trace causal projection remain intact with zero finalizer rejection. Model prose nevertheless upgrades the three typed “priority inversion candidates” to “构成优先级倒置结构” before later restoring the candidate caveat, and calls uncalibrated CPU/IO composite indices “low”. Typed system blocks do not make either overclaim. Across r847/r848 this is model-language variation; keep as soft-guidance observation rather than a prose-scanning hard gate or system rewrite. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
