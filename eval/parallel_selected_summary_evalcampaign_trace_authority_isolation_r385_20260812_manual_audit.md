# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T11:54:01Z
- sweep_start_ts: 20260812-045359
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h10_spantop_member_subrows | PASS | eval/results/real_trace_h10_spantop_member_subrows-20260812-045401 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 122s | 43 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B643 生产生效：JIT exact family 只剩一席；模型逐条保留 TextView 1.781ms 与 DecimalQuantity 0.607ms/各自行号，不再以目标线程零命中否认 typed inventory。 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260812-045401 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 187s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B642 生产生效：无 18.853/8.622/31.4 或“独立可叠加”污染；但系统附录精确发布锁方向 7.405+4.710=12.115ms 互斥小计，Finalizer handoff 却固定声明小计未提供，模型依后者错误降为 leader=7.405ms。B644。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

### H10 — pass

- `deterministic_semantic_spans` 只发布一个 JIT family；投影不再出现同一 exact occurrence 的 adjacent/background 双席。
- 最终答案保留两条不同成员、1.781/0.607ms 与各自行号，且只把它们当确定性语义工作清单；没有提升为目标线程已证因果。
- 邻近/背景没有进入主因，显式时间窗、链上排序、双轴与自动补齐均保留。

### H11 — fail；确定性系统合同冲突，不是模型波动

- B642 正向：Finalizer 上下文不再泄漏模型 aggregate 内嵌的方向加法/独立性；正文没有 18.853ms、8.622ms、31.4ms 等无凭证值。
- 但 post-finalize ◎ 总览已精确发布：`锁与优先级 · 2席 · 小计 12.115ms(区间互斥)`；同一轮 pre-final `repair_direction_authority` 却写
  `same_direction_subtotal_authority=not_provided; published_direction_value=leader_only`。
- 模型遵循高显著 handoff，将锁方向上限写成 7.405ms，丢掉系统自身已有的精确互斥小计。根因是 subtotal 判定只活在 tool 私有渲染函数，agent handoff/最终边界另抄固定 leader-only 合同。
- B644 最优方案：把区间存在、未合并、同板、逐对互斥、按展示 `%.3f` 微秒相加的判定提升为共享 typed 算术；agent 必须消费 tool 的同一展示模型、折叠、TOP-N 与语义 fallback 结果。精确小计存在时发布小计；重叠/缺包络/跨板时仍 leader-only/禁止；跨方向 joint total 永不顺带授权。

结论：runner 2/2，人工 1/2。`B642/B643=production-positive`；`B644-TRACEDIRECTIONSUBTOTALAUTH1=confirmed/P0`。
