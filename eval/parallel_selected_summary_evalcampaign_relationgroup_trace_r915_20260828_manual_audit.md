# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T00:49:37Z
- sweep_start_ts: 20260828-174935
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-174937 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 145s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、四跳唤醒链、11.000ms 链上 iowait 主根因、三个 1.000ms 反转/调度供给候选、实际占时/规则可消双账户、背景隔离、完整 Trace 因果投影和自动补采均在，且无固定时长降级。模型正文仍把 `fscache_page_wait_on_page_bit` 扩成“fscache 或页面缓存预取”修向，并把单独列为 0 的“非 IO D-state”说成线程不是不可中断等待；系统投影后文已正确写成“非 IO D-state 0 + iowait 11ms”。判为模型语义遵循 partial，未发现投影/根因权限回归，不以正文关键词硬门或由系统改写结论。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260828-174937 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 648s | 52 | read=23,repo_map=5,list=0,trace=0,source_lens=1 | midloop=18,inv=6/0,fin_reject=16,unavail=0,prune=0 | fail | Analyzer 本轮 `sub_topics=0`，B1426 的关系参与者分组没有自然触发，故仅能判无回归/待匹配生产形。read 的确定性失败来自同轮两个关系缺陷：有 anchor 的 BusContext→Mutable data_flow 只解析出半个 endpoint identity；无 anchor 的 Mutable→Finalizer 本应获得 body-only remove `failure_ref`。旧 locator 规范化保留前者半 identity，随后完整 locator 门把整份 relation delta 清空，只剩 participant additions-only lease；同时整块替换被禁止，模型无法原子删除第二条坏边，只能猜 relation/match，16 次拒绝后恢复仍含坏边的旧稿并跳过结构化 checks。确认 B1427；不是模型波动，也不是关系证据门过严，而是 typed failure 到可执行能力的断链。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
