# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T13:06:20Z
- sweep_start_ts: 20260804-060619
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260804-060620 | log_regex,typed_operation_terminal,answer_regex | none | 132s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 complete receipt 精确回溯到 `curl .../user_guide.html`，内容 identity 为 `sha256:c94c…:bytes:248161`；模型先识别首页不是目标材料，再取得完整手册并给出实质摘要。B75 source-provenance 修复生效。 |
| 2 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260804-060620 | log_regex,answer_regex | none | 286s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 输出 `20,0,5`，不是 `17,0,5`。完整 reference projection 已生效：fallback 按 targets 顺序补 GroupX=0、丢弃 GroupB；但 `qualify_records` 接受 `source_filters(active=true/canonical_id not_empty)` 后静默忽略，inactive r3 的 3 被算入 GroupA。另有两次把 workflow ledger 误选为 record input 的可恢复规划噪声。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
