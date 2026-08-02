# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T18:29:52Z
- sweep_start_ts: 20260802-112952
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260802-112952 | answer_regex,answer_contains | none | 111s | 23 | read=4,repo_map=1,list=1,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Process convergence recovered: 111s, one repo map, four reads, five explorer iterations, one completion. This run did not reproduce the bad source-inventory profile, so it is not a live-path proof of the new support-only branch; production tests remain that branch's proof. The final gets both Name literals and retry/no-prev guard right, but incorrectly turns a preference into an exhaustive binary: it says any existing previous emit means patch. Source line 49 was read and explicitly says full emit is also valid whenever a complete rewrite is needed. Context was accurate and sufficient; this is one-run model/evidence-summary narrowing, not a reason for a prose hard gate. |
| 1 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260802-112952 | log_regex,answer_regex | none | 206s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B45 replan bypass is closed: two invalid complete evaluations were repaired, the loop continued, and the terminal typed status was partial_answer_possible. The model still printed a trailing “任务已完成/完整内容” claim and an invalid /focus REPL command, contradicting the exact partial constraint and the manual's /repos focus spelling. Four rounds also expose a generalized large-material gap: arbitrary shell extraction creates payload refs without typed source/range lineage; even a full 177KB text extraction is reduced to a 4K prompt prefix, so the system cannot prove paged coverage or guide non-overlapping continuation. Runner PASS is a false positive. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
