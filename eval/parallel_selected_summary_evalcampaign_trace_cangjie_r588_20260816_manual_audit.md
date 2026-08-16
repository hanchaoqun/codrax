# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T21:43:02Z
- sweep_start_ts: 20260816-144300
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260816-144302 | typed_inventory_rowset,dimension_substring,answer_contains | none | 156s | 26 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | All 12 typed rows are present with correct file, symbol, and package: 2 extend, 2 foreign func, and 8 public class rows. The runner false-negative is structural: analyzer emitted `has_per_member_table=true` plus 3 buckets/3 dimensions, but QFComparison required three sections and made the table optional, so the final duplicated each bucket heading and used section items. This is a precise cross-family contract gap, not Cangjie extraction loss or model fact loss. |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-144302 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 201s | 39 | read=2,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 13762.791708..13763.024898 window and Trace causal projection are present. On-chain running supply deficit 65.912ms, D-state 36.757ms/11 intervals, runnable 1.536ms, deterministic JIT guidance, business clues, and background-only rows remain distinct. `io_wait=0.000ms` is correctly bounded to the accounting bucket; the answer separately states `dma_fence_default_w` is only a call-site and does not prove the waited object, holder, or subsystem. No final reject, malformed JSON, missing answer, Mermaid issue, or active-stream age degradation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- Human correctness: 2/2 pass; runner: 1/2 pass.
- Trace closes the production replay for the zero-`io_wait` inference boundary without weakening explicit-window causal diagnosis, system supplementation, or on-chain-only root-cause ownership.
- Cangjie exposes a generalized compiler contradiction: a closed typed per-member-table declaration is authoritative in QFEnumeration but silently weakened after the same request routes to QFComparison because it has multiple buckets. The fix belongs in the comparison family compiler and applies to every source language and conceptual member set; no request/final prose scan and no Cangjie-specific rule is warranted.
