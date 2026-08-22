# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T09:14:56Z
- sweep_start_ts: 20260822-021455
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-021456 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 175s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 系统面完整保留显式 2.000..2.020s 窗、四线程三条唤醒边、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度候选、实际占时/规则可消双账户、背景隔离和 Trace 因果投影，零成文拒绝。模型却把 pre_wakeup_dependency 候选写成 network/cookie 被依次阻塞，并把调用点扩写为具体 fscache 页面或网络文件系统对象；同一答案又承认等待对象、持有者与直接阻塞未证，属于模型机理过度主张。保持 soft-guidance 观察，不扫描或改写正文。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-021456 | answer_regex,answer_contains | none | 175s | 32 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=4,unavail=0,prune=0 | uncertain | return 与 entry typed 生产证据均在，但 finalizer 仍报告 entry_unavailable=2；模型因缺少完整 typed candidate 自行拼接 lookup/return/callback，连续 4 次关系拒绝后删减图。正文核心事实基本正确，关系闭包和图指导性未转正。精确定位为 evidence pool 仍以 OwnerSymbol 优先筛 call，导致 qualified owner 在 compiler 前遮蔽精确 Subject。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
