# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T15:59:25Z
- sweep_start_ts: 20260811-085924
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260811-085925 | answer_regex,answer_contains | none | 130s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | Runner 假绿。探索证据正确记录 `ConsoleSink::write` 使用 `fputs`+`fputc` 写 `stderr`，终稿却写成 `std::puts` 写标准输出；`ConsoleSink::name()` 也被误说成工厂匹配依据。首稿虚分发/实现/工厂选择图因只有直接 call recipe 而被整块删除，确认 B538；事实漂移登记 B542 watch，不扫终稿关键词硬修。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-085925 | answer_regex,answer_contains,mermaid_edge_count | none | 440s | 40 | read=11,repo_map=2,list=0,trace=0,source_lens=0 | midloop=16,inv=6/0,fin_reject=5,unavail=0,prune=0 | fail | B541 实现未获生产闭合：operation 已发射，但模型没有再发 `Orchestrator.busCtx` 字段声明，严格合取无输入；completion 的 soft plan 未教学 declaration+operation 配对。Explorer 23 轮、Finalizer 5 拒绝，最终 Mutable/BusContext 仍为 unproven 孤点；图仅保留 stage precedence 与两个辅助 call，未回答载体数据流。440s 始终活跃并最终发布模型答案，无四分钟降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- `B541b/P1-high`：类型桥的消费谓词正确，但生产 acquisition 不闭合。最优方案是在模型已经选择并接地 exact operation 时，由 parser/repomap 把该 endpoint 的唯一静态 binding/type/owner 作为 system-authored identity metadata 随 operation 发布；不要求模型另发 identity-only declaration，不创建 edge。
- `B538-DYNAMICVISANNOT1/P1-high`：C++ 虚分发、实现覆盖、工厂 return/selection 缺少统一 typed diagram relation/annotation 载体。直接 call 门正确，但不能以删图作为长期表达方案。
- `B542-EXACTLINEFACTDRIFT1/P1-watch`：同一回答中，已接地源码行的 API 与输出目标在成文时被替换成相似但错误事实。先做跨语言 structured exact-line fact/context 审计并异构复现；不得扫描最终 prose 或由系统改写答案。
- 活动流规则正证：Go 案 440s 有持续 tool/LLM/patch 进展，系统继续等待模型答案；四分钟不是降级阈值。
