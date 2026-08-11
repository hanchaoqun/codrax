# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T03:44:14Z
- sweep_start_ts: 20260810-204413
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260810-204415 | log_regex,answer_regex | none | 50s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Published payload is exactly `{"ids":["u1","u3"]}`. `instructions.md` was consumed through the typed `planner_distilled` material carrier because its complete rule text was already present in the planner sample; `users.json` was the executed data input. One judge self-questioned this distinction, then correctly resolved it from the workflow ledger. No JSON repair or retry occurred. |
| 1 | read_combo_pipeline_sequence_table | TIMEOUT | eval/results/read_combo_pipeline_sequence_table-20260810-204414 | answer_regex,answer_contains | none | 1201s | 41 | read=46,repo_map=3,list=0,trace=0,source_lens=0 | midloop=24,inv=7/0,fin_reject=21,unavail=0,prune=8 | fail | No final answer shipped. Analyzer roles were correct, and the first finalizer prompt carried two exact typed `data_flow` recipes. The model repeatedly retargeted those exact endpoints to conceptual `Orchestrator -> BusContext` edges, so relation rejection was correct; it eventually removed every edge. Post-finalize then classified `required_diagram_edge_absent` as Explore-owned despite the delivered recipes, cleared the useful draft, and repeated broad exploration. This is B504: precise recipe-present authoring debt must remain Finalizer-local; recipe-absent evidence debt still backtracks to Explore. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 1/2; human: 1/2.
- Data strict-output lane is healthy. The terminal payload contract applies to `published_answer`, not to interactive progress/thinking text in `run-1.out`.
- The read relation case is not a relation-validator contradiction: every rejected conceptual edge differed from the exact typed endpoint tuple. The production gap is repair ownership after the model reduced the diagram to zero edges. An exact prompt-delivery receipt existed, yet the generic violation forced `back_to_explore`, producing 46 reads, four explorer dispatches, two finalizer dispatches, 21 finalizer rejects and a 1200-second timeout.
- B504 changes only retry ownership from the same producer-owned typed receipt. It does not create a relation, weaken edge validation, inspect request/model/final prose, or rewrite the answer. B501's layered soft recipe remains a model-consumption watch because this run did not follow it.
