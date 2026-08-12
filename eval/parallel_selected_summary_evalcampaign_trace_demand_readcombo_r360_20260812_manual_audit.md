# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T04:14:51Z
- sweep_start_ts: 20260811-211448
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260811-211451 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Direction is broadly correct and the system projection preserves the explicit window, four-state account, typed wakeup chain, on-chain-only crown, actual-occupancy/rule-eliminable dual axes, VerifyClass business lead, background demotion and frame-causality caveat. The model-authored lead nevertheless sums CookieMonsterCl 23.994ms + NetworkService 19.041ms to “about 43ms” and compares that overlap-prone same-chain sum with the 10.331ms supply seat, despite the typed projection saying wall-clock seats are non-additive. It also expands priority_inversion_candidate into waiting for a lock without holder/lock evidence. B603 is a near-data caliber/mechanism-authority context gap; do not scan/rewrite final prose. No malformed recovery, system answer, or elapsed fallback. |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260811-211451 | trace_attachment,answer_regex | perf_triage+trace_query | 231s | 36 | read=3,repo_map=1,list=0,trace=2,source_lens=1 | midloop=2,inv=2/1,fin_reject=0,unavail=1,prune=0 | fail | Runtime/source boundary for the 86.111ms measurement is explicit and frame verdict correctly remains unproven. But the answer calls internal/skill/defaults.go:1203 the implementation of B/E pairing. That file stores perf-triage prompt guidance; production pairing is in internal/tracequery/query.go (computeTraceMarksWithInventory/findSpanWindowsCompacted: source+PID stack, B push, unnamed E pop, lifecycle/malformed reset and fail-closed handling). It also momentarily says eventName=print returns buf while discussing tracing_mark_write=buffer. B604 is source-evidence authority misranking: prompt/policy string literals must not satisfy executable-parser implementation authority. One unavailable source_lens repair occurred; no malformed answer recovery, system answer, or elapsed fallback. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
