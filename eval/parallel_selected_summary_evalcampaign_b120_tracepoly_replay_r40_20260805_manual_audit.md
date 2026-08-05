# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T15:50:44Z
- sweep_start_ts: 20260805-085043
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260805-085044 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 246s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B120 production positive: projection, target state, wakeup path and ranked seats consistently use the requested 34579.472865..34579.587805 / 114.940ms window; the earlier 50ms probe no longer displaces them. Human fail remains because the model's malformed blocks payload was quarantined to one caveat-only block, then deterministic trace materializers supplied the entire visible report without a model-owned summary/diagnosis. The compact final ledger also chose the adjacent 0.171ms IO row instead of the published on-chain direction maximum 10.433ms. |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260805-085044 | answer_regex | none | 296s | 22 | read=2,repo_map=1,list=1,trace=0,source_lens=1 | midloop=8,inv=7/0,fin_reject=3,unavail=0,prune=0 | fail | B117/B118 remain covered: no duplicate member roster and the explicitly removed optional diagram is not resurrected. The final text distinguishes a PyO3 wrapper and Rust core better than r39, but still invents self._tokenize_fast, leaves the wrapper row uncited, and describes lib.rs:10/:40 as two cooperating definitions. Analyzer spent five rounds establishing endpoints and explorer spent nineteen rounds closing an identity that the source already proves at lib.rs:42; EVAL-B107-ENDPOINTAMBIG1 remains a generic cross-language binding/call identity gap. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
