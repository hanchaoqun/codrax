# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T15:11:56Z
- sweep_start_ts: 20260811-081155
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-081157 | answer_regex,answer_contains | none | 367s | 40 | read=18,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=4/0,fin_reject=2,unavail=0,prune=1 | fail | B540 反臂生效：Analyzer 纠正后发布空 participant slate，独立表格中的 BusContext 等未再变成图硬席。答案有 sequenceDiagram 和表，但图被 relation gate 压成 stage precedence 加一条孤立 dispatch call，未表达真实跨阶段状态流；表中把 pipelineTopology 列作状态载体、把 recordTaskFinalize/EventObjectiveDone 放入 Finalize 输出链也不够精确。两次成文修补后仍由模型交付；367s 活跃流没有系统代答/四分钟降级。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-081157 | answer_regex,answer_contains,mermaid_edge_count | none | 400s | 40 | read=15,repo_map=2,list=0,trace=0,source_lens=0 | midloop=13,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | B540 正臂暴露 all-outside 静默清空：Analyzer 把过窄 scope 与五个 scope 外 participant 同时发出，旧 parser 只 warning 后发布空 slate。Explorer/Finalizer 因而丢失用户明确点名的 Analyzer/Explorer/Extractor/Finalizer/Mutable/BusContext 关系义务，最终图仅剩四阶段顺序；正文又把 BuildInitialInstruction、Mutable 写入和跨阶段传递混在一起。确认 B540b 必须 fail-loud 促使 Analyzer 扩 scope；400s 活跃流未发生系统降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner 的 `2/2 PASS` 只证明弱 oracle 命中，人工为 `0/2`；Mermaid edge count 与关键词在场不能证明用户要求的关系已经交付。
- B540 对“sequence + sibling table”反臂有效，运行时间从 r318 的 904s 降到 367s，但仍有两次 relation patch，答案事实精度与 carrier flow 表达未闭环。
- QF 逻辑视图精确复现“全部 participant 越界后静默清空”缺口；修复应发生在 typed emit-analysis 边界，不能靠扫描最终答案或由系统补边。
- 两案分别运行 367s/400s，期间 LLM 流和结构化修补持续活跃；系统保持等待，最终发布模型答案，没有因单次 4 分钟请求预算写出降级答案。
