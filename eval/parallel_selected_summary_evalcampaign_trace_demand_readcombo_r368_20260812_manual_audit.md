# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T06:38:18Z
- sweep_start_ts: 20260811-233816
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260811-233818 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 43 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit window, target-state account, typed wakeup chain, on-chain-only ranking, occupancy/eliminable dual axes, 10.331ms compute-supply seat, D/IO, semantic work, business clue, background demotion, causal projection and automatic supplement are intact. The model-authored lead nevertheless sums overlapping 23.994ms and 19.041ms seats into 43.035ms and upgrades priority-inversion candidates into the primary proven mechanism, while the same answer's typed projection explicitly says `成员区间重叠,合计不可直加`. This repeats a known model fluctuation with adequate contrary typed context; do not add request/final-prose scans, hard rejection, or system-written conclusions. |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260811-233818 | trace_attachment,answer_regex | perf_triage+trace_query | 250s | 46 | read=9,repo_map=1,list=0,trace=2,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B616 is production-positive: the impossible per-subtopic repository-anchor gate disappeared, completion downgrades fell from 13 to 0, there was no checkpoint restart, and runtime fell from 684s to 250s. No patch was needed, so B617/B618 remain targeted-positive but production-unexercised. The answer accurately preserves the 86.111ms external observation and source/runtime evidence boundary, but B611/B612 remain partial: it cites only the `FindSpanWindows` wrapper/early callee gutter yet claims the unseen correlation implementation as generic “same PID B/E pairing”. Current code actually keys synchronous LIFO state by exact physical source plus emitter TID, applies lifecycle/malformed resets, and fails closed on unresolved provenance. It also conflates “measured duration is an investigation clue” with the stronger deadline/jank verdict; absent refresh/deadline authority should withhold only the latter. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
