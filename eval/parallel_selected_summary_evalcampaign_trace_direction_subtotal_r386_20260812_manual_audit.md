# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T12:37:59Z
- sweep_start_ts: 20260812-053758
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h10_spantop_member_subrows | PASS | eval/results/real_trace_h10_spantop_member_subrows-20260812-053759 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 119s | 38 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | JIT 两成员与 target/peer 边界正确；但摘要把“全部等待”封闭为 GPU fence + 算力供给，typed 状态账仍含未归因 sleep/等待，且供给缺口不是等待分量。B647。 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260812-053759 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 166s | 37 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B644 生产生效，锁方向精确发布 12.115ms；但摘要把“没有跨方向 relation carrier”升级成“相互独立/无重叠”，随后又说不可相加，同页矛盾。B646。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

### H11 — fail；B644 已转正，但缺失关系证据被升级成物理独立

- `repair_direction_authority` 与 Final Trace Boundary 均发布同一 typed 小计：锁与优先级方向 #2 7.405ms + #3 4.710ms = 12.115ms，模型正文正确消费；没有再退回 leader-only，也没有扩成 18.853ms。
- 跨方向 `joint_total_authority=not_provided` 仍被遵守：答案未计算跨方向总和，并明确不构成同时可消除的总量保证。
- 残余 B646：摘要写“这些席位各自独立、提升空间之间无重叠证据”，关系段再写“四个修复方向相互独立”，但 typed authority 明确是 `unlisted_pair_physical_relation=unresolved`、`direction_independence_authority=not_provided`。没有重叠记录只能禁止联合计价，不能证明独立或无重叠。答案后文又写“不可相加”，构成同页矛盾。
- 这是弱模型在长上下文中把“不知道”压成肯定结论的系统上下文显著性 gap，不应通过扫描/改写终稿修复。

### H10 — fail；语义 inventory 正确，但原因分解被错误封闭

- B638/B643/B641 均保持生产正向：两条 JIT 成员、2.388ms 合计、1.781/0.607ms、不同名称和行号完整；它们被标成另一线程的邻近语义事实，没有晋升为 CompThread 自身因果或可消量。
- 模型对目标线程“没有确定性语义 span”的窄结论成立；但随后写“窗口内 CompThread 的等待全部由 GPU fence 等待和调度供给缺口构成”。typed 状态账仍有 118.586ms sleep、仅部分链上已归因且存在未归因等待；65.912ms 是 running 的供给折算影响，不是等待分量。状态分区只封闭“目标经历了哪些状态”，根因席位清单不封闭“为什么”。
- B647 与 B646 同根：未知/未封闭事实虽然散见上下文，但没有在最终 decision capsule 中与方向值同等显著，模型总结时自行补成“全部”。

结论：runner 2/2，人工 0/2。`B644=production-positive`；
`B646-TRACEUNKNOWNRELATIONSUMMARY1=confirmed/P0`；
`B647-TRACECAUSEDECOMPOSITIONCLOSURE1=confirmed/P0`；固定年龄降级、系统答案代写、原文关键词硬门均未出现。
