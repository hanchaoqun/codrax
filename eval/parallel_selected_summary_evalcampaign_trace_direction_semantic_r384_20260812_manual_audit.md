# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T11:41:23Z
- sweep_start_ts: 20260812-044121
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h10_spantop_member_subrows | PASS | eval/results/real_trace_h10_spantop_member_subrows-20260812-044123 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 143s | 35 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=2,prune=0 | fail | B638/B641 positive: model names the two distinct JIT spans with 1.781/0.607ms and exact line ranges, and does not claim target-thread execution. Residual is deterministic: system projection still publishes the same exact family once as adjacent E47 and again as background E52; the answer also juxtaposes a zero-result keyword probe with the non-empty typed inventory. |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260812-044123 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 195s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B637 positive: the impossible 42.193ms broad-envelope overlap is gone. B639 was present in Finalizer context, but unsupported model-authored aggregate members remained later in the prompt and reintroduced exact arithmetic strings. The answer again declares four directions independent, sums same-direction seats and says their effects can stack, despite typed leader-only/no-joint-total authority. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
