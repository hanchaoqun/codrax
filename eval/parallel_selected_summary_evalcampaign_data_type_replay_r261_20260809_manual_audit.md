# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T03:53:45Z
- sweep_start_ts: 20260809-205344
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260809-205345 | log_regex,answer_regex | none | 51s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exact JSON is `{"ids":["u1","u3"]}`. Planner again distilled the complete instruction artifact and used one custom transform; neither the malformed repair-plan fallback (B448) nor late-rule generation (B445) executed, so their production witnesses remain pending. |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260809-205345 | answer_regex,answer_contains | none | 158s | 22 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | Table lists all 12 production implementations with correct files and separately discloses 3 test implementations. Diagram contains all 12 same-direction `implementer -> LoopController` edges. There is no read-but-unemitted pre-complete loop. The sole Finalizer reject is legitimate: the model's first draft reversed all edges; one patch flipped body and anchors and then passed typed validation. Runtime fell from r260 488s to 158s. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
