# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T01:49:39Z
- sweep_start_ts: 20260816-184937
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260816-184939 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 192s | 40 | read=2,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit-window causal diagnosis retained the exact five-state account (running 26.946ms, runnable 3.636ms, sleep 84.358ms, D/io 0), on-chain wakeup/rank evidence, supply fold, business span clues, and a Trace causal projection. It answered demand/dependency over supply while keeping frequency headroom secondary. Two rejected read_file calls in a trace-only lane were model tool-adherence noise. Some mechanism prose overstates inferred lock/priority causality, but the typed projection and caveat preserve the evidence boundary. |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260816-184939 | answer_regex,answer_contains,mermaid_edge_count | none | 516s | 42 | read=22,repo_map=3,list=0,trace=0,source_lens=0 | midloop=17,inv=4/0,fin_reject=3,unavail=0,prune=1 | fail | Runner's one-edge/name oracle is too weak. The final graph leaves BusContext and Mutable disconnected while prose claims all stages share and append to the same MutableState. Explorer had already found `Mutable: bus.Mutable`, but the completion repair repeatedly navigated to local Mutable calls and missed `BuildAgentContext(o.busCtx, ...)`; 22 reads, 17 mid-loop hints and three finalizer rejects ended in an honestly unproven but materially incomplete requested data-flow view. Confirmed generalized soft-navigation gap B954, not permission to synthesize an edge. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap decision

- `B954-CARRIERARGNAV1` (P1, confirmed): the relation/argument evidence vocabulary and hard validator already support an exact declared carrier passed as a complete call argument across languages. The completion navigator, however, indexed only relation endpoints. A call such as `BuildAgentContext(o.busCtx, ...)` therefore had no searchable `BusContext` endpoint and lost to unrelated local receiver calls.
- General fix: join parser-owned declared type bindings to complete arguments at parser-owned call sites in the same bounded source files, and use the result only to rank the next surgical read. The model must still read the line and author the `argument -> receiver` evidence; the system does not close participant coverage, create a relation, alter direction, or write the diagram.
- Scope guard: the existing Trace-family exclusion remains intact. No request/final-prose keyword scan, no answer mutation, and no change to explicit-window causal projection or automatic trace supplementation.
