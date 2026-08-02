# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T02:09:28Z
- sweep_start_ts: 20260801-190927
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260801-190928 | log_regex,answer_regex | none | 155s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 `17`、2 条逐行贡献与 reconcile pass 正确，普通标量没有被 reference-output 图误拦；但 deterministic fallback 把同一 `orders.csv` 的 aggregate/child 两个别名臆造成 source/reference，先后执行 normalize/apply/mapping，11 批才完成。 |
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260801-190928 | log_regex,answer_regex | none | 657s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 首次错误 roster 输出时 live decision 已正确为 `continue/output_incomplete_reference`，最终 `17,0,5` 后才 complete，DATASTATE3 跨真实流程覆盖；但中间仍出现 `next_stage=complete/allowed=[]` 与 decision continue 的次级矛盾，已另批统一。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Detailed findings

### `data_multifile_reference_projection`

最终值 `17,0,5` 正确：inactive 行被排除，GroupA/GroupB/GroupC 的贡献可复算，
GroupX 按 `targets.csv` 的完整 reference universe 补零，reconcile 通过。首个
`assemble_answer` 只给出已有分组且带键名时，live `OutputProjectionGraph` 已携带
`reference_key_count=3`、不完整状态和合法修复方向，`Decision` 同步发布
`continue/output_incomplete_reference`；最终纯 CSV 三项形成后才发布 complete。因此
`EVAL-B32-DATASTATE3` 已由真实多文件、跨批次流程覆盖，普通 runner PASS 不再掩盖
“live complete、后置门拒绝”的双权威。

人工审计同时发现次级一致性问题：输出图和 decision 要求继续时，ledger-only stage 仍为
`complete`，allowed set 仍为空。提交 `b218cf65c` 让精确 OutputProjectionGraph 只在业务
账本已完成、输出仍不完整时把有效 stage 重开到 `emit_output_contract_answer`，并从同一
typed stage 开放 `assemble_answer`。它不读取答案语义，也不替模型生成或改写最终值。

本例耗时 657 秒、15 批、5 次 repair、7 个失败 action。错误 filter 值和多次输出格式修复
仍属于高成本过程样本；暂不按字段名或 case 固化硬门，后续用不同 reference/join case
判断是否存在新的共享参数契约缺口。

### `data_basic_sum_with_rules`

最终 `17`、两条 amount 贡献及 reconcile 均正确，精确 output graph 的负臂也成立：普通
非 reference 标量输出没有被错误标成 incomplete reference。

但初始 typed contract 从未要求 entity resolution。材料覆盖后，系统把
`coverage_records.json` 与它的 child alias `orders.csv#records` 当作两张独立表，仅凭 schema
相似度自动铸造 `normalize_entities`，随后级联 `apply_entity_resolutions` 和多个
`mapping_candidate`。两者的精确 lineage root 都是 `orders.csv`，这是 deterministic
fallback 扩权，不是模型随机选择。

提交 `ba6bf1b6a` 在自动关系脚手架铸造点要求两侧各自拥有独立 lineage root；同源
aggregate/child、父/子或重复别名不再自动 self-join/normalize，真实独立 source/reference
仍保留。显式模型动作不受此保守 fallback 限制。完整 dataworkflow/repl 测试通过。

同批全测还捕获到上一轮违规时效的边界：`Result` 与 typed `Violations` 可以同记录共存，
这样的结果不得清除自身违规。`ba6bf1b6a` 将成功进展边界收紧为
`Result present && Err empty && Violations empty`，修复后进展仍能让旧违规退为历史。
