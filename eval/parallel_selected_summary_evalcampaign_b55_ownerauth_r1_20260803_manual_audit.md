# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T01:13:28Z
- sweep_start_ts: 20260803-181327
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260803-181328 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 130s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 显式 1.000–1.010s 窗、worker-200→app-100 唤醒链、#1=8.300ms、实际占时/规则可消两轴、因果投影与系统补采均保留。唯一 reject 是 Finalizer 漏搬 Explorer 已提交并验收的 relation_claims，属于无可见语义收益的重复格式合同（B55-RELCOPY1）。final prompt 还把同一 #1 seat 重复两次、同一 target census 重复三次（B55-CTXDEDUP1）。模型正文把 prio=120 一处称为 system，与同段规则和 typed facts 的 ohos_rt 自相矛盾；final 输入已三次明确给出正确值，暂记 B55-PRIOWORD1/model-watch，不按答案词面加硬门。 |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260803-181328 | trace_attachment,answer_regex | perf_triage+trace_query | 180s | 44 | read=2,repo_map=3,list=0,trace=2,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | MarkerPID 不再被升级成同步 B/E 配对键；答案正确说明匿名 E、同 ftrace 线程栈闭合、86.111ms 与 16.67ms 阈值及缺少 sched/binder/IO 的证据边界。heavy-compute 明确降为 pretriage navigation candidate，未冒充 typed 根因；零 final reject。个别“H: 前缀平台归属/同进程重叠歧义”措辞比源码锚点更宽，但 prompt 已给通用 marker 语义，暂记模型措辞观察，不拟合专名。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
