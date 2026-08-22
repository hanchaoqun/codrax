# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T11:57:23Z
- sweep_start_ts: 20260822-045721
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-045723 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 170s | 36 | read=1,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、四线程已证唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度供给候选、实际占时/规则可消双账、业务下钻、背景隔离和完整 Trace 因果投影均在；零成文拒绝，未按固定 4ms/4m 或活动流年龄降级。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-045723 | answer_regex,answer_contains | none | 320s | 38 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=5,unavail=0,prune=0 | uncertain | 最终正文正确识别 JsonPlugin、REGISTRY 查找、cls() 构造、executor callback 与装饰器导入期注册；但经历 5 次成文拒绝。B1339 首次精确发布候选后，普通块被清空时零锚点提示未复用候选；diagram 删除/替换带内联声明的边后又丢失 run_pipeline/resolve/executor 业务节点名，最终图仅显示 rp/res/exe 且留下空 subgraph，图层明显缩水。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
