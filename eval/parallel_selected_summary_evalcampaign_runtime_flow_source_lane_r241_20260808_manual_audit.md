# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T04:51:36Z
- sweep_start_ts: 20260808-215133
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-215136 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 114s | 29 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | B411 production closed: the first completion emit was accepted, `pre_complete_downgrades=0`, and only two typed trace queries ran. The one finalizer reject is a distinct display-identity gap: participant labels decorated with `[CPU N]` no longer equal the report-local typed endpoints, so a faithful temporal subset was rejected until the model copied the exact capsule. The final prose still overclaims UI/RenderService/GPU thread roles although typed role/internal-work authority is absent; causality itself is disclosed as unproven. This remains B403 model semantic adherence, not evidence for a prose hard gate or system answer rewrite. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260808-215136 | answer_regex,answer_contains | none | 516s | 35 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=9,inv=2/0,fin_reject=10,unavail=0,prune=0 | fail | Ten finalizer rejects / eight patch attempts form a P0 retry storm. The final answer says `Analyze -> Explore -> Plan -> Finalize`, omitting the canonical read `Extract` stage and inventing `Plan`; the deterministic supplement later publishes the correct `analyze -> explore -> extract -> finalize`, so the page contradicts itself. The initial finalizer prompt already contained the exact canonical sequence and a copy-ready one-edge source capsule. Production never selected the exact required-diagram repair lane after relation-only patch rejects. Separately, one citable call row (`runAnalyzePhase -> dispatchStage` at line 2485) authorized four repeated visible call occurrences, so evidence occurrence cardinality is not enforced. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
