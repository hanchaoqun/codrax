# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T09:37:47Z
- sweep_start_ts: 20260901-023745
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260901-023747 | answer_regex | none | 143s | 39 | read=6,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | 主链与 walker 角色、Mermaid 和引用基本完整，但正文同时声称两支“并发进行”又明确“先收集文件列表，再逐文件逐行匹配”；源码与 explorer 上下文均明确是顺序执行，属于模型自身事实矛盾。系统不应扫描正文后代写结论，留作异构复放观察。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260901-023747 | answer_regex,answer_contains | none | 391s | 50 | read=12,repo_map=2,list=0,trace=0,source_lens=1 | midloop=14,inv=3/0,fin_reject=9,unavail=0,prune=0 | fail | 四阶段顺序、输入/输出/状态载体表和三条 precedence 主链均已出厂，但最终 sequence 仍保留完全断开的 Orchestrator，答案重复且系统补齐位置不完整。九次拒绝中确认同一 typed tuple 被 replace 与 add 在同一事务重复生产，以及旧教学要求同批预测 orphan、与当前两阶段 schema 自相矛盾。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit findings

1. `B1534-DIAGRAMPAIREDRELATIONPOSTEDITCLOSURE1` 获生产正证：模型删除 forward calls 后，系统只暂存未发布图并发布 4 条精确 dependent relation rows；模型自行删除 4 条悬空 reply。系统未自动删除、改向或改写任何关系。
2. `B1536-DIAGRAMLEASEIDENTITYENRICHMENTSTABILITY1` 的 r1005 同一继承边 removed+added 自冲突在本轮未再出现；但本轮图形没有触发新 stabilization 日志，故只记“旧症状缺席”，不冒充精确生产分支正证。
3. `B1535` 的旧 `not a typed carrier` 拒绝缺席，但模型未重放旧 `n1` seed alias 形，精确 alias 正证仍待后续异构样例。
4. `B1537-DIAGRAMATOMICREPLACEADDDUPLICATE1/P1` confirmed：一次 patch 同时选择 3 个 stale-anchor replace 与相同 3 个 typed addition，编译器允许形成双份 precedence rows，直到后置关系门才拒绝。应在 typed 原子事务编译期按 block/relation/canonical identity pair 检出同一事务的重复 producer，并要求模型选择 replace 或 add；不得静默丢弃任一模型操作。
5. `B1539-DIAGRAMORPHANTEACHINGPHASECONFLICT1/P0` confirmed：动态 schema 与执行器已明确“第一阶段只提交 relation、第二阶段才发布完整 orphan roster”，但 evaluator prompt 和全局 patch teaching 仍指导模型在同一 patch 预测 `diagram_participant_edits`。模型因此连续提交错误分支并消耗多轮。修向仅统一 typed phase teaching：只有当前 schema 真正发布 participant branch 时才允许提交；不扫描请求、thinking、答案或 Mermaid 文本。
6. `B1538-DIAGRAMSTAGEDORPHANLINEAGE1/P1` conditional：最终 Orchestrator 已断开却失去 eligible disposition，说明中间 relation-error generation 可能切断 staged orphan lineage。先修 B1537/B1539 并复放；若无中间重复事务后仍复现，再实现 generation fingerprint 绑定的跨 relation-generation roster 继承，避免叠加补丁。
7. 本批没有 Trace 用例；沿用 r1005 已验证的不变量：显式窗、链上根因、实际占时/规则可消双账户、因果投影和自动补齐均不得被 diagram 修补改动，也不存在固定 4ms/4m 或活跃流年龄降级。
