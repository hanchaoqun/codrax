# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T02:23:31Z
- sweep_start_ts: 20260830-192330
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260830-192331 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 179s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Exact 2.000–2.020s window and target filters survived all three typed queries. The final answer keeps the proven four-node wakeup chain, ranks threadpool-400's 11.000ms IO wait first, keeps three independent 1.000ms scheduler/priority candidates, separates raw occupancy from rule-eliminable impact, retains business-oriented next steps, and emits one complete Trace causal projection. Adjacent/background rows remain support-only; no fixed-time active-stream degradation or source-code spill occurred. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260830-192331 | answer_regex,answer_contains | none | 298s | 30 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=1,unavail=0,prune=0 | pass | Analyzer accepted required `diagram` plus sibling `member_set` in one typed call-chain request, proving the multi-surface contract fix in production. Final answer correctly renders `buildAnalysisIR -> gate.RunWith` and `gate.Run -> gate.RunWith`, denies a directed path between the requested endpoints, and lists key functions in a separate member block. One finalizer reject exposed model omissions (one endpoint edge/anchor, exact visible labels, member facet); two bounded patches repaired metadata without stale-loop, relation invention, whole-answer replacement, or degraded recovery. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
