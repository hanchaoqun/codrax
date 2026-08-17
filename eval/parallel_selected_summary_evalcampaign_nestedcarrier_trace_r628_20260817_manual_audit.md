# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T15:46:49Z
- sweep_start_ts: 20260817-084648
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-084649 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 202s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗内链上 #1 worker-200 8.300ms、target sleep 10.000ms、跨核 CPU2→CPU1 权限边界、Trace 因果投影和背景 support-only 均保留，模型没有再宣称同核竞争。B989：探索阶段 1.000..1.011 的宽窗结果被旧 ±1ms 同窗容差并入 principal value，最终 target/CPU 主值面出现 11.000ms、10.020ms 与 1.011 尾行，污染用户 1.000..1.010 显式窗；链证据本身未丢。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-084649 | answer_regex,answer_contains,mermaid_edge_count | none | 409s | 39 | read=26,repo_map=2,list=1,trace=0,source_lens=0 | midloop=13,inv=5/0,fin_reject=2,unavail=0,prune=0 | fail | B987 仍未生产闭环。第一次 exact completion 先读 `cgec_enforcers.go:767-791`，再读 `contract_check_block.go:3501-3525`；终稿仅保留四阶段顺序边，Mutable/BusContext 仍断开。深审确认 scoped/multi-repo projection 保留完整 FileIndex，却可省略旧 SymbolDefs；成员身份晚解析只读后者，因而从未看到 `BusContext.Mutable` 唯一声明。B988 已改为复用完整导航索引，仍只给下一次读取坐标。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusions

- Runner 为 `2/2 PASS`，人工为 `QF=fail`、`Trace=partial`。Mermaid 边数 oracle 仍无法证明用户点名的载体参与者已接入数据流；Trace oracle 也未覆盖“主值必须来自用户显式窗而非邻近探索窗”。
- B988 的根因不是 parser 无信息。生产图的 FileIndex 已包含 `BusContext.Mutable`、`AgentContext.Mutable` 及 `ctx.Mutable` 调用关系；断层在晚解析只消费可能被投影省略的 legacy SymbolDefs。修复 `577fc86e4` 统一使用 FileIndex 派生的完整 navigation index，要求 exact member、exact typed owner、requested scope 和唯一 surviving declaration；它不发射 evidence、关系边或答案。
- B989 的根因是历史 F-2 ±1ms 聚合容差同时被主值消费。修复 `6e35a8ec5` 保留该容差用于探索/因果聚合，只让 target state account 与 CPU occupancy 主值用 2µs 端点容差附着到选定窗，避免 1ms 扩窗因总量更大而夺取 principal seat。
- Trace 仍保留显式窗、链上-only 主因、实际耗时/新修向与规则可消量双轴、跨核权限、因果投影和自动补齐；邻近 1.011 数据仍可作探索背景，但不得改写 1.010 主值。系统没有扫描用户/模型原文设硬门，也没有生成或替换模型结论。
- 两案成文均在活跃语义输出后继续完成；QF 409 秒和 Trace 202 秒均没有固定 4ms 活跃流降级。
