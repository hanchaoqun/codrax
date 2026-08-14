# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T17:55:24Z
- sweep_start_ts: 20260814-105522
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260814-105524 | write_apply,write_patch_oracle,answer_contains | none | 88s | 24 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B813 production positive: one-line `retrun -> return` delivery and project test passed; typed worktree audit disclosed retained untracked `main`, separated it from the clean delivery ref, and did not delete it. |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | eval/results/real_trace_h7_self_seat_full_spectrum-20260814-105524 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 173s | 42 | read=3,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Trace query returned the complete 32-row typed root-cause ranking, including 65.912/36.757/49.623/0.033ms and compaction authority. Analyzer emitted required `causal_attribution` plus required `member_set`, but its first `bounded_fact_set` rejection deterministically taught `bounded_effect_verdict`; the accepted narrow scope then suppressed the root-cause roster and Trace causal projection. This is B817, not oracle drift or missing trace evidence. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

- The write case is a clean production proof for B813. Verification created an
  untracked executable named `main`; the delivery commit remained scoped to
  `main.c`, while the user-visible final report explicitly preserved and
  disclosed the worktree side effect.
- The Trace case is a system classification-repair failure. The original typed
  answer dimensions already distinguished a causal verdict from a requested
  contributor roster (`causal_attribution` + `member_set`). The consistency
  validator ignored the roster cardinality and prescribed the finite
  `bounded_effect_verdict` tuple. Finalization then obeyed that narrow contract
  and omitted the full on-chain ranking/projection even though `trace_query`
  had returned it completely.
- Secondary model issues remain visible (over-interpreting
  `dma_fence_default_w`, calling sleep normal VSync work, and reporting 11
  running fragments where the typed state ledger says 41). They do not explain
  the missing projection and are not justification for answer rewriting or
  case-specific prose gates.
- No malformed-JSON salvage, stale-draft fallback, empty answer, system-authored
  conclusion, or active-stream fixed-age degradation occurred in either case.
