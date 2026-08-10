# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T16:47:35Z
- sweep_start_ts: 20260810-094733
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260810-094735 | log_regex,answer_regex | none | 229s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B465 阻止矛盾 scope 静默签绿，但 terminal 因 instructions.md 保持 script_consumed 且从未被动作消费而 fail-loud。终态只允许 reconcile/assemble，不能回到 material/rule coverage；6 次 repair 重复同错，零答案。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-094735 | answer_regex,answer_contains,mermaid_edge_count | none | 470s | 40 | read=16,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=2/0,fin_reject=14,unavail=0,prune=2 | fail | Runner 只看到 Mermaid/关键词即 PASS；人工判 FAIL。最终图仅保留 3 条 dispatchStage 内部调用，7 个请求 participant 断开，系统 caveat 明示图不完整。最后一稿 pre-emit 已接受 7 条 unproven boundary，但 AnswerDocumentV2 defensive clone 漏拷 ParticipantBoundaries，post-emit 反判 missing_unproven_boundary 并耗尽重试。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
