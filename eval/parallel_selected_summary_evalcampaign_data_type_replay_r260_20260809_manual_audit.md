# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T03:29:49Z
- sweep_start_ts: 20260809-202948
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260809-202949 | log_regex,answer_regex | none | 45s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exact JSON is `{"ids":["u1","u3"]}`. Planner distilled the complete instruction artifact and the single data round consumed `users.json`; B448 malformed-plan fallback and B445 late-rule regeneration were not exercised, so both retain production-replay-pending status. |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260809-202949 | answer_regex,answer_contains | none | 488s | 35 | read=15,repo_map=1,list=0,trace=0,source_lens=1 | midloop=11,inv=6/0,fin_reject=5,unavail=0,prune=0 | fail | Table correctly enumerates 12 production implementations and excludes test implementations, but the required diagram contains 12 nodes and zero edges. Pre-complete repeated the impossible read-but-unemitted demand four times; Finalizer then rejected five evidence-backed edge drafts because deterministic `repomap_implementer_relation` rows reached prompt notes but not the validator evidence index. The accepted zero-edge diagram is a runner false green. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
