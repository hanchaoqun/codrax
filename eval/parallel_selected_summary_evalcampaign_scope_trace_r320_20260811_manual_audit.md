# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T15:23:06Z
- sweep_start_ts: 20260811-082305
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-082306 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 183s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 显式窗、Trace 因果投影、自动补齐、链上 VerifyClass 4.600ms 主席、runnable 0.800ms 次席、实际占用/规则可消双轴、帧证据 absent 与帧因果未证限定均完整；背景 CPU 供给没有冒充链上主因。模型却把“app-100 没有 tracing_mark_write B/E span”扩写为“没有任何帧处理或渲染工作被执行、仅处于调度等待”，缺少 instrumentation 不能证明没有执行，且同页已有 running=1.200ms。主结论可用，但该证据口径越界使人工判为 uncertain，后续只做 typed 口径软教学，不扫描答案硬改。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-082306 | answer_regex,answer_contains,mermaid_edge_count | none | 450s | 45 | read=13,repo_map=3,list=0,trace=0,source_lens=1 | midloop=9,inv=8/0,fin_reject=3,unavail=0,prune=1 | fail | B540b 正臂生效：Analyzer 修正后用同一关系面合法保留 Analyzer/Explorer/Extractor/Finalizer/Mutable/BusContext 六席，额外 Orchestrator 越界行被逐行删除。Mutable 可由精确成员操作覆盖；BusContext 仍无法与 `o.busCtx.EvidenceItems` 这类源码端点建立精确类型身份，三次成文修补后图中 BusContext 仍是孤点，用户要求的载体数据流没有形成。确认 B541 不是模型波动，而是跨语言声明类型/receiver identity 缺少 typed projection；450s 活跃流继续等待模型，未发生四分钟系统代答或降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner 2/2，人工为 `uncertain + fail`；不得把 runner PASS 当作关系表达或证据口径闭环。
- B540b 已获生产正证；下一高 ROI 缺口是 B541：以解析器给出的声明类型/receiver 绑定对齐业务 participant 与精确操作端点。该投影只能服务 identity/coverage，不能生成 edge、改变关系方向或替模型画图。
- Trace 核心不变量本轮完整；“span 缺失即未执行”只登记为证据口径软教学，不建立最终答案关键词硬门。
- 两案在有模型流、工具调用或修补进展时均没有按 4 分钟降级；活跃流不得由系统合成或替换模型答案。
