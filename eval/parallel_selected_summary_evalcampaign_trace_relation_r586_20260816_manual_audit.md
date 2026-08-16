# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T21:27:23Z
- sweep_start_ts: 20260816-142722
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-142723 | answer_regex | none | 131s | 26 | read=3,repo_map=4,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass-with-citation-caveat | One justified first-draft reject: directed claim forms had no anchors. One patch retained both facets and emitted five typed relations; no identity-qualifier reject, no missing-principal-path supplement, no diagram/answer degradation. One item citation was detached and disclosed; the visible chain itself remains correct. |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-142723 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 191s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | partial | Zero finalizer rejects. Explicit-window Trace causal projection, on-chain ranking, actual-vs-eliminable account, all 11 target waits, call-site/object boundary, and incomplete enumeration limits survived. B933 stopped the previous GPU-fence/object invention. New B938: the model over-inferred `io_wait=0` as proving no storage-I/O mechanism; zero only proves this scheduler accounting bucket is zero. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

1. B933 is materially improved but not fully closed. The final typed boundary reached production and the model kept
   `dma_fence_default_w` as a kernel call site, explicitly leaving the waited object/holder unknown. It did not repeat the
   r585 GPU-fence mechanism claim. However, it wrote that exact `io_wait=0.000ms` “proves” no storage-I/O component.
   This is B938: a zero scheduler accounting bucket does not exclude an underlying storage/dependency mechanism.
2. B934 remains production-positive: the final answer discloses root-rank and critical-blocking compaction instead of
   claiming an exhaustive visible roster. The separately complete 11-row target-wait roster remains exact.
3. B936/B937 code paths were not needed by this stochastic replay because the model's single patch explicitly retained
   `facet_ids/surface_role` and copied exact recipe identities. Their unit and production-wiring pins remain the proof for
   the omitted-carrier and display-qualifier arms; r586 provides a clean non-regression witness.
4. The Poly final answer expresses relations through a structured ordered list rather than Mermaid. That is valid for this
   optional-diagram case: five same-direction typed anchors survive and no system-authored edge or diagram was created.
5. No malformed-JSON answer recovery, empty answer, stale-draft fallback, active-stream age threshold, fixed 4ms
   degradation, or system replacement of model conclusions was observed in either case.
