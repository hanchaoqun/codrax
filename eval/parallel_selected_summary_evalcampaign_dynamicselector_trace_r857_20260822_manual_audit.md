# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T10:59:04Z
- sweep_start_ts: 20260822-035903
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-035904 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 168s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 系统面通过：明确 2.000000–2.020000s 窗、target-filtered 自动补采、四跳唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度供给候选、实际占时/规则可消双轴、业务下钻、邻近/背景隔离和完整 Trace 因果投影均保留，零成文拒绝；活动流未按固定 4ms/4m 或耗时降级。人工保留 uncertain：模型把三跳唤醒先后表述为“传导叠加”，但证据只证明唤醒关系与各席位时长，不证明同步阻塞或整链机理；答案后文已披露该限制，继续走软教学观察，不扫描或改写正文。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-035904 | answer_regex,answer_contains | none | 183s | 33 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=5,unavail=0,prune=0 | uncertain | B1336 生产正证：第一拒后可选图进入 same-generation opaque relation lease，模型获得 failure_ref 并可自选删除/保留；但同一 patch 按兄弟 validator 要求给 chain-1 补精确 run_pipeline→resolve anchor 时，lease 又以 unlisted_relation_added 拒绝，形成确定性合同自冲突并继续消耗 4 轮。最终正文和有序链正确，墙钟由 r856 400s 降至 183s；关系图只剩 run_pipeline→resolve，且 remove_if_isolated 删除 JP/H 后仍留下 `Note over JP,H`，结构表达薄弱且存在悬空引用。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
