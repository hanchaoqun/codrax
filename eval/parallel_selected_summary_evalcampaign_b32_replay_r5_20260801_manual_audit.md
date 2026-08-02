# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T02:32:58Z
- sweep_start_ts: 20260801-193257
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260801-193258 | log_regex,answer_regex | none | 192s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Correct strict answer `17`; rules/material coverage, 4 decisions, 2 contributions and reconcile pass are present. DATAREL1 is covered: no normalize/mapping/apply detour, 11→7 data rounds. One rejected over-broad qualification candidate remains a P2 watch because an older field-contract failure briefly competes with the complete live state. |
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260801-193258 | log_regex,answer_regex | none | 234s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Correct strict answer `17,0,5`; inactive rows excluded, mapping/entity evidence present, 10 decisions, 4 contributions, 5 resolutions and reconcile pass. 15→8 rounds and 657→234s. First answer `17,4,5` had correct cardinality but usurped GroupX's zero slot; terminal grounding caught it, while the live graph still said satisfied/complete. This is DATASTATE5, fixed after replay by `5bdfe9ac9`. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-run judgment

- Runner correctness is genuinely 2/2: both final outputs and typed ledgers agree with the fixture bytes.
- `EVAL-B32-DATAREL1` is covered by a different execution: the simple case no longer invents a relation between one source table and its aggregate/child aliases.
- `EVAL-B32-DATASTATE4` wiring is intact, but its input authority was incomplete. A same-count per-slot mismatch was absent from the live `OutputProjectionGraph`, so the evaluator saw `next_stage=complete` before the terminal gate rejected the answer.
- The generalized fix does not parse the user request or model prose and does not manufacture an answer. Typed complete-reference intent plus source-material lineage feeds a `reference_grounding_mismatch` state, reopens `emit_output_contract_answer`, and leaves value/slot correction to the model. A deterministic same-count rewrite is explicitly forbidden.
- Watchdog audit found a pre-existing over-hard negative: structural key-table census alone could hard-block a valid subset answer. The new pin requires an entirely empty guard without typed complete-reference intent; census remains discovery-only.
- P2 watch: `data_basic_sum_with_rules` retained a rejected `qualify_eligible_orders` candidate using nonexistent status fields. One specimen is insufficient to change deferred-evaluation authority; seek a second topology before implementation.
