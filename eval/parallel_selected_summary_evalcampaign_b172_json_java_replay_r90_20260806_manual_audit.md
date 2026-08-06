# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T10:26:17Z
- sweep_start_ts: 20260806-032615
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260806-032617 | log_regex,answer_regex | none | 127s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final JSON is exactly `{"ids":["u1","u3"]}` and preserves source order. Carrier decoding was healthy. A local required-material repair nevertheless introduced unrelated `decision_records_required=true`; the otherwise complete custom transform then failed because `result.rows` was empty and cascaded into an eight-round ledger workflow. Typed admission already carried `required_material_scheduling`, but the repair locus fell back to a coarser error-text class. Product result is correct; repair locality/model burden is a confirmed P1 system gap. |
| 2 | github_issue_gson_lazy_number | FAIL | eval/results/github_issue_gson_lazy_number-20260806-032617 | write_apply,write_patch_oracle | none | 157s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | Patch is semantically appropriate and the new `java/direct_main@.` surface is discovered and attempted. This host has no usable JDK, so both the authored Java probe and direct-main runner report typed `runner_missing`; the controller correctly refuses a behavior-verification green. This is production-wiring evidence, not a completed Java behavior replay. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
