# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T12:37:10Z
- sweep_start_ts: 20260818-053709
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260818-053710 | answer_regex,answer_contains | none | 368s | 42 | read=22,repo_map=2,list=0,trace=0,source_lens=1 | midloop=17,inv=8/0,fin_reject=0,unavail=0,prune=4 | fail | B1074 获生产正证：Finalizer 的 Unit 1/2 都收到按 typed topic affinity 排序的 config/render 事实，旧 1024 截断不再饿死 Unit 2；终稿也不再声称所有 RuntimeSettings 字段均为指针。但 Explorer 仍未采到 cmd/root.go 的六级配置发现顺序，终稿只写默认值<YAML<CLI，并把两个空标题与随后两组列表分离；Mermaid 正常/降级路径仍有过宽表述。当前根因已从“证据分区丢失”移动到每个调查单元的机制闭包不完整，宽 contains/regex 仍假绿。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260818-053710 | answer_regex,answer_contains | none | 536s | 44 | read=21,repo_map=4,list=0,trace=0,source_lens=0 | midloop=15,inv=10/0,fin_reject=4,unavail=0,prune=2 | fail | B1073 获生产正证：route 保留 diagram_required=true 与当前消息逐字 presentation span，Analyzer/Finalizer 收到 required sequence 和 canonical stage-lane authority，终稿确为 sequenceDiagram。仍有 4 次关系门拒绝；确定性系统自冲突是同一 Finalizer prompt 一面写 Required kind: sequence，一面在 First-Pass Diagram Reference 展示 flowchart TD，诱导首稿使用错误图族和未证边。终表又声称完整序列 6 节点却只列 5 行，遗漏 StagePerfTriage，故人工不通过。Runner 仍会从整份过程文本命中已拒绝草稿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Deterministic findings and generalized fixes

1. `B1073-PRESENTATIONPROVENANCEREPAIR1` 生产闭环：路由日志明确发出
   `diagram_required=true`，逐字 presentation span 保持，Finalizer 合同为 `Required kind: sequence`，
   最终可见图也为 `sequenceDiagram`。本轮不再出现“路由清空必需图权威后合法改成 flowchart”的旧故障。
2. `B1074-MULTITOPICAFFINITYRANK1` 生产闭环：组合案 prompt 的 Unit 1/2 各自收到完整池排序后的
   `topic_affinity_hint`，包括 config loader 与 Mermaid fallback 的晚到 typed 行；标签仍是 prompt-only，
   没有进入 validator、拒答或系统代写。
3. 新确认 `B1075-REQUIREDDIAGRAMSEEDKIND1`：同一结构化 prompt 同时发布 required sequence 与 flowchart
   首轮参考。根修收敛在图种权威：有有效 `RequiredKind` 时，seed 只尝试该语义族；缓存/编译图只有
   `DiagramKindAllowsMermaidSyntax` 同族才可复用；只有定义/ownership 节点而无已证消息边时，sequence
   生成离散 participants，不退到 flowchart，也不虚构消息。测试同时钉住 retry seed 和 First-Pass
   Diagram Reference。系统仍不生成最终图、关系或结论。
4. 新确认 `B1076-MULTITOPICMECHANISMCLOSURE1`：证据分区已修好，但 Explorer 对 typed unit 的“加载机制/
   覆盖优先级”没有形成逐维机制闭包，未读取/交付六级配置发现实现；Finalizer 无法凭缺失证据补造。
   后续应在结构化 unit/dimension 与 evidence coverage 上做软补采，不得扫描用户原文/答案关键词或硬写
   某个配置案例。
5. `B1077-STAGEWORKFLOWTABLECOMPLETENESS1` 先记观察：typed authority 同时给出两个条件预阶段和四个主阶段，
   模型却在“6 节点”声明下漏列 PerfTriage。这次先视为结构化聚合消费不完整，不以正文字符串门或系统改表
   拟合；待 B1075 回放降低矛盾教学与重试噪声后再判是否稳定系统 gap。
6. 两案均没有畸形 JSON、空答案、旧稿恢复、结束误判或 active-stream 固定 4ms 降级。均无 Trace 输入；
   显式时间窗、Trace 因果投影、自动补齐、typed 链上-only 根因、邻近/背景 support-only、实际占用与规则
   可消除双轴、模型答案/图/关系/结论所有权均未改动。
