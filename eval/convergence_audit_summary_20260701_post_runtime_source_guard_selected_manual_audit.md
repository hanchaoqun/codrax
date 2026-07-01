# Selected Eval Manual Audit

- date: 2026-07-01T04:33:08Z
- sweep_start_ts: 20260701-123308
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

The runner records typed metrics and declared oracle surfaces only; this file records the manual correctness/noise audit after reading final outputs and selected logs.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260701-123308 | typed_inventory_rowset,answer_contains | none | 97s | 18 | read=5,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer lists the 4 `@Entry` ArkTS page structs and 2 `@Builder` fragments with correct corpus paths and citations. First source_inventory pass still hit Go substring noise before grep/read correction, but the typed auxiliary/corpus inclusion issue is fixed for this case. |
| 3 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260701-123445 | answer_regex,answer_contains | none | 116s | 28 | read=4,repo_map=3,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Correctly answers count=1 and member `explorer`, with registration and `Name()` evidence. Some extra implementation proof was gathered, but no completion loop or answer pollution. |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260701-123308 | typed_inventory_rowset,dimension_substring,answer_contains | none | 213s | 27 | read=6,repo_map=4,list=0,trace=0,source_lens=4 | midloop=8,inv=4/0,fin_reject=0,unavail=0,prune=0 | partial | Required Cangjie rows are present, including extend blocks, foreign funcs, and Cangjie public classes with packages. However the visible answer also appends a system-generated Java public-class supplement from unrelated fixtures, so the commercial answer surface is polluted by out-of-universe rows. Recorded as D1-G210. |
| 5 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260701-123641 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 107s | 28 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Trace-only path stayed on trace_query, preserved wakeup chain, D/IO blocker, priority inversion, state drilldown, and final trace causal projection. No repo/source fallback or completion retry observed. |
| 4 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260701-123641 | trace_attachment,answer_regex | perf_triage+trace_query | 128s | 27 | read=3,repo_map=1,list=0,trace=2,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Correctly combines runtime trace fact with current-source parsing/jank threshold evidence. Analyzer prose briefly claimed external-only/no source, then the structured route still gathered source proof; note as prompt clarity debt but not answer-breaking. |
| 6 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260701-123828 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 300s | 39 | read=14,repo_map=1,list=0,trace=6,source_lens=0 | midloop=3,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass_with_fixed_noise | Final answer identifies the on-chain priority-inversion wakeup chain, ThreadPoolForeg D-state/IO root, CPU/IO background, representative windows, and trace-only boundary. Compared with the prior 1200s timeout this converged, but logs showed residual attached-trace blob read/emit_evidence noise; fixed in the follow-up code slice by keeping trace artifact hints on trace_query and telling emit_evidence users not to retry external rows. |

## Architecture Findings

- The D1-G208 runtime/source slices are materially effective: the prior Donghu timeout shape now finishes in 300s with 6 trace_query calls and 2 completion calls instead of 101 trace_query calls and 17 completion calls.
- Residual runtime artifact noise was still visible before the follow-up code change: a broad attached-trace `read_file` window triggered `grep/read_file` blob guidance and `emit_evidence` rejected runtime rows. This is now guarded by focused tests in the current patch.
- D1-G209 is not fully commercial-complete: source-inventory row correctness is improved, but answer projection still needs a typed requested-universe boundary so non-target language/source-family rows cannot become principal supplements.
- Mixed trace+source prompt wording still has a clarity smell: free-form analyzer reasoning may say "external only" while typed analysis/source lanes correctly proceed. No hard gate consumed that prose, but future prompt simplification should reduce that contradiction.
