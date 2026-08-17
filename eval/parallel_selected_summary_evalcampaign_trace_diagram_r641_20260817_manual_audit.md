# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T20:46:10Z
- sweep_start_ts: 20260817-134609
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260817-134610 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 227s | 32 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B1010 生产闭环：模型正文和系统事实附录均锁定用户选择的 2.000..2.020s 精确窗，Σ五态=20ms、窗长=20ms；四个 query 视图、五维覆盖、完整 threadpool→network→cookie→app 链和 11ms IO 首席均保留，目标直接阻塞仍诚实未证。残余为模型软性过述：把跨 CPU 迁移扩写为跨 NUMA；三个互不重叠的 1ms runnable 席虽可作方向总量，但应继续清楚区分单席 max-only 与方向去重总量。系统没有改写模型结论。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-134610 | answer_regex,answer_contains,mermaid_edge_count | none | 325s | 41 | read=12,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | B1011 只部分生效：阶段先后图合法且正文正确，但 BusContext/Mutable 没有任何连向阶段的有向边。导航先选择 forcedReadCancelled 内局部 busCtx.Context()，后选择 EmittedAnswerSymbols()，均只触及一个请求参与者；真实 BuildAgentContext 的 Mutable: bus.Mutable 投影、dispatchStage 的 Execute/Apply 链未进入 typed 关系证据。Finalizer 正确拒绝模型虚构的 stage→carrier 边并删图边；新确认 B1012：缺口是跨组件载体投影/交接的软导航排序与结构续读，不应降低关系证据门或由系统造边。探索达 21 轮、12 次 read、7 次中途提示，属于高 ROI 导航效率债。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
