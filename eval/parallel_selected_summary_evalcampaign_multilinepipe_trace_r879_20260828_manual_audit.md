# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T09:47:12Z
- sweep_start_ts: 20260828-024711
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-024712 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 244s | 40 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000000..2.020000s 窗、四跳唤醒链、11.000ms 链上 iowait 第一席、三项独立 1.000ms runnable/优先级候选、实际占时/规则可消双账户、背景隔离、自动补采与完整 Trace 因果投影均在；活动流没有按固定毫秒/分钟阈值降级。模型仍把 `fscache_page_wait_on_page_bit` 调用点扩写成已知磁盘/网络缓存来源，把 network/cookie 的 sleep 猜成网络 IO，并把非墙钟 IO 活动指数当成系统高负载印证，超出 typed 权威；保持软教学观察，禁止正文扫描硬拒或系统改写。系统投影另把同一个 IO 活动综合指数背景行重复发射两次。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260828-024712 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 638s | 40 | read=36,repo_map=4,list=0,trace=0,source_lens=0 | midloop=27,inv=13/0,fin_reject=7,unavail=1,prune=0 | fail | B1366 的 `codraxNode`/多行 pipe 标签破坏未复现，但首轮关系添加成功后进入新的 relation/participant 代次。patch executor 的当前 lease 在 evaluator MutableState，下一轮 schema 读取另一 AgentContext MutableState；relation-scope reject 未找到当前代次，错误落入泛化 `answer_doc.patch_correct`。模型随后猜测新 ref、又复用旧 `ra1-f418...`，连续得到 unknown/stale，最终 7 reject 后降级恢复旧稿。问题是 typed 状态载体分裂，不是 Mermaid 文案或模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
