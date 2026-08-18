# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T08:21:21Z
- sweep_start_ts: 20260818-012119
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-012121 | answer_regex,answer_contains | none | 126s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | B1055-v2 仍未 production 闭环：本轮 analyzer 发出 `question_kind=mechanism` 与 `entities=[monotonic_now_ns,cmd_sleep]`，却未发 `exact_targets` 或 required dimension。模型自行发出 handler 调用行，所以 `cmd_sleep` 部分可引用；三个平台 API 仍由 definition-site 表格承载，selected-body producer 无新增行。另有一次 member-set support-ref 修补。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-012121 | answer_regex,answer_contains | none | 362s | 34 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=6/0,fin_reject=1,unavail=0,prune=0 | pass | 最终正确否证了用户假定的线性链：`buildAnalysisIR -> RunWith` 与 `gate.Run -> RunWith` 是 shared-callee。Mermaid 合法、两条边方向与 typed recipes 一致，图后中间函数均有源码锚。效率较差：analyzer quote provenance 1 次重试、Explorer 多次端点/member-set 修补、finalizer 1 次把主边与支持项拆块，总耗时 362s；未发生答案代写。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### sr_c_platform_fork

- r672 的 analyzer 输出与 r671 再次不同：仍是 `mechanism`，但 `requested_answer_dimensions` 直接为非维度答案，`exact_targets` 缺席，只保留 analyzer 主实体 `monotonic_now_ns`、`cmd_sleep`。因此 v2 的 exact-target 兼容臂没有触发。
- 模型本轮主动把 `cmd_sleep` 的循环守卫和末尾采样发成 call evidence，handler 部分得到精确引用；平台 API 仍只在 definition evidence 的 summary/聚合说明中，最终表的三行位置只是函数定义首行，不能单独证明 API 调用与换算。
- 泛化修向升级为 B1055-v3：机制符号候选取 `ExactTargets ∪ PrimaryEntities`，只做规范化符号尾部完全相等；禁止使用后续日志、子主题、实现者扩张后的宽 `Entities`，并继续要求模型已选择定义、已读行、parser provenance 和非背景 context role。

### qf_sequence_analyzer_gate

- 源码事实不是用户假定的 `buildAnalysisIR -> gate.Run`：`buildAnalysisIR` 在 `analyzer.go:2724` 调 `gate.RunWith`，`gate.Run` 在 `gate.go:135` 也调 `RunWith`。最终答案明确纠正为 shared-callee，而没有为满足问题字面虚构方向边。
- 时序图的 `Caller->>Middle` 与 `Gate->>Middle` 均有 typed call recipe；中间函数移到非 `principal_path_edge` 支持块，避免把同一函数体内的普通调用伪装成端点链。
- 362s 和多轮修补表明低心智仍有提升空间：`relation_scope_quote` 来源验证、decorated member 的 support-ref 形、exact endpoint 与实际 wrapper mismatch、principal-edge/supporting-item 拆块分别消耗轮次。它们是合同教学/结构编译整合候选，不构成系统自冲突，也不应靠当前函数名专用规则处理。
- Mermaid 渲染合法，无系统补写关系或替换模型结论；本批没有触碰 Trace 因果投影或流式发布阈值。
