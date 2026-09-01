# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T06:50:23Z
- sweep_start_ts: 20260831-235022
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault_symptom | FAIL | eval/results/github_issue_zod_prefault_symptom-20260831-235023 | write_apply,answer_regex | none | 225s | 27 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B1530 production-positive。补丁与回归正确；生产验证仅由 Python 静态读取 TypeScript 源码，最终 completion=unverified、proof=weak、ledger=low_confidence 且共用 production_verification_source_static_only，机器 FAIL 是诚实的 proof_weak 而非交付错误。controller 曾请求 all_verified，但确定性完成门正确降为 accept_unverified。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260831-235023 | answer_regex,answer_contains | none | 787s | 60 | read=31,repo_map=3,list=0,trace=0,source_lens=0 | midloop=18,inv=3/0,fin_reject=10,unavail=1,prune=4 | fail | B1531 production-positive：本轮已无新端点 visible-label 必填拒绝。首稿无证 call/return/state 边被正确拒绝；但 generic replace 没复用原有 Analyzer/Explorer/Extractor/Finalizer，最终另建 analyze/explorer/extractor/finalizer，造成同一阶段双节点、note 与关系链分离，并保留断开的 BusCtx。孤立参与者名单只有应用边编辑后才可得，原子事务却要求模型同轮预判，贡献多轮 roster 重试。表头还退化为“项目/列2…”泛化标题。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized Findings

- `B1532-DIAGRAMREPLACEDECLAREDALIASREUSE1/P1`：sequence add 车道已有 typed 唯一声明复用，但 generic replace 车道缺席。精确修向是在任何 add/replace precedence 或 exact typed endpoint 上复用原图中唯一匹配的显式 participant ID；有多个匹配时 fail-closed，零匹配时保留模型选择。只复用既有 carrier，不生成关系、方向、label 或结论。
- `B1533-DIAGRAMORPHANPOSTEDITTRANSACTION1/P1`：边编辑后的孤立参与者集合是 post-edit typed 事实，当前未落地事务要求模型在同一调用里预测它，导致完整大补丁反复重交。精确修向是先把模型选定的边编辑保存为未发布的 pending draft，再用该 draft 铸造完整 orphan roster；下一次只要求模型为每个精确 row 选择 remove/retain，发布前仍跑全部关系、participant 和 Mermaid 校验。系统不替模型选择处置。

## Implementation Status

- `B1532`：已由 `f802ef98f` 实现并推送；generic replace 现与其他 typed add/replace 车道共用唯一既有 participant 复用规则。
- `B1533`：已实现 typed 两阶段事务并通过 production-envelope 与动态 schema 回归；第一阶段只保存未发布关系结果，第二阶段只接收完整孤立节点处置，系统不选择删除、保留或可见措辞。待全量测试、提交推送与 r1003 生产回放。
