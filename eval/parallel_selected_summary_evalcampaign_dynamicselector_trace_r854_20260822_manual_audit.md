# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T10:03:02Z
- sweep_start_ts: 20260822-030300
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-030302 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 182s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 系统面完整保留显式窗、四线程三条唤醒边、11.000ms 链上 IO 第一席、三个独立 1.000ms 候选、实际占时/规则可消双账户、背景隔离、自动补采和 Trace 因果投影，零成文拒绝。模型开头正确限定尚无同步阻塞者/锁持有者，但仍把已证唤醒先后表述成“由一条跨 CPU 的依赖唤醒链引起”，超过 pre-wakeup dependency 的直接因果口径；继续作为 soft-guidance 遵循项，不扫描或改写正文。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-030302 | answer_regex,answer_contains | none | 278s | 38 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=5,unavail=0,prune=0 | uncertain | B1333 的 csv/json candidate relation recipes 在初始提示中完整出现，但五次拒绝的 optional-diagram repair 首选了 generic component fragments；candidate 配方未被重复到模型当前修补焦点。最终图只保留通用 call/callback，并加入 resolve→KeyError 与 resolve→cls 支持分支，注册、查表、factory return、JsonPlugin/type 关系均未显示。正文主要事实正确，但图没有回答动态解析关系，B1333 仅初稿供给转正，重试接线仍开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
