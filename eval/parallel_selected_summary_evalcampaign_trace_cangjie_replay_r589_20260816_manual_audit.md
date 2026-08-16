# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T21:59:28Z
- sweep_start_ts: 20260816-145926
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-145928 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 226s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass | Explicit window and Trace causal projection are both present. The model separates target running 74.915ms / supply-fold deficit 65.912ms, D-state 36.757ms across 11 segments, runnable 1.536ms, sleep 118.586ms and io_wait=0 without using the zero bucket to exclude a device/dependency mechanism. On-chain candidates alone occupy the root ranking; adjacent runnable pressure, background CPU, business spans and deterministic JIT clues remain supporting lanes. `dma_fence_default_w` is correctly bounded to a recorded kernel call site rather than an independently proven fence object/holder/subsystem. The visible answer preserves actual wall-clock occupancy versus rule-priced eliminable impact. One analyze-stage grep attempt was unavailable but did not remove trace authority or trigger answer degradation. |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260816-145928 | typed_inventory_rowset,dimension_substring,answer_contains | none | 238s | 27 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=4,unavail=0,prune=0 | pass | Manual row audit finds all 12 requested declarations correct and complete in one category-first table: 2 extend, 2 foreign func and 8 public class rows, with symbol, file and package. B940 is production-positive because the compiler requested exactly one principal table and the model emitted it. The runner failure is a system contract conflict, not extractor loss or model fluctuation: Markdown renders `category | member | ...`, while aggregate coverage only accepted the first visible column and ignored exact row-id sidecars even after the model removed the invalid mixed-family block partition. Four deterministic rejects exhausted retries and forced the otherwise useful prior draft into degraded output. B941 fixes this generically via exact member identity + index-aligned source location + complete sidecar/visible-row parity; hidden sidecars without exact visible parity still cannot invent rows. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 1/2 PASS. Manual correctness: 2/2 pass.
- `B940-COMPARISONPERMEMBERTABLEAUTHORITY1` is production-positive: the required carrier is one principal table and no duplicate bucket sections are required.
- `B941-MARKDOWNSIDECARPRIMARYAXIS1` is a confirmed deterministic contract conflict. It is language- and case-independent because the failure is between renderer-visible Markdown column order and typed source-inventory sidecar identity.
- No malformed JSON, Mermaid syntax failure, answer disappearance, or active-stream fixed-age degradation occurred. The Cangjie degradation followed four pre-emit contract rejects after a complete model answer already existed.
