# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T09:28:40Z
- sweep_start_ts: 20260822-022840
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-022840 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 146s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 系统面完整保留显式窗、四线程三条唤醒边、11.000ms 链上 IO 第一席、三个独立 1.000ms 候选、实际占时/规则可消双账户、背景隔离、自动补采和 Trace 因果投影，零成文拒绝。模型已明确调用点不证明具体对象/持有者/后端，但仍把该 IO 等待称为“沿唤醒链向上传导的阻塞源头”，超过 pre_wakeup_dependency 的关系口径；继续作为 soft-guidance 遵循项，不扫描或改写正文。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-022840 | answer_regex,answer_contains | none | 273s | 33 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=4,unavail=0,prune=0 | uncertain | B1330-G 获得生产正证：finalizer 首次发射 csv/json 两条完整 typed dynamic-selection candidate，六类 hop 齐全。但候选非 call hop 只作为文字清单，未进入统一 edge_recipe/edge_anchor_json 配方；模型仍手工把 lookup、MRO、return 拼成 call，4 次拒绝后删除用户要求的 diagram。最终正文事实基本正确，关系图与动态边界表达未闭环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
