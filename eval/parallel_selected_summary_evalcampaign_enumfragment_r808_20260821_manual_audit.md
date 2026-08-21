# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T11:14:21Z
- sweep_start_ts: 20260821-041421
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-041421 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 247s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 精确 2.000..2.020s；4 节点唤醒链；11.000ms 链上 IO 第一席；3 个独立 1.000ms 优先级候选；主要占时/规则可消双账和完整 Trace 因果投影均在。邻近/背景未加冕，无固定 4ms/4m 降级。调用点只按调用点披露，未伪造具体等待对象、持有者或后端。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-041421 | answer_regex,answer_contains | none | 536s | 38 | read=30,repo_map=3,list=0,trace=0,source_lens=1 | midloop=26,inv=7/1,fin_reject=1,unavail=2,prune=1 | partial | 最终答案有合法四阶段 sequenceDiagram 和逐阶段输入/输出/状态载体表，且模型自行按 typed precedence 修正关系；但 46 轮探索、30 次读文件。Analyzer 未显式给 sub_topics，R2 却把支持性实体 emit_analysis/emit_answer_document_v2 按共同前缀拆成两条内部调查线；前置 trace/log 阶段的文字顺序也有局部自相矛盾。B1290 原始畸形 enum 未自然复现，不能宣称生产转正。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. Trace 人工通过。目标窗、目标线程状态墙钟、链上 IO 等待、唤醒方向、根因排序、实际占时与规则可消两种口径以及系统补齐的因果投影相互一致；背景压力只在背景区展示。活动流持续输出期间没有按固定毫秒或分钟阈值恢复旧稿或生成降级答案。
2. read 的 runner PASS 不等于过程无 GAP。最终答案由模型完成，并在一次 typed relation reject 后用 `emit_answer_document_patch` 把图收窄为 analyzer→explorer→extractor→finalizer 三条已证 precedence；系统没有替模型选择边或改写结论。图对用户要求基本够用，但相较第一稿丢掉 Orchestrator/BusContext 交互，关系表达仍偏薄。
3. 最终正文对条件性 LogTriage/PerfTriage 的顺序存在局部矛盾：开头说它们在主管道前，StageAnalyze 段又说 analyze 完成后才决定触发。该问题不适合靠扫描正文关键词硬拒；继续通过准确的 typed topology 与模型教学回放观察。
4. 高 ROI 确认项为 `B1291-COUPLEDPERMEMBERTABLESUBTOPIC1`：请求要求一个耦合 sequenceDiagram，并额外要求逐阶段表；`HasPerMemberTable` 只证明存在同级表面，不证明 AnalyzerHints 中任意同前缀实体属于表的成员宇宙。旧 R2 将支持性 `emit_*` 实体派生成两个 sub_topics，造成不必要的双路探索、证据膨胀和成文关系返工。
5. B1291 只读取 `DiagramHint.Required/Kind`、`HasPerMemberTable`、枚举边界、完备性义务和 question buckets。明确枚举/完备性/多桶仍可拆分；单个耦合 flow/sequence/call_dag/architecture 图加同级明细表保持共享调查。没有读取用户原文、模型原文、答案或 Mermaid message，也没有替模型决定图、关系或结论。
6. B1288 的 live `addition_ref` 在 r807 已有“发布+模型选择”正证，但 r808 通过整块替换完成，原子 addition-ref 执行仍仅由测试闭环；B1290 在 r808 没有自然触发，仍是生产调用链测试闭环、待真实畸形参数触发。
