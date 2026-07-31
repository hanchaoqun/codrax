# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T09:50:51Z
- sweep_start_ts: 20260731-025050
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_code_boundary | PASS | eval/results/read_combo_log_current_code_boundary-20260731-025051 | log_attachment,answer_regex | log_triage | 146s | 28 | read=5,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | S1 真实覆盖：当前源码 lane 恢复，最终引用 status_messages.go、write_retry_helpers.go、finalizer.go，并区分 first-byte/stream transport timeout 与 contract/semantic validation。 |
| 2 | logtri_oversized | PASS | eval/results/logtri_oversized-20260731-025051 | log_attachment | log_triage | 147s | 23 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 自动 oracle 仅检查日志附件被处理，漏掉主结论错误。log_triage 已给出 `main.crashy()` at external source line 100、caller `main.main()` line 200；最终答案却让当前 checkout mismatch 覆盖运行时栈顶，声称“无法定位具体发出位置”。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- B3 第二轮自动 2/2 PASS，人工只有 1/2 PASS；`logtri_oversized` 是 false PASS。
- 第一例证明 S1 已修复原始权限串线：artifact citation 边界不再关闭 current-source lane；mixed 请求保留了源码读取和引用。
- 第二例暴露更上游的通用路由语义混淆：`needs_repo_access=true` 只说明要进入 repo/read pipeline，并不等于答案必须以当前 checkout 为证据。附件独立诊断被错误制造为 current-source obligation 后，运行时直接栈顶被源码 mismatch 否定。
- 最优修向是新增闭集 typed `current_source_evidence_mode=required|optional`，与 `needs_repo_access` 正交；artifact-only 为 optional，显式“结合当前源码”及普通源码请求为 required。生产消费只读该 enum，不扫描用户原文或答案正文。旧缺字段 hint 保留 `needs_repo_access` fallback，避免持久化/测试适配器兼容回退。
- runner 后续应为该 case 增加 principal runtime-origin oracle，至少固定栈顶 `main.crashy()` 不得被 checkout mismatch 擦除；这是 eval 观测面，不进入产品 hard gate。
