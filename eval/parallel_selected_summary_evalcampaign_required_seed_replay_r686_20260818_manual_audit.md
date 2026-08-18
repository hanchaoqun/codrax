# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T12:56:48Z
- sweep_start_ts: 20260818-055646
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260818-055648 | answer_regex,answer_contains | none | 261s | 31 | read=19,repo_map=0,list=1,trace=0,source_lens=0 | midloop=12,inv=6/0,fin_reject=0,unavail=0,prune=0 | fail | 两部分已分节且不再声称所有 RuntimeSettings 字段都是指针，但仍把 settings 发现简化成“工作目录 codrax.yaml”，未交付六级查找顺序；CLI override 也被泛化成所有 flag。Mermaid 节把 OutcomeLibraryRejected 与 retry gate 的关系写得过宽，最后又由系统补充暴露“主路径上的关系未完整呈现”。B1074 分区不是当前根因，B1076 的逐 unit 机制闭包仍开放。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260818-055648 | answer_regex,answer_contains | none | 588s | 51 | read=23,repo_map=4,list=0,trace=0,source_lens=1 | midloop=10,inv=5/0,fin_reject=6,unavail=0,prune=0 | fail | B1075 获生产闭环：Diagram Contract 与 First-Pass Diagram Reference 均为 sequence，跨族 flowchart 已消失；终稿图和四阶段表也保留。新 P0 是关系 scope 假连接：Analyzer 把仅属于表格示例的 AnalysisIR 留作 incident participant，弱连通算法又把多个函数共同返回的 nil 当成共享节点，向模型提供 AnalysisIR.MarkHypothesis→nil 候选；终稿最终画成可见的 AnalysisIR→BusContext 伪关系。另有 6 次 patch 拒绝，模型多轮推理已说删除两条未证边但 payload 仍保留；先修确定性假候选，再观察 patch churn，不能放松 typed 关系门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Deterministic findings and generalized fixes

1. `B1075-REQUIREDDIAGRAMSEEDKIND1` 生产闭环：required kind、首轮 reference 与最终载体三面均为
   sequence；首轮 reference 是无边 participant 集，未再出现 flowchart，也未由系统生成最终关系。
2. 新确认 P0 `B1078-TERMINALRETURNCOMPONENTJOIN1`：flow participant 的弱连通图按 endpoint 字面量全局
   合并，多个不相关函数共同 `return nil` 会通过 `nil` 拼成一张假关系图。该假组件不仅改变 coverage，还把
   单个局部 return 作为 requested-relation repair candidate 发给模型，最终出现技术身份
   `AnalysisIR.MarkHypothesis -> nil`、可见文案却是 `AnalysisIR -> BusContext` 的伪关系。
3. 根修只消费 typed `AnchorReturn`：return 的目标是这一条返回操作的局部 sink，不再按相同拼写跨操作合并。
   一条直接的 `ToolA -> Payload` return 仍可覆盖两个明确请求参与者；若 Payload 后续进入别的消费者，需要
   独立 parser-owned assignment/argument/data-flow 证据，不能仅靠同名终值搭桥。正负 pin 均不读请求、推理、
   答案或 Mermaid 消息词面。
4. `B1079-DIAGRAMPARTICIPANTSURFACEPROVENANCE1` 留档：analysis skill 已明确禁止把 sibling table 的 example/
   state carrier 当图参与者，但本轮模型仍违反软教学；现有 tool 只核验 exact source quote，无法精确证明该名字
   属于哪一个展示面。暂不按词序/关键词硬删 participant；先由 B1078 保证错误 roster 只能得到 disconnected+
   unproven，不能获得伪边，再设计显式 per-surface typed provenance。
5. `B1080-PATCHPAYLOADSTALEEDGECHURN1` 观察：6 次拒绝中，模型推理多次正确识别应删边，但随后 patch payload
   仍保留同两条边。这一轮尚不能证明是系统覆写还是模型结构化输出波动；关系门正确拒绝，不以降杆止血。
6. r685 的 `B1077-STAGEWORKFLOWTABLECOMPLETENESS1` 本轮未复现：用户要求 analyze→finalizer，四主阶段表完整，
   条件预阶段不属于该区间；降为非 gap，不增加强制六行合同。
7. 两案无畸形 JSON、空答案、旧稿恢复、stream 误停或固定 4ms 降级。均非 Trace；显式时间窗、Trace 因果
   投影、自动补齐、链上-only 根因、邻近/背景 support-only、实际占用/业务线索与规则可消除双轴均未改；
   系统没有接管模型答案、图、关系或结论。
