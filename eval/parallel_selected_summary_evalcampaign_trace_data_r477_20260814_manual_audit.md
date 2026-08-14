# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T09:57:07Z
- sweep_start_ts: 20260814-025705
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260814-025707 | log_regex,answer_regex | none | 56s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Initial plan had a valid complete-reference output tuple but an invalid non-object `assemble_answer.output_field`. Compact repair received the error and schema but not the prior params; the model explicitly said the original plan was unavailable, fixed the action, and recreated `complete_reference=true` without `reference_path`. The second structural error then exhausted the one repair. B782 carries the bounded previous tool params into the same-tool repair so unrelated valid fields remain visible; no business value is synthesized. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260814-025707 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 215s | 46 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=1 | pass | The model-owned answer preserved complete target states, on-chain #1/#2 priority/supply candidates, D/IO, target compute-supply deficit, business thread clues, and representative windows. It explicitly disclosed absent frame/deadline authority and did not claim a proven dropped-frame cause. Adjacent/background rows stayed support-only; actual occupancy and rule-eliminable quantities remained separate. Analyzer needed four attempts to align source-exclusion and runtime-breadth JSON despite precise teaching/errors; recorded as adherence churn, not a new prose hard gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusion

- Runner: 1/2 PASS. Human: one pass, one fail.
- Trace causal projection remains present and authority-calibrated. No current-source reads occurred, and active streaming was never degraded by fixed request age.
- Data failure is a generalized structural-repair context-loss defect rather than a calculation defect. The model had already computed `17,0,0`; execution never began because the compact repair had to reconstruct an unseen JSON object.
- B782 changes only repair context. It does not merge plans, choose complete-reference semantics, calculate values, alter the final answer, or add request/model/final-prose gates.
