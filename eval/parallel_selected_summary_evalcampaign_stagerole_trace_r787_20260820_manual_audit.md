# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T20:55:32Z
- sweep_start_ts: 20260820-135530
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | FAIL | eval/results/trace_query_wakeup_causal_io_chain-20260820-135533 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 203s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、自动补查、四跳唤醒链、11ms 链上 IO 首席、三席 1ms 调度供给、双轴与 Trace 因果投影均完整；但 typed causal rows 对三个低优先级链上依赖均给出 closed_range_stable、proven_lower_ms=1、runnable=1ms，却因 dominant_state 为 sleep/io_wait 把 priority_inversion_candidate 置 false，模型遂完全漏掉优先级维度，并继续把 wakeup path 越界叙述成直接阻塞。B1259 未复现，但本用例不构成其生产正证。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260820-135532 | answer_regex,answer_contains | none | 1666s | 68 | read=45,repo_map=2,list=0,trace=0,source_lens=0 | midloop=37,inv=10/1,fin_reject=34,unavail=1,prune=0 | fail | 34 次 finalizer 拒绝后降级，答案重复、图不合法且明确披露未产出有效 answer_document。一次预检同时发布多份 typed relation delta，但 emitFixHintsRepair 只保留第一份，租约要求修全部失败边的同时又把其余同轮失败边判为 unlisted_relation_removed/added，形成确定性自冲突。第二次 finalizer 仍同形。活动流始终健康，非超时降级；B1259 的 dataflow.Analyze 角色错配未复现，但有效成文未完成，故只能记 not-contradicted。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

### B1261-MIXEDRELATIONDELTAUNION1 / P0

- 同一预检轮可产生多个 `emitFixHint.DiagramRelationRepairDeltaJSON`；可见错误要求模型修复全部列出的失败关系。
- `emitFixHintsRepair` 当前只携带第一份非空 delta，后续租约因此把同轮其余失败关系误判为租约外增删。模型不可能同时满足“修复全部”和“不得改第一份以外关系”。
- 最优修复是只对同轮 schema-valid typed delta 做全集合并、精确去重和确定性排序；畸形兄弟项不得污染有效项。若无法完整表示全部 delta，禁止安装残缺硬租约，回到普通重试。不得扫描 Mermaid 文本推导修点，也不得由系统代写关系。

### B1260-PRIORITYDIMENSIONDOMINANCE1 / P1

- `cookie-200`、`network-300`、`threadpool-400` 均有硬优先级关系和正的 1ms runnable 片段，但 `dominant_state` 分别为 sleep/sleep/io_wait，生产逻辑据此把 `priority_inversion_candidate` 全部置 false。
- 主状态分类与独立可优化维度被错误耦合：保留 11ms IO 或 17/14ms sleep 占用，不应删除同线程已证的 1ms 低优先级 runnable/算力供给席。
- 修复必须保持不同机制分账、同区间不重复计价；没有硬低优先级关系或没有正的 runnable/running gated impact 时仍不得铸造优先级候选。

### 回放不变量

- 两路都没有基于 4ms、4m、首字节、stall 或累计年龄降级。
- Trace 显式用户窗、自动补齐、链上根因权属、背景隔离和模型答案作者权保持不变。
- 不从用户原文、模型 reasoning/final prose 或 Mermaid 字面推导硬门。
