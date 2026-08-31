# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T14:27:01Z
- sweep_start_ts: 20260831-072659
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-072701 | write_plan,write_patch_oracle | none | 50s | 27 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确规划 `Main.java:16 retrun -> return`，保持 `pending_approval`，未修改源码；写模式守卫无回归。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260831-072701 | answer_regex,answer_contains | none | 970s | 73 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=23,inv=5/0,fin_reject=20,unavail=0,prune=0 | fail | 恢复稿事实、关系和图表顺序可用，但 20 次确定性关系合同拒绝、19 次 patch 后降级出厂；不能把恢复稿可读视作生产通过。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `patch_java_typo` 人工通过：仍是单文件单行低风险改动，审批、隔离和零源码副作用符合 oracle。
2. `qf_sequence_analyzer_gate` 人工失败。恢复稿正确说明 `buildAnalysisIR` 与 `gate.Run` 没有连续有向调用路径，只展示两侧汇聚到
   `RunWith` 的已证关系；Mermaid 位于 18 项关键函数清单之前，说明 B1492 的模型块重排操作已经被模型正确使用。但最终答案携带
   “最终重试未能产出有效 answer_document”与降级说明，runner 也因 `degraded_answer_checks_skipped:1` 判 FAIL，B1492 不能据此宣布生产闭环。
3. 新确认 P0 `B1493-PARTICIPANTRELATIONALIASPARITY1`。关系修补的普通生产者会把当前图声明中证据绑定的别名加入可执行候选；参与者覆盖的
   additions-only 生产者却只做 request-participant 映射。当前图声明 `Analyzer as agent.buildAnalysisIR`、`Norm as analyzerGraphForNormalize`，
   typed 证据为 `buildAnalysisIR -> analyzerGraphForNormalize`，但 lease 只发布技术端点 `buildAnalysisIR`，同时提示模型复用已声明 participant。
   模型使用 `Analyzer` 被拒、改用 `buildAnalysisIR` 又产生重复/隐式 participant，形成确定性合同自冲突，不是模型波动。
4. B1493 根修让参与者覆盖生产者与普通关系生产者共享同一个 evidence-bound declared-alias binder。它只读取 typed endpoint、证据身份和解析后的
   participant 声明；限定在候选精确端点侧，歧义或 owner-only 标签继续 fail closed，不从消息、关系标签、请求或答案文字猜身份，也不替模型选边。
5. 独立确认 P0 `B1494-REPAIRBASEOCCURRENCE1`。生产校验可在确定性归一化视图上给 `Gate -> RunWith` 铸造
   `body_occurrence=2`，安装 lease 时却绑定原始 rejected patch base；该底稿实际只有 1 条对应可见边，执行器因此稳定拒绝
   `body_occurrence=2 exceeds 1 visible edge(s)`。旧逻辑虽然重新铸造 ref，却没有把结构坐标同步回 live base。
6. B1494 根修只在 exact live base 对同一 `block_id/from_node/to_node` 恰有一条可见边时，把大于 1 的陈旧 occurrence 重绑为 1；零条、重复边、
   重复 block 或缺坐标全部保持原值并继续 fail closed。修复不读取 visible label/message，不推断关系含义，也不按运行时长、轮次或上下文降级。
7. 新增 producer-wiring 与 executor-level 回归分别钉住：参与者 additions-only delta 必须携带 `Analyzer/Norm` 现有别名；唯一 live edge 可安全重绑，
   两条同向边不得折叠。聚焦测试已绿；完整套件及 r977 恰好 2 路生产回放待本批提交后执行。
