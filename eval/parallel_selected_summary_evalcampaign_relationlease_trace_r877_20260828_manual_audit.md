# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T09:01:08Z
- sweep_start_ts: 20260828-020106
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-020108 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 235s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、5 个 typed 维度、四跳 threadpool-400→network-300→cookie-200→app-100 链、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双账户、背景隔离和完整 Trace 因果投影均在；仅一次 schema 位置修补，没有固定耗时降级。模型却把 `fscache_page_wait_on_page_bit` 扩写成磁盘读/缓存页可用机理，并把 pre_wakeup_dependency 写成“阻塞上游唤醒信号传递”，而同轮 typed 上下文明确 completion/direct-blocking dependency 均未提供；继续归 B1269/B1271 软遵循观察，禁止 prose 硬门和系统改写。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-020108 | answer_regex,answer_contains,mermaid_edge_count | none | 547s | 40 | read=31,repo_map=6,list=0,trace=0,source_lens=5 | midloop=20,inv=8/0,fin_reject=1,unavail=0,prune=2 | partial | B1363 生命周期生产烟测正向：无 stale/lease-absent、无“全 unchanged”空补丁；一个真实 node/identity 冲突直接获得 live failure_ref，模型一轮局部删除后通过。但最终图仅四阶段三条边，Mutable 与 BusContext 仍为孤立节点；模型明明拿到 `bus.Mutable→AgentContext.Mutable` typed fragment，却先错误映射为 `BusContext→Mutable`，被正确拒绝后选择删除。旧 runner 只数边仍误签 PASS；新 B1364 eval-only incident-node oracle 回算为 4，低于该 case 的 6，不进入生产硬门、不读关系文案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
