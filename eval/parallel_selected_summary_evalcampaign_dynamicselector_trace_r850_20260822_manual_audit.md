# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T08:42:05Z
- sweep_start_ts: 20260822-014203
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-014205 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 系统面通过：显式窗口、完整唤醒链、链上 IO 等待 11ms、优先级反转候选、邻近/背景降权和 Trace 因果投影均保留，零成文拒绝。模型正文一处把链路时序写成“阻塞影响逐级传递”，随后又正确披露当前只证明唤醒依赖而非资源持有者；属于软性表述波动，不能用输出关键词硬改正文。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-014205 | answer_regex,answer_contains | none | 221s | 33 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=4,unavail=0,prune=0 | uncertain | 最终结论基本正确，但 typed dynamic-selection candidate 未发布，模型自行拼接关系图后连续 4 次被精确关系合同拒绝并最终缩图。新增诊断确认两个 csv/json 候选均因 `return_unavailable` 被拒；赋值、查表、入口调用和参数流均已存在，缺口是解析器精确返回事实未进入专用关系证据通道。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
