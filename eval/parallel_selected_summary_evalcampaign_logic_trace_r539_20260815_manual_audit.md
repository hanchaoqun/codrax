# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T00:12:52Z
- sweep_start_ts: 20260815-171251
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260815-171252 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 222s | 48 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit window, causal projection, deterministic supplement, typed on-chain ranking, occupancy/business clues and rule-priced eliminable directions are present. The model nevertheless calls the four directions physically independent, later says their wall-clock gains may overlap and cannot be added, then advertises their 78.061ms arithmetic sum as an ideal gain. Typed B646 context already says physical relation unresolved and joint-total authority absent, so this is model noncompliance/internal contradiction over sufficient precise context; do not scan/rewrite the prose or mint a hard conclusion gate. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260815-171252 | answer_regex,answer_contains,mermaid_edge_count | none | 359s | 39 | read=22,repo_map=5,list=0,trace=0,source_lens=1 | midloop=13,inv=5/0,fin_reject=1,unavail=1,prune=0 | fail | Runner false green: the accepted graph retains only Analyzer→Explorer→Extractor→Finalizer precedence and leaves BusContext/Mutable as isolated nodes while prose claims they carry cross-stage data. The first model draft had the correct typed no-arrow `BusContext` subgraph containing `Mutable`; validation correctly rejected an extra contain arrow, but participant repair simultaneously demanded a “disconnected participant”, causing the valid grouping to be flattened. B865 fixes this generic directed-boundary-vs-structural-ownership contract conflict without creating an edge or diagram. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
