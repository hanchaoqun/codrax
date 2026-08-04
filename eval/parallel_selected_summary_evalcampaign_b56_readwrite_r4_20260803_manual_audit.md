# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T02:59:18Z
- sweep_start_ts: 20260803-195916
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260803-195918 | write_apply,write_patch_oracle | none | 232s | 19 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Patch is confined to `relativedelta.py`; whole-number floats normalize to `int`, fractional floats fail fast. Python verification probe and `python3 -m unittest discover -v` both executed; 5 tests passed and changed-path authority is `project_runner/target_behavior`. The probe omitted contract refs, but the real project runner closed every required outcome, so this remains an efficiency watch rather than a verifier failure. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260803-195918 | answer_regex,answer_contains | none | 285s | 28 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=4,unavail=0,prune=0 | fail | No degraded-finalize event (new false-green guard stayed at 0), but the accepted completion and final answer still rename the proven `buildAnalysisIR -> gate.RunWith` edge as reaching requested `gate.Run`. Analyzer emitted typed `call_chain + axis=call + relational=true` and exactly two non-path symbol entities, but omitted optional `ExactTargets`; the endpoint reachability gate was therefore skipped. Four finalizer rejects repaired diagram edge shape, not the false endpoint conclusion. Filed and implemented as `EVAL-B57-CALLTYPED1`. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
