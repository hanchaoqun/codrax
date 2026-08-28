# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T20:35:36Z
- sweep_start_ts: 20260828-133535
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-133536 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Explicit 2.000..2.020s window, app-100 state partition, target-filtered timeline/wakeup/root-rank queries, four-hop threadpool→network→cookie→app chain, 11.000ms on-chain IO first seat, three separate 1.000ms lower-priority runnable candidates, actual-occupancy/rule-eliminable accounts, background isolation, auto-supplement and full Trace causal projection all survive. The model nevertheless calls the sleep “designed downstream waiting” and recommends page prefetch/cache warm-up plus cross-CPU wake efficiency even though the supplied reader-facing evidence explicitly says the call-site does not identify an object/subsystem and cannot authorize prefetch/cache remedies. This repeats B1269/B1271 model adherence variance with accurate contrary typed context; do not add request/final-prose scans, rejection, or system-written conclusions. |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-133536 | answer_regex,answer_contains | none | 174s | 27 | read=18,repo_map=0,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=0,unavail=0,prune=0 | fail | B1404 production-positive: the legal CLI literal no longer becomes a stale durable denial and the generic contradiction caveat is absent. Default 50, YAML overlay and CLI Changed guard are correct. The answer still never reads LoadRuntimeSettings' yaml.NewDecoder/KnownFields/Decode body, so it substitutes a struct tag for the requested parse operation (B1405). It also says codrax.yaml.example was not directly read although the run performed an exact grep and grounded lines 485/487; finalizer retained the stale truncation advisory but omitted the resolving evidence (B1396 witness). A separate soft validator misreads scalar 50 as a visible member count against a four-row aggregate (B1407); no retry occurred, but the contract signal is false. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
