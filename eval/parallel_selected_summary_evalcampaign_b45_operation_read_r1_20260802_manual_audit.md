# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T17:56:59Z
- sweep_start_ts: 20260802-105658
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260802-105659 | log_regex,answer_regex | none | 115s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | The link inventory again found the exact manual. The evaluator correctly continued twice while excerpts were truncated. The extraction command then failed, and the replan path emitted plan `status=complete` without calling the operation evaluator, bypassing the new material-coverage contract. The final is more honest than r4—it says only chapters 1–2 were summarized and points chapters 3–8 to the raw HTML—but still opens with “任务完成” and presents unsupported details from unseen chapters. This is partial material coverage, not completion. |
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260802-105659 | answer_regex,answer_contains | none | 419s | 36 | read=7,repo_map=20,list=0,trace=0,source_lens=20 | midloop=17,inv=9/1,fin_reject=0,unavail=0,prune=2 | fail | Final answer is substantively correct: both Name() literals, registry points, retry threshold, table and Mermaid are grounded. The process is systemically unacceptable: analyzer mislabeled the finite two-target comparison as repo-wide function source inventory. After exact source reads had answered the question, completion was downgraded repeatedly, forcing 20 source-inventory calls across unrelated fixtures/skill paths, 42 explorer iterations, 9 completion attempts, two prunes and 419s. The injected owner table is also redundant with the model answer. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
