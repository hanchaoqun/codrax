# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T07:26:20Z
- sweep_start_ts: 20260819-002619
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260819-002620 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 206s | 43 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 有限事实题保持有限：终稿给出 Running=157.248ms、Runnable=5.604ms、Sleep=70.338ms、D/IO=0.000ms；独立 completion-closed IO 尺为至少 4 次/4.384ms，未与调度状态尺混算。CPU0/4 策略区间与转换数、目标在 CPU12/CPU4 的 running bucket 及代表频率均以自然语言发布，并明确 CPU 共位/代表频率不能证明目标切片重叠或限频，故结论仍为“策略存在、目标绑定未证”。无内部枚举泄漏、无因果投影误注入、零成文拒绝。P2 展示债：两个次级列表只显示标签，模型草稿中的行属性未随列表项渲染；关键数值已在主叙述和表格完整保留，未影响本题结论。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-002620 | answer_regex,answer_contains,mermaid_edge_count | none | 283s | 33 | read=5,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=5/0,fin_reject=1,unavail=0,prune=0 | partial | B1147/B1148 获生产正证：同一耦合关系题不再拆为六条独立车道，explorer 116→12、read_file 54→5、wall 940s→283s。终稿正确保留四阶段先后及若干有证调用；首稿两条无 typed 支持的箭头被 validator 正确拒绝，修补没有伪造关系。但用户指定的 BusContext/Mutable 数据关系仍只剩无箭头 containment，终稿明确披露未证，未完成“各阶段如何经 BusContext/Mutable 传递数据”的主体要求。日志已有 `extractStageHasRequiredWork -> BuildAgentContext` 与 `BuildAgentContext -> bus.Mutable.Objective` 局部操作，需继续核查 exact argument/声明身份是否在提取或组件闭包处断开；不得直接制造 BusContext→Mutable 箭头或放宽 gate。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusion

- B1146 的有限 CPU 事实读者语言已在真实 Trace 题生产闭环；不需要也没有注入“Trace 因果投影”。
- B1147/B1148 将耦合关系题从爆炸式独立调查恢复为共享证据调查，时长与工具 churn 大幅下降；仍存在一个跨语言通用的“参数/容器字段身份无法闭合到指定参与者”缺口。
- 新记 P2：结构化列表项携带的 attributes/values 在次级显示面可能丢失。该问题只能修渲染/authoring 载体，不扫描答案原文，不由系统代写结论。
- 两案均未观察到活跃流按 4ms 或固定总年龄降级；Trace 显式时间窗、因果投影、自动补齐以及链上-only 根因合同未改动。
