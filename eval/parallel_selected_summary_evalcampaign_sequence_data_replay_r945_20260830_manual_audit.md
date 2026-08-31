# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T02:08:09Z
- sweep_start_ts: 20260830-190808
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260830-190809 | log_regex,answer_regex | none | 41s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Strict JSON was exactly `{"ids":["u1","u3"]}`; the planner selected one allowed `custom_transform`, consumed `users.json`, reached typed terminal status `complete`, and needed no repair. Rank-local schema no longer advertised an excluded script field. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260830-190809 | answer_regex,answer_contains | none | 313s | 31 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=1,unavail=0,prune=0 | pass-with-residual | Final answer correctly reports the two converging edges `buildAnalysisIR -> gate.RunWith` and `gate.Run -> gate.RunWith`, explicitly denies a directed path between the requested endpoints, preserves the Mermaid view, and lists supporting intermediate functions separately. The sole finalizer reject was repaired in one patch without stale-anchor looping or degraded recovery. Residual: analyzer first hard-rejected the legitimate sibling `member_set` dimension and forced it to `relation_path`, so the first draft mixed the support roster into `principal_path_edge`; this is a typed multi-surface contract gap, not model variance. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
