# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T22:16:10Z
- sweep_start_ts: 20260816-151608
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260816-151610 | typed_inventory_rowset,dimension_substring,answer_contains | none | 149s | 26 | read=8,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | All 12 declarations, exact file/line, symbol, package and citations remain correct, and B941 reduces deterministic finalization churn from four rejects/degradation to one reject followed by a valid structured document. The visible answer nevertheless loses row-to-bucket membership: the first draft used item.label for `extend block` / `foreign func` / `public class` and cells for file/member/package; the exact row-id gate called label the “first visible value” and told the model to replace it with the member. The patch did exactly that, yielding a table whose first and third visible columns both contain the member and no row contains its category. Runner therefore counts all 12 rows in every bucket. This is B942: member identity and visible bucket are independent typed axes but the contract gave them one label slot and no per-row bucket preservation gate. |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-151610 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 226s | 44 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit window, model-authored diagnosis and Trace causal projection remain present with zero finalizer rejects. The answer separates running 74.915ms / supply-fold deficit 65.912ms, D-state 36.757ms across 11 complete state segments, runnable 1.536ms, sleep 118.586ms and io_wait=0.000ms. On-chain rows alone receive root rank; adjacent/background pressure, business spans and deterministic JIT remain support lanes. It distinguishes 12 blocked_reason records / Σ39.157ms from the 11-segment D-state wall-clock account and repeatedly states that `dma_fence_default_w` does not identify the holder/resource/subsystem. One model phrase calls it a “device-side fence driver”, which is stronger than the typed call-site evidence, but the same answer and deterministic boundary explicitly retract subsystem authority; record as minor model wording variance, not a new hard-gate rule. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 1/2 PASS. Manual correctness: 1/2 pass.
- `B941-MARKDOWNSIDECARPRIMARYAXIS1` is production-positive for convergence: Cangjie now emits a valid structured document after one repair and no degraded fallback.
- `B942-PERMEMBERBUCKETAXIS1` is confirmed: the source-inventory row identity repair deleted a separate required comparison axis. This is generic to every language and every per-member comparison table, not a Cangjie label special case.
- No malformed JSON, Mermaid error, answer disappearance, or active-stream fixed-age degradation occurred.
