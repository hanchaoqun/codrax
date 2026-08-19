# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T19:25:41Z
- sweep_start_ts: 20260819-122539
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260819-122541 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 142s | 44 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 233.190ms 窗口四态闭合：running=157.248ms、runnable=5.604ms、sleep=70.338ms、D=0；per-CPU running 枚举完整。CPU0/4 的 policy limit 被限定为窗口背景，因缺目标 running slice×policy 时间重叠而诚实判定“无法证明目标受限”。该有限影响问题没有被扩成 root-cause/完整因果投影，邻近频率事实未晋升主因。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-122541 | diagram,component_responsibilities | none | 610s | 43 | read=17,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=7/0,fin_reject=4,unavail=0,prune=0 | partial | B1180 生产正证：同一确定性 completion 从 r739 的 CPU/RSS 失控卡死降至 61–149ms。答案仍有系统性关系失真：为覆盖 BusContext/Mutable，typed candidate 把错误分支的 Scheduler→Mutable.SetResult 和 Explorer 内部 renderExplorerToolBudgetPlan→append 与 applyStageOutput 的 append→BusContext 拼成可见图；它们分别是真实局部边，却不是用户要求的主 read-mode 数据流。四次成文拒绝及 488 条累计 evidence 说明候选门在追逐局部 participant incidence，而非保持已证主脊柱+未证边界。需按关系 scope/同一 owner/value-flow component 收窄候选，不能以本例关键词硬补，也不能由系统代画。 |

## 人工结论

- B1180 已完成生产闭环：精确 owner 反向 DP 缓存消除了确定性资源爆炸，未裁证据、未缩语言矩阵、未改变 4-hop 上限。
- 新立 B1182/P1：required-diagram participant 修复候选允许“真实但与请求关系 scope 无关”的局部边共同通过，最终图形式有边、语义却不是请求的数据流。最优修向是让候选必须位于请求主脊柱的同一 typed operation component，或诚实保留 unproven boundary；禁止跨 owner/跨 value-flow component 拼桥。
- Trace 案例未观察到因果投影、自动补齐、链上-only 根因或 4ms 活跃流退化；本题是有限事实+单一影响判定，因此零完整投影是正确 scope，而非缺失。
- 本轮没有畸形 JSON、旧稿降级、空答案或系统替模型成文；QF 的问题来自输入给模型的 typed 候选语义边界不精确，模型仍是可见图与结论作者。
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-122541 | answer_regex,answer_contains,mermaid_edge_count | none | 610s | 43 | read=17,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=7/0,fin_reject=4,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
