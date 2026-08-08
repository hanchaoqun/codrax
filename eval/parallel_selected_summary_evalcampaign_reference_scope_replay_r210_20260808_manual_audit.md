# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T12:12:17Z
- sweep_start_ts: 20260808-051216
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260808-051217 | log_regex,answer_regex | none | 132s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | S37by production-positive: planner emitted and carried complete_reference=true with targets.csv / target_id; output graph reports reference_complete_required=true, reference_complete=true, reference_projected=true and zero_filled_count=1; published answer is 17,0,5. The system projected the model-owned typed scope and did not infer scope from prose or overwrite contributions. Process remains inefficient: 11 data rounds and 2 repair rounds; action params are still an open object, so the model mixed source_filter_field into filter_records and lookup_specs into join_records before runtime reported the per-kind allowed set. File generalized JSON-contract gap EVAL-B349-ACTIONPARAMSCHEMA1; do not fit this fixture or relax deterministic action validation. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-051217 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 184s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | S37bw remains production-closed: the exact VerifyClass occurrence occupies one semantic/on-chain seat E25(+3), with absorbed evidence and locators retained. Explicit 114.940ms window, four trace_query calls, deterministic supplement, wakeup path, ranked on-chain causes and actual-occupancy versus rules-based-eliminable dual axes remain present. Main root #1 CookieMonsterCl-59843, #2 NetworkService and #3 ThreadPoolForeg are typed on-chain; larger adjacent/background CPU/IO rows remain reference-only and never enter root ranking. frame_evidence_status=absent limits concrete frame-drop causality only. The model still conditionally expands priority_inversion_candidate toward lock/PI remediation without holder/waiter proof; typed caveat is correct, so retain as soft model-adherence observation rather than scanning/replacing/rejecting prose. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
