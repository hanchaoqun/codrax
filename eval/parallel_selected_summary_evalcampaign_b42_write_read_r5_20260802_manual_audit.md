# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T15:41:22Z
- sweep_start_ts: 20260802-084121
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_log_current_source_explanation | PASS | eval/results/read_combo_log_current_source_explanation-20260802-084122 | log_attachment,answer_regex | log_triage | 215s | 34 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 系统不再重写 intent：前两次矛盾 payload 被 fail-loud，第三次由 analyzer 自己选择 root_cause。但拒绝提示只给“改 intent=root_cause”方向，未提示机制/日志边界问题可清除 diagnostic/current-risk，仍形成 root_cause_trace 与 68k 上下文。最终把 stream classifier complement 当 validation、把 canUseFinalizerOutputAfterTransientProgress 的 degraded fallback flags 当校验结果、把 render 分类当重试控制，三层均错误；真实校验路径是成功 finalize 后的 runContractCheck/Violation/requeue。已有软指导与 8 次 read 足够，答案错误记 model variance；只修对称分析重试上下文，不做答案 gate/替换。 |
| 1 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260802-084123 | write_apply,answer_regex | none | 317s | 21 | read=6,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass (proof boundary unverified) | PROOFIDENT 已覆盖：两个 probe 均为 path:cli/src/api/templates/js-binding.ts，impact ledger 无 changed_symbol，文件义务 4/4 covered。JS probe/Node runner 缺失，两个 behavior contracts 真正 unverified，final fail 正确。新 typed 矛盾：localization review 对同一路径携带两个 owner anchors，却仍报 plan_source_paths_missing_owner_context；根因是 cumulative-review 新 batch id 隔离了前批 durable owner anchor。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Context precision audit

- `CONTEXTROUTE1` 的系统语义改写已消失；模型原始 typed 字段不会被替换并持久化。
- 现有自洽 reject 文本仍不对称：只教模型把 `intent` 改成 root cause。对于
  `explain/architecture/mechanism + attached-log boundary`，另一个同样合法的修复是清除
  diagnostic/current-risk flags。该选择必须交还 analyzer。
- 8 次源码读取已包含 timeout classifier、finalizer transient-output helper 和 answer evaluator，
  但模型没有定位 `runContractCheck` 后的 violation/requeue 控制流。继续增加同类 prompt 或正文
  rewrite 会过拟合，暂记模型波动。
- write context 的 file identity 已准确；跨批 owner proof 的 scope 过滤却让同一份 typed 上下文
  自相矛盾，应按 source path identity 持久消费 owner anchor，同时继续隔离旧 verify-failure prose。
