# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T23:38:40Z
- sweep_start_ts: 20260817-163838
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-163840 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 251s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 模型正确识别 worker-200 的链上依赖/优先级反转候选；但确定性 Trace 因果投影一面将同一 typed `root_cause_primary + causality=on_wakeup_chain` 行降为背景，一面宣称“窗口内未定位到链上主根因”，构成系统内部权威矛盾。显式窗口、自动补齐仍在，无固定 4ms 降级。 |
| 1 | read_combo_config_two_knobs_precedence | FAIL | eval/results/read_combo_config_two_knobs_precedence-20260817-163840 | answer_regex,answer_contains | none | 987s | 50 | read=5,repo_map=1,list=0,trace=0,source_lens=1 | midloop=5,inv=2/1,fin_reject=14,unavail=0,prune=0 | fail | Analyzer 仍把有限双键值/层比较标为 `has_per_member_table=false`，导致 target×role 矩阵未启用；Explorer 未读 `cmd/root.go:3147`，把 retry 默认值误写为 CLI sentinel 0。Finalizer 又因精确 row_id、无引用 typed 行和通用标签绑定合同冲突连续拒绝 14 次；patch 重试重复追加同坐标 citation，降级稿引用池膨胀。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- `B1027-CONFIGVALUEMATRIXPREDICATE1`（P1）：有限多配置键、active value inventory、逐键值/层请求已经由 typed IR 表达，但错误的 `has_per_member_table=false` 让精确覆盖矩阵失活。修复应只基于 typed family/targets/profile/fields 规范化该谓词，不扫描用户或模型原文。
- `B1028-ROWIDPATCHAUTHORITY1`（P1）：精确 `source_inventory_row_id` 在 mixed/family-less carrier 内会被同名标签的弱匹配覆盖；无 citation 的 typed 行又落入“不带引用不算覆盖、带引用也不允许”的不可满足合同。精确 row identity 应优先，typed 无引用行允许以 row 身份交账并清除陈腐引用。
- `B1029-PATCHCITATIONIDEMPOTENCE1`（P1）：补丁重试对继承 citation pool 非幂等，反复追加同坐标引用并不断重映射。应按规范化坐标去重并稳定 remap。
- `B1030-TRACECHAINROOTCLASSIFICATION1`（P1）：同一投影内 typed `on_wakeup_chain/root_cause_primary` 被放入背景，同时宣称链上无主因。该项需下一独立批追踪 tier/causality carrier；不得靠系统代写模型结论或背景晋升修复。

本批安全边界：不修改 Trace 计算、时间窗选举、因果投影触发或自动补齐；不从用户/模型 prose 做硬门；不接管模型的配置值、根因结论或图关系；活跃流无固定 4ms 降级。
