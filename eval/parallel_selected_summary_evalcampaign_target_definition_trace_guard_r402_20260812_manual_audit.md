# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T19:39:19Z
- sweep_start_ts: 20260812-123917
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-123919 | answer_regex | none | 126s | 24 | read=0,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Runner only checked names. Final answer no longer says fallback body is out of scope and cites line 24, but the native diagram is still split: Python→export and wrapper→core are disconnected. The exact parser call at Rust line 42 was not projected because the principal member set had 7 members and 8 observation-shaped support refs. Initial diagram invented unsupported call/callback arrows, then one patch consumed a reduced skeleton. B667/B668 filed. |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-123919 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 164s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 233.190ms window preserved. Model answer leads with on-chain 65.912ms compute-supply and 36.757ms D-state/dma_fence, retains PI/scheduling/IO and business-span axes, and labels adjacent/background as non-chain support. Trace causal projection and auto supplement remain present; no answer replacement or fixed-age stream degradation observed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
