# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T11:15:40Z
- sweep_start_ts: 20260808-041539
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-041540 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 150s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Exact user window, wakeup chain, on-chain-only roots, actual occupancy and rule-eliminable axes are correct. B344 provenance alias closes as E24(+1), but the same exact VerifyClass occurrence is also E43 on the background lane and appears twice in the semantic table with contradictory authority. Filed B346. |
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260808-041540 | log_regex,answer_regex | none | 151s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | Final `17,0,5` and reconciliation are correct. The model consumed `executable_next_rank`, but twice encoded `actions` as a JSON string; compatibility recovery restored future-rank actions after the provider schema boundary, and the deterministic controller split them later. Filed B347 for post-normalization schema authority; no unsafe action executed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- Trace: `frame_evidence_status=absent` is correctly limited to missing frame/deadline proof; it does not erase the selected-window typed chain. Main/root candidates are exclusively on the proven chain. Adjacent CPU/IO pressure remains background and is not promoted by magnitude or temporal proximity.
- Trace: `E24(+1)` proves B344/S37bu closed the model-query/system-supplement provenance split. `E43` is a separate query-view publication of the exact same subject/name/start/end occurrence, but carries background authority. Its duplicate appearance is a system projection gap, not model prose fluctuation.
- Data: the dynamic rank schema and final prompt capsule are present and were read by the model. The remaining churn is structural compatibility authority: a stringified nested array is normalized after function-call generation, but the restored nested enum is not shown to have passed the original schema again.
- No raw user/model/final-answer prose drove a hard gate, and no system normalizer replaced the model's conclusion in either case.
