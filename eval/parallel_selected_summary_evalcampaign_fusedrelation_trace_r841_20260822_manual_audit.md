# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T04:53:22Z
- sweep_start_ts: 20260821-215321
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-215322 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 234s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 用户窗、6 次 target-filtered typed query/自动补采、threadpool→network→cookie→app 链、11.000ms 链上 IO 第一席、三项各 1.000ms runnable/优先级候选、实际占时/规则可消双账户、链上业务下钻、背景隔离和完整 Trace 因果投影均在。模型明确说明 wakeup 只证明依赖、不证明同步等待，也没有把 fscache 调用点升级为具体后端；0 次成文拒绝。活动流 234s 正常完成，没有固定 4ms/4m 降级。 |
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260821-215322 | answer_regex,answer_contains | none | 238s | 33 | read=12,repo_map=3,list=0,trace=0,source_lens=1 | midloop=6,inv=3/0,fin_reject=5,unavail=0,prune=0 | partial | 终稿正确给出 run→fetchUser→send→dispatchOnce→fetch 与 send→sleep→setTimeout，且精确保留 `@app/core`→`packages/core/src/index.ts`；引用事实正确。但本轮首稿把列表和图分别提交且二者都没有 anchors，因此没有自然触发 B1326 的 fused-owner 路径，不能据此把生产验收转正。5 次拒绝中前三次是模型遗漏 anchors/错误使用 atomic add/未同时保留列表修补，后两次暴露 B1327：系统的“copy-ready/validator-aligned” skeleton 仍把 raw `call` 枚举画成可见消息且 anchors 无 reader label，模型照抄后被同系统的 reader-label 门拒绝，再改成“调用”才通过。该模板教学在所有源码语言/关系种类上可复现，不应按 TypeScript 文案特判。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
