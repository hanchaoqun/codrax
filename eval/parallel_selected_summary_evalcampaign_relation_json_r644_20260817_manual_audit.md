# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T21:43:58Z
- sweep_start_ts: 20260817-144357
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260817-144358 | log_regex,answer_regex | none | 143s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=6,plan_errors=2,fin_reject=0 | pass | 可见答案严格为 `{"ids":["u1","u3"]}`，无围栏和解释；但工作流把带脚本的 `custom_transform` 保留为延迟产出路径，调度合同又永久禁止延迟脚本执行，形成 B1015 自冲突。模型被迫以 derive/compute/reconcile/assemble 六轮绕行，正确性通过、效率与合同一致性不通过。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260817-144358 | answer_regex,answer_contains | none | 255s | 28 | read=15,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 终稿完整列出 12 个生产实现，并以 12 条 `implements` 关系组成合法 Mermaid 图；没有成文拒绝或系统补边，B1014 在 direct type-relation 面未复现。15 次读取/7 次提示说明取证仍偏重，但没有发现事实或关系缺失。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner：`2/2 PASS`；人工正确性：`2/2 PASS`。
- `qf_type_relation_loop_controller` 给出关系图生产正证：12 个类型、12 条实现边均来自源码，图语法和方向正确；系统没有替模型绘图或补造关系。
- `data_json_strict_ids` 的最终 19 字节答案正确，但确认 P1 系统 GAP `B1015-DEFERREDSCRIPTSTAGECONTRACT1`：前缀拆分把脚本节点保存到标称“typed action queue”，`deferredActionAllowed` 又无条件拒绝任何带脚本的 `custom_transform`，生命周期却把 `stage_not_allowed` 解释为可保留。最终投影账同时把 `custom_transform` 列为合法 producer，导致模型收到“应执行、但当前不能执行”的矛盾合同。
- 最优修复不放开陈旧脚本：前缀拆分只延迟可重放的无脚本 typed action；遇到脚本后缀即在真实上游工件形成后要求重规划。旧 checkpoint 或漏网脚本用 typed `script_requires_replan` 淘汰，不能无限保留，也不能扫描用户或答案 prose 来判断。
- 本批未进入 Trace 路径；显式窗、因果投影、自动补齐、链上-only 主因以及背景 support-only 不变量未改动。两条活跃流均没有固定 4ms 无答案降级。
