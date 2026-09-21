# Fixed db0596794c36 pair — manual audit

- date: 2026-09-21T03:06:27Z
- sweep_start_ts: 20260920-200627
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_marker_local_surfaces_20260920

Fixed binary `db0596794c36`, two concurrent cases, one run each. Machine 1/2; human 0/2. No same-version third run, no retrospective PASS after code fixes. Final Markdown, fixture, tool results, actual finalizer context and emitted document/patch were read.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_marker_local_surfaces_20260920/trace_query_wakeup_causal_io_chain-20260920-200628 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 158s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 20ms/11ms and wakeup times 2.016/2.018/2.020 preserved; network sleep14ms wrongly assigned recursive window2.001..2.018 rather than physical2.002..2.016, also in system table; incorrectly calls network higher-priority than equal-priority cookie. |
| 1 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_marker_local_surfaces_20260920/trace_query_business_marker_io_chain-20260920-200628 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 279s | 39 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | Ownership receipt unavailable; body also mixes exact5ms marker runtime with7ms wider account as5–7ms, calls IO request window the LoadDocumentIndex window, and excludes actual IRQ waker from chain. New exact marker account reached finalizer, but did not guarantee correct use. |

## Evidence and attribution

- Explicit report: `.codrax/output/20260920-200903.460-58465.md`. Corrects the previous wrong400→300 wakeup timestamp and now discloses missing concrete wait-object/backend evidence. It retains one Trace causal projection and an available schema2 root-cause sidecar; these do not close the wrong sleep window or equal-priority claim.
- Business report: `.codrax/output/20260920-201104.218-58476.md`. Finalizer log2748 explicitly supplies OpenDocument50ms=running5+runnable1+sleep44, with source/owner/local window. Model nevertheless emits5–7ms and later exact5ms. LoadDocumentIndex is1.004500..1.044500 (40ms), not request1.005..1.040 (35ms);31ms is worker sleep, not request residence. The47ms unrelated backup is correctly kept outside the causal chain. One projection survives, but still uses model-selected0.999..1.052, not accepted business focus; no forced focus or silent window replacement was added. Root sidecar exists with `unavailable/no_selectable_typed_on_chain_candidates`, not missing-file success.
- Both `.answer-surfaces.json` files were written but `unavailable`: final scheduler trims outer whitespace after rendering; the new audit snapshot had not mirrored this exact existing transform. Explicit case uses whole-answer oracles so still machine PASS; business scoped oracles fail closed. Do not relax the reader or fall back to headings. Deterministic RED→GREEN fix is logged separately in unified audit §38.2.
- Analyzer business dispatch made5 attempts,4 rejected and1 accepted, yet old telemetry says5 writes and prompt still says exactly once/every field required. Precise profile rejects remain valid; there is insufficient evidence to attribute all retries to wording. The contradictory teaching and success-agnostic strict counter are independently confirmed system defects, addressed separately.
- Network state-duration/recursive-window pairing is a system handoff and display defect, not merely model variability. Preserve actual query, clocks, state totals and causal qualification; label aggregate measurement windows separately from physical state intervals. No new prose-scanning hard gate is authorized.

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
