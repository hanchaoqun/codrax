# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T01:24:08Z
- sweep_start_ts: 20260801-182407
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_system_inventory | PASS | eval/results/operation_system_inventory-20260801-182408 | log_regex,answer_regex | none | 27s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Typed route is operation/computer_operation, low-risk read-only plan auto-executed exactly four successful commands. Final values match raw outputs: macOS 26.5.2 build 25F84, 18 CPU cores, 137438953472 bytes = 128 GiB, Apple M5 Max GPU with 40 cores and Metal 4. No repository or write lane was entered. |
| 1 | log_path_question_multi_runtime_files | FAIL | eval/results/log_path_question_multi_runtime_files-20260801-182408 | answer_regex,answer_contains | none | 83s | 17 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Both named logs were admitted and each read exactly once; all six requested facts are correct. The two principal ordered lists have empty titles, however, so the visible answer does not bind either list to its source filename even though the user asked for separate per-file reports. The filenames appear only together in the final caveat. Runner failure is therefore a real presentation/identity weakness, not missing artifact parsing. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings and disposition

### Multi-log artifact grouping

The deterministic runtime-artifact selection view was complete before
finalization and listed both exact artifact IDs, kinds, and sources. Analyzer
sub-topics also named one log per topic, and the finalizer prompt explicitly
required a clearly labeled section for each topic. The model's own thinking
planned per-file grouping, then emitted two `ordered_list` blocks with empty
`title` fields. Evidence content and order were correct; only the visible
artifact-to-block identity was lost.

This is recorded as `EVAL-B31-ARTGROUP1/P2-watch`, not immediately hardened:
one replay does not justify rejecting an otherwise factually complete answer
solely because an optional title is empty. A generalized future solution, if
the failure repeats across multi-log/trace/data cases, is a typed per-block
artifact-subject binding carried by `claim_uses` or a dedicated structured
field. It must be validated from the runtime-artifact selection IDs, not by
searching filenames in rendered prose, and it must not rewrite the model's
answer. A weaker non-overfitted prompt improvement may request exact artifact
source titles from the already typed selection view.

### Eval observability

The runner reports `runtime_artifact_attached=none` and
`runtime_authority_path=none` even though the CLI admitted two request-path logs
and the explorer read both. Those metrics currently recognize pre-stage
log/perf authority but not direct named-artifact reads. Record
`EVAL-B31-RUNMETRIC1/P3`: audit telemetry should distinguish
`direct_named_artifact_read` from no runtime evidence. This did not affect the
answer and is below production correctness work.

### Operation lane

No production gap was found. All steps were read-only, separately logged with
exit code 0, and the final report was a faithful unit conversion/summary. The
operation artifact file is an expected command-output carrier under `.codrax`,
not a repository source mutation.
