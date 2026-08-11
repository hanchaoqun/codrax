# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T18:49:45Z
- sweep_start_ts: 20260811-114944
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-114945 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 145s | 27 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 固定 5.000000..5.007000s 请求窗、app-100 四态、worker-200→app-100 typed 唤醒边、4.600ms 类校验链上席、0.800ms runnable、实际占用/规则可消双轴、背景降权、Trace 因果投影与系统补采均保留；但模型把 `pre_wakeup_dependency` 越权解释成“app 等 worker 完成类校验后才被唤醒/类校验导致唤醒延迟 5ms”。span 实际到 5.005400s，而 wake 在 5.005000s，且 prompt 已明确 `wakeup_path_blocking_authority=not_implied`；这是 B544 的再次生产 witness，不可用 prose 关键词硬门。 |
| 2 | github_issue_gson_lazy_number_symptom | FAIL | eval/results/github_issue_gson_lazy_number_symptom-20260811-114945 | write_apply,write_patch_oracle | none | 147s | 22 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 源码 patch 的 equals/hashCode 值语义正确；`make check` 实际执行 Python 源码 oracle 并通过 1 条真实断言，Java probe/direct-main 因 JDK 缺失而不可用，故整体不升级为 verified 是正确的。系统却把 typed 的“1 passed + Java unavailable”压扁成“没有断言验证过”，controller context 也只给 failure summary，模型进而错误声称 Python 检查需要 Java。确认 B561：部分验证证据在 controller/final rendering 投影丢失；必须披露部分通过，同时维持完整行为未验证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B544-TRACECLAIMCALIBER1` 从 watch 提升为 `P1-soft/context-carrier-first`：typed handoff 已有 phase 与 blocking-authority 边界，但模型再次把时间重叠写成“等待完成/直接延迟”。后续只能增强结构化关系语义与时间端点显著性，或引入 answer-side typed mechanism carrier；禁止扫描模型/终稿原文硬拒，禁止系统改写结论。
- `B561-PARTIALVERIFYPROJECTION1/P1-high`：多验证通道的最终 `unavailable` 判定本身正确；错误在证据投影。`ChangeReport` 已保留 `make-test passed=true`，但 write-controller prompt 和 unverified card 把它抹成零断言。最优修复是从 `TestResults`/typed status 同源投影 `passed/failed/total`，显示“已有局部检查通过，但 Java 行为验证不可用”，不把静态检查升级为行为证明。
- 本轮两案都在 147s 内结束，不涉及 B560 的长流边界；B560 已由独立单测固定“真实模型进展跨 4 分钟不降级”。
