# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T20:18:44Z
- sweep_start_ts: 20260817-131843
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260817-131844 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 269s | 40 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | r639 的 Finalizer 权威提示冲突已消失：正文保留 2.000..2.020s、20ms、typed 链和 11ms IO 首席，并诚实披露目标直接阻塞关系未证。残余三项：四节点三边被模型称作“四跳”；同向 max-only 的 #2~#4 被相加为 3ms；系统附录错误选择探索期 0..2.02s 宽窗，发射“Σ五态=20.000ms;窗长 2020.000ms”。后者是确定性 B1010，前两项先按软教学/模型波动观察，禁止用答案关键词硬门。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-131844 | answer_regex,answer_contains,mermaid_edge_count | none | 314s | 39 | read=7,repo_map=5,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=1,unavail=0,prune=1 | partial | 图语法有效，但只保留阶段先后与若干不连通局部片段；BusContext/Mutable/AgentContext 孤立。探索读到 BuildAgentContext 后停在首个局部载体，没有继续覆盖同一 dispatchStage 内的 ag.Execute(agentCtx) 与 applyStageOutput(output)。最终拒绝正确删除未证边，但关系图失去主要数据流。确认 B1011：导航应沿 enclosing callable 继续寻找后续消费/回写；复合赋值调用行应软教为 call、argument、result-assignment 三个独立关系候选，系统仍不得自铸边。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
