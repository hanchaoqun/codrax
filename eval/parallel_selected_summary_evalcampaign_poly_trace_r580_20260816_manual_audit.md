# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T19:07:09Z
- sweep_start_ts: 20260816-120707
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-120709 | answer_regex | none | 143s | 27 | read=0,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | B923/B924 production-positive: coarse PyO3 declaration no longer hides the unique line-47 binding; the model re-emitted `m -> wrap_pyfunction!(tokenize_bytes,m)`, and selected definition bodies supplied exact wrapper/core/helper calls without system-authored path or answer. B921 also joined `_fastlex.tokenize_bytes` to `py.tokenize_bytes` as a non-call registered-export handoff. The first final draft correctly failed only because it omitted the requested `FastTokenizer.tokenize` entry, then patched cleanly. B925 is now confirmed rather than a one-run observation: the visible summary and ordered list already named `_fastlex`, but the structured requested-dimension receipt still declared the native-module member-set absent and forced a redundant third round/extra heading. The caveat also imprecisely calls import-success state a build-time condition; retain as model-level partial, not a prose hard gate. |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-120709 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 241s | 45 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Explicit 233.190ms window stayed authoritative across 6 trace queries. The model answer and deterministic materializer preserved Trace causal projection, on-chain-only ranking, actual-occupancy versus existing-rule-eliminable double entry, D-state/caller, scheduler/compute/priority/IO lanes, deterministic JIT optimization, and chain-member business spans. Off-chain logd pressure stayed adjacent/background and was not crowned. No finalizer rejection, JSON recovery, empty answer, or active-stream age degradation occurred. Partial because the model recommends “改善散热” although typed evidence expressly says the frequency/policy witness does not independently prove a thermal mechanism; this is a context/soft-guidance calibration signal, not authority for the system to rewrite the conclusion or scan prose as a hard gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
