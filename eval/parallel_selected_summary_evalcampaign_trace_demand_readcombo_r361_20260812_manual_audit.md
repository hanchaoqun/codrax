# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T04:36:14Z
- sweep_start_ts: 20260811-213609
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260811-213614 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 178s | 41 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、链上板、双轴、VerifyClass 与背景边界均在；但正文把 #5–#7 跨席相加成 17.609ms，又把多类需求席相加成 44.082ms。还把正值 10.331ms 算力供给缺口绝对化为“供给充足/并非瓶颈”，并残留无 holder/waiter 权限的“锁”措辞。日志证明 Finalizer 已收到逐席比较、禁止跨席求和和 compute_delivery_positive 边界；Explorer 的 bounded tool preview 却在这些边界之前截断，早期错误综合污染了闭环。B603 因此是 transport-order partial，而非投影缺数据。 |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260811-213615 | trace_attachment,answer_regex | perf_triage+trace_query | 269s | 40 | read=6,repo_map=0,list=0,trace=2,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B604 生产正例：模型放弃 skill/defaults 教学字符串，转读 parse.go/query.go/trace_mark_integrity.go 的可执行实现，正确说明 B/E 栈与 S/F cookie 配对，并把 86.111ms runtime 实测、当前源码机制和 frame/jank 未证三层分开。独立 GAP：模型提交 7 个逐成员有源码支持的函数，但 Required Principal Member Set 将大小写不同的 isTraceMarkPayload/IsTraceMarkPayload 折叠为 6，末尾又补发 7 项并产生硬转软披露；确认 B606 case-fold identity gap。整案 269s 超过四分钟仍正常交付，未因管线累计时长降级；这不是单次模型流超过四分钟的生产见证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
