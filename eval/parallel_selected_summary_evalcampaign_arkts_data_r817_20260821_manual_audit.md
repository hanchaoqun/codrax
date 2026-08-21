# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T17:01:32Z
- sweep_start_ts: 20260821-100130
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260821-100132 | log_regex,answer_regex | none | 183s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 规则、过滤、贡献和 reconcile 均完成，但系统最终把未声明 complete_reference 的 targets.csv 仅当候选，默认按 reconcile 的 GroupA/GroupB/GroupC 排序输出 17,4,5；没有按 T1/GroupA、T2/GroupX、T3/GroupC 投影并给 GroupX 补 0。错误发生在 typed output projection 收口，不是算术或材料缺失。 |
| 1 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260821-100132 | typed_inventory_rowset,answer_contains | none | 514s | 45 | read=5,repo_map=2,list=0,trace=0,source_lens=2 | midloop=4,inv=1/0,fin_reject=20,unavail=0,prune=0 | fail | 探索正确找到并逐文件验证 4 个 @Entry 与 2 个 @Builder；Principal Enumeration Rows 也明确发出两族 6 行。repo lens 把 @Entry 记在装饰器行、证据记在 struct 行，旧准入只接受同一行，校验域因此只剩 @builder，连续 20 次拒绝正确 @component row_id 后降级旧稿。可见内容虽完整，但结构化清单失败、重试风暴和降级说明均为系统 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusion

- `B1299`（数据引用投影）：系统在发现一个未确权的结构化 reference candidate 时，不应自行按现有 reconcile groups 终结；应回到 typed 规划轮让模型明确选择 complete-reference 或 present-only，再由既有逐槽 grounding gate 校验。禁止从 instructions/request prose 做硬扫描或由系统替模型选择目标集合。
- `B1300`（跨行声明身份）：多语言装饰器/修饰符和声明可落在不同行。准入应使用唯一的 `file + member + typed surface_family` 结构身份桥；同文件同名同族歧义继续 fail-closed。该方案适用于 ArkTS、Cangjie 及其他装饰器语言，不按单个 case 或固定行距拟合。
- 本批不涉及 Trace 执行面；显式时间窗、Trace 因果投影、系统自动补采、链上根因和 4ms/4m 活动流策略均未修改。
