# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T08:27:24Z
- sweep_start_ts: 20260822-012723
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-012724 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 135s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 系统面完整保留 2.000..2.020s 显式窗、四线程唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms 候选、实际占时/规则可消双账和 Trace 因果投影；邻近/背景未升为主因，未按固定 4ms/4m 降级。模型正文仍把“上游依次唤醒+链上候选”写成“依次传导阻塞影响”，虽然后文 caveat 收回直接阻塞/对象/后端证明，属于既有软教学遵循波动，不能据此扫描或改写正文。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-012724 | answer_regex,answer_contains | none | 221s | 30 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=3,unavail=0,prune=0 | uncertain | 最终类、查表、构造、回调及装饰器作用均正确，Mermaid 合法；但图只保留 run_pipeline→resolve 和返回，动态 selector 候选上下文仍未发射，模型靠自行综合写出 resolve→cls，首稿因此有 3 次关系拒绝/patch。生产 ledger 已有 selector application、indexed write/lookup、return、entry call、argument 六类精确事实，故缺口位于候选 compiler 合取而非采证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
