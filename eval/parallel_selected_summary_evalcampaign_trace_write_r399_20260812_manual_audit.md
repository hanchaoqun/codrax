# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T17:10:41Z
- sweep_start_ts: 20260812-101039
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260812-101041 | write_plan,write_patch_oracle | none | 65s | 23 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan-only case: one `main.cpp` patch changes only `retrun` to `return`; the unified diff passed the plan preflight. No apply/verify claim is made. Analyzer used deprecated `required_answer_dimensions` once despite correct schema/teaching, then the strict decoder's exact suggestion recovered it in one retry. |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-101041 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 160s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B660/B661 are production-positive. The final typed tail carries the exact complete 11-row wait roster and supply-fold equation. The answer lists all 11 rows without residual estimation and states `9.003 + 65.912 = 74.915ms`, with 65.912ms as the only supply deficit. Explicit window, projection, automatic supplement, on-chain roots, business spans, and background separation remain present. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusions

### B660/B661 production closure

The trace Finalizer received both new request-scoped typed relations at its last decision
boundary. `target_wait_enumeration_authority` declares the same-result principal roster
complete at ordinals 1..11 and forbids deriving missing occurrences or residual duration
from an independently capped candidate view. `supply_fold_value_authority` carries the
engine values and exact equation `9.003 + 65.912 = 74.915ms` for the target running seat.

The model consumed both correctly on its first answer attempt: it lists all eleven
D-state intervals and does not claim #9..#11 are absent or estimate about 9.8ms; it names
9.003ms as ideal-equivalent running and 65.912ms as the low-frequency supply deficit.
The independent 12-record / 39.157ms blocked-reason census remains separate from the
11-interval / 36.757ms scheduler account. The explicit 233.190ms user window, complete
Trace causal projection, automatic supplements, typed on-chain principal population,
actual-occupancy versus existing-rule-eliminable axes, business span clues, and
adjacent/background isolation all remain present. There was no finalizer rejection,
answer rewrite, or prose hard gate.

### Write-plan result and JSON observation

The C++ case is deliberately plan-only. Its evidence is a one-line `kind=patch` unified
diff for `main.cpp`, accepted by the patch preflight; it is not evidence that apply or
project verification ran. The first analyzer call used the obsolete field name
`required_answer_dimensions`. This is not a contradictory system contract: the prompt,
tool schema, and repository teaching consistently require `requested_answer_dimensions`,
and strict decoding returned one exact nearest-field diagnostic before the corrected
second call succeeded. Treat this single event as recoverable model fluctuation rather
than silently accepting an alias or adding an output-text gate. The 46k-token analyzer
context for a micro write-plan remains an efficiency/mind-load observation for repeated
cross-case evidence; it is not yet a code-change finding.

Active streamed bytes remain authoritative regardless of cumulative age. Neither four
minutes nor a literal `4ms` permits answer degradation; only caller cancellation/deadline,
first-byte silence, byte stall, or precise transport/decode failure may terminate or
activate a bounded recovery path.
