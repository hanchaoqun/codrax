# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T14:03:44Z
- sweep_start_ts: 20260803-070342
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260803-070344 | trace_attachment,answer_regex | perf_triage+trace_query | 298s | 44 | read=10,repo_map=1,list=0,trace=1,source_lens=0 | midloop=5,inv=4/2,fin_reject=0,unavail=0,prune=0 | pass-core / relation-gap | Runtime span、86.111ms、16.67ms budget、证据边界和当前源码机制最终齐全；首个 Explorer 错判“仓内无实现”被第二 evidence task 纠正。新 GAP：final handoff 明示 explicit_caller_callee_edges=0，模型仍把 5 条跨解析/转换/分类/agent 的逻辑步骤全部标成 relation_kind=call，系统只记 diagram_edges advisory，未拒绝 typed 伪调用边。另有 2 次 aggregate negative_observation schema 修形，属精确提示后自愈。 |
| 1 | trace_query_wakeup_causal_runnable | FAIL | eval/results/trace_query_wakeup_causal_runnable-20260803-070344 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 538s | 44 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=16,inv=1/0,fin_reject=34(旧 runner 双计；实际17),unavail=0,prune=0 | fail-system | 探索期 accepted target-state claim=15ms/旧 ID；补采后的 final typed authority=10ms/新 ID。final handoff 同时要求复制新 authority 和保留旧 claim；validator 对 include/omit 分别硬拒，构成不可满足合同。连续 17 次实际成文拒绝后人工 SIGTERM，避免继续烧至 1200s；根因不是模型。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
