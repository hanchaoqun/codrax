# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T00:24:27Z
- sweep_start_ts: 20260801-172426
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260801-172427 | answer_regex,answer_contains | none | 161s | 36 | read=10,repo_map=4,list=0,trace=0,source_lens=1 | midloop=7,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | The requested diagram/table are present, but both insert `StageWriteAnalyze` into the read-mode execution lane. Production `Run` dispatches `runWriteAnalyzePhase` only for plan/apply/verify; read goes directly to `runTaskPhase -> runTaskGraph -> runReadSchedulerLoop`. The analyzer promoted a real cross-lane symbol from broad repo-map navigation, and the finalizer had no pre-answer typed read-lane membership authority. Several list citations also point to unrelated definitions. The post-answer verified stage table is correct but does not cure the contradictory model answer. |
| 1 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260801-172427 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 29 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | Principal analysis is useful and grounded: app-20 runnable=5.000ms/62.5%, fragmented churn is absorbed, same-CPU rival context is separated, and observed occupancy vs rule-eliminable axes are explicit. Two system gaps make the shipped answer inconsistent: metric snapshot publishes the same app-20/window state account twice with 19/20 versus 20/21 switch/segment boundary counts; the prose-board appendix falsely says the model's first cause is rival-30 although the lead and rank #1 are app-20. No frame/deadline evidence exists, so the result is a selected-window cause candidate rather than proof of a concrete dropped frame. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
