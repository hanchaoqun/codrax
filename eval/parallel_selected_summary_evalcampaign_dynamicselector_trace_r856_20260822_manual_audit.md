# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T10:42:05Z
- sweep_start_ts: 20260822-034204
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-034205 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 235s | 40 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | uncertain | 系统面通过：明确 2.000000–2.020000s 窗、四线程唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度供给候选、实际占时/规则可消双轴、邻近与背景隔离及完整 Trace 因果投影均保留；零成文拒绝，活动流未按固定时长降级。模型把已证唤醒先后进一步写成 IO 等待“导致无法及时唤醒并传导整链”，强于证据自身，后文又披露未证同步阻塞；归既有模型结论强度观察，不增加 prose 硬门或系统改写。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-034205 | answer_regex,answer_contains | none | 400s | 39 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=2/0,fin_reject=5,unavail=0,prune=0 | uncertain | B1335 生产正证：finalizer 收到 csv/json 两个完整 typed candidate，精确 `resolve -> cls()` return 未再丢失，B1334 candidate-first 提示也自然触发。最终正文/有序链正确连接 run_pipeline、resolve、REGISTRY、@register、cls()、JsonPlugin 与 executor/super 协作链；但模型自选可选时序图连续 5 次违反端点/关系/标签合同，400s 后删图才通过。关系正文可用，结构化表达和重试成本未闭环，新增 B1336。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
