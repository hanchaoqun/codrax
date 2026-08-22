# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T03:34:24Z
- sweep_start_ts: 20260821-203423
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260821-203424 | answer_regex | none | 115s | 27 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | Analyzer 首次即以 `relation_path` 承载跨模块调用路径，未再铸造伪 `member_set`；最终保留五条源码可复核边和 walker 的业务职责。两次成文拒绝分别源于首次手填引用序号、首次全块替换遗漏既有 `claim_uses`，属于 JSON/全块替换心智负担，未改变最终事实，列 P2 异构留观。 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-203424 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 224s | 38 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | typed/system 面保留 2.000..2.020s 显式窗、8 次查询、四线程三跳唤醒链、11.000ms 链上 IO 首席、三项独立 1.000ms 调度候选、CPU/跨核证据、实际占时与规则可消双账户、背景降格、自动补采和完整因果投影；但关系路径 patch 把运行时唤醒边伪标为源码 `call`，正文又把唤醒前的等待区间说成唤醒后耗时，并把可重叠的跨线程区间作可加和叙述，故人工判 fail。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

1. `B1321-RELATIONDIMMEMBERCOLLISION1` 获得生产正证：Rust analyzer 直接选择 `relation_path`，已有主路径一次覆盖，不再触发 sibling roster 或成员表修补。
2. `B1323-RELATIONPATHFAMILYAUTH1/P1`：关系路径是跨家族答案维度，但当前通用覆盖/patch 教学仍隐含“源码调用边”载体。Trace 唤醒链因此被要求补 `principal_path_edge`，模型随后把三条运行时 wakeup 关系错误铸成 `relation_kind=call`。应由 typed question family/authority 选择载体：源码调用关系继续使用源码 path edge 与 typed relation anchor；运行时 Trace 关系使用 model-owned principal runtime-observation path、`external_observation` 权威及既有 typed wakeup chain，禁止伪造源码 call anchor。系统只校验载体与证据类型，不生成关系或改写答案。
3. `B1324-TRACEINTERVALSEMANTICSGUIDANCE1/P1`：typed Trace 已提供状态区间与唤醒方向，但成文前缺少低心智的时间语义提示。应从 typed wakeup/state authority 生成软指导：状态区间在结束事件之前；不同线程区间可重叠/嵌套，没有 typed 串行结算凭证不得相加为端到端延迟；唤醒边只证明方向与时序，不自动证明具体业务/IO 后端机理。不得扫描最终 prose、硬拒措辞或由系统代写结论。
4. `B1325-REPLACEBLOCKMETADATALOAD1/P2-observe`：Rust 首次全块替换遗漏既有 `claim_uses`，第二次恢复后通过。先继续异构回放；若复现，再降低 full-block replacement 的字段搬运负担或提供 typed partial mutation，不针对本例 JSON 形做容错硬拟合。
