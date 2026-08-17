# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T19:10:31Z
- sweep_start_ts: 20260817-121029
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-121031 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 278s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 控制枚举已退出读者正文；显式 1.000000..1.010000 窗、worker-200 链上 #1=8.300ms、邻近/背景分权、Trace 因果投影与自动补齐均在。但模型把 runnable 等待误写成“低优先级线程持有 CPU”、把 sleeping 目标写成“无法获得调度”，并臆造“等待 worker 工作完成/直接阻塞”。typed 上下文已明确否定这些越界机理，故是末端机制边界显著性不足而非查询缺证。后续只从 exact cause family 派生自然语言 permitted/not-proven scope，禁止扫描或改写答案。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-121031 | answer_regex,answer_contains,mermaid_edge_count | none | 622s | 48 | read=10,repo_map=4,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=6,unavail=0,prune=1 | partial | stage 顺序的自然语言边标签、BusContext/Mutable 无箭头 grouping 与 Mutable 未证边界均保留；但 6 次 final reject/patch 才过。候选只给 `participant_endpoint_side` 与技术 identity，模型反复猜 `BusContext` 如何映射到 `o.busCtx.Mutable`；终图重复画 BusContext，且把 raw `argument_flow` 显示给用户。后续候选增加 exact participant node mapping、技术 identity 仅留 anchor 的提示及 relation-kind 同源自然标签；仍由模型选边和成图。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case decision

- B1001: Trace 末端读者标签缺少同席位机制边界。精确 typed 数据足够，但允许机理与未证机理离最终 cause label 太远，模型仍会把 runnable、sleep、running supply、D/IO、semantic span 混写。修复必须是 exact typed cause family → 紧凑自然语言 scope 的软引导；不得读原始问题/答案做关键词门，不得替模型定因或改写正文。
- B1002: 图关系 repair candidate 缺少“失败 participant 节点 ↔ 技术 endpoint anchor”的可执行映射和读者边标签，造成真实关系证据难以落图及内部枚举泄漏。修复只扩充候选 carrier，不自动选择候选、不铸边、不改关系方向。
- 两案活跃流均远超 4ms 并正常完成；不得以固定 4ms 或固定总年龄触发草稿降级。
