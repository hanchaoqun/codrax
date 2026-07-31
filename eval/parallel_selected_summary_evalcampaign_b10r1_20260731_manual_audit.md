# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T19:01:21Z
- sweep_start_ts: 20260731-120119
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260731-120121 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 36 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 114.940ms 窗、四态、根因排序、唤醒链、可消除量和 Trace 因果投影均完整；但覆盖块误选自动补采的 120ms 窗。正文又把正值 10.331ms 供给折算缺口写成“排除了算力不足”，并将可能重叠的前三席直接相加为 53.468ms，违反 typed 口径。 |
| 1 | read_combo_trace_current_code_boundary | PASS | eval/results/read_combo_trace_current_code_boundary-20260731-120121 | trace_attachment,answer_regex | perf_triage+trace_query | 184s | 37 | read=6,repo_map=1,list=0,trace=4,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Y1 生效：pretriage 原因已是 `reason_candidate` + `causal_authority=pretriage_model_extraction`，不再进入聚合事实。新暴露的 typed gap 是 `root_evidence:trace_gap` 仍铸为 `observed_direct_cause`；模型据此把“无 sched 数据”反向解释为“无抢占/无睡眠的纯计算”，与 `tier=data_gap/context_only` 冲突。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

runner 的 2/2 PASS 只证明 fixture 的最低词面 oracle 命中，人工正确性为
0/2。两例都不是单个 span 名或单个模型句子的拟合问题，而是 typed 权限/
窗口口径没有贯穿到最终消费面。

### Mixed trace + source

批 Y1 的 `PerfJank.CausalAuthority` 已完整生效。perf bundle、explorer
上下文和 finalizer 均把 `heavy-compute` 显示为
`reason_candidate`，authority 为 `pretriage_model_extraction`；稳定闭包也只
确认 86.111ms 慢 span 与缺少 scheduler 数据，没有再把该 reason 写进聚合
事实。

最终答案仍声称 `trace_gap` 表示线程处于单一执行片段、没有被抢占或睡眠，
进而推出“纯计算负载”。日志给出确定根因：两份
`root_cause_data_gap` 记录已正确位于 `artifact_span`，但它们各自的
`root_evidence:trace_gap` 副本仍位于 `observed_direct_cause`。缺证据被权限
层包装成直接因果，模型的错误结论由此获得了结构化支撑。这应在观测铸造层
统一修复；禁止扫描“纯计算/无抢占”等答案词面。

### D4 demand vs supply

显式窗口能力没有回退。最终答案携带完整的 Trace 因果投影，分析窗为
34579.472865..34579.587805（114.940ms），四态闭合，根因排名、唤醒链、
可消除量、系统自动补采均在场。主方向“需求侧占主导”也与证据一致。

仍有三个系统级问题：

1. 覆盖边界块选择了更早一次 34579.470..34579.590 的 120ms 查询结果，
   覆盖显式 114.940ms 窗的状态账，形成两套 running/runnable 数值。自动补采
   可以扩窗，但不得夺取 principal window authority。
2. typed 根因板同时发布主线程 10.331ms 的供给折算缺口席，正文却写
   “排除了算力不足/供给压力极小”。正确的泛化结论应为“需求侧占主导，
   供给侧不是主因但存在有界次级候选”，不能把非主因升级成不存在。
3. 前三席 23.994/19.041/10.433ms 可能跨线程、跨链段重叠，投影自身反复
   标明墙钟不可加和；正文却直接相加成 53.468ms。排名值可比较，不等于
   可求和，除非 typed fold 明确为 `sum_disjoint` 或 interval union。

## 后续批次

- Z1/P1：把 `trace_gap`/data-gap 的 root-evidence 副本统一降为 coverage
  provenance，并在 typed guidance 明确“缺少区间不能证明连续执行、CPU
  占用、未抢占或未睡眠”。
- Z2/P1：覆盖摘要按 typed 显式请求窗优先选择状态账；自动补采宽窗保留为
  supplemental，不改变因果投影和补采行为。
- Z3/P1：从 typed rank/supply seat 生成 demand-dominant + secondary-supply
  指导，并固定席位默认不可相加；不扫描用户输入或模型答案原文。
- Z4/P2：复核 36 次唤醒/34 次 CookieMonsterCl 是否有单一 typed census；
  若只有分支/样本计数，则补统一计数权限，否则仅记模型波动。
