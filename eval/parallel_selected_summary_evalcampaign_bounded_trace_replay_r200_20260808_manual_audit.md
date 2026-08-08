# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T08:37:47Z
- sweep_start_ts: 20260808-013746
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-013747 | answer_regex,answer_contains | none | 133s | 26 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B336 v2 生产正证：completion 从 r199 的 20 轮降为 1 次且零拒绝，source lens 9→1；最终四 stage、职责和三条 Mermaid precedence 边完整。模型首次 emit_evidence 把每个 salience 拆成相邻独立对象，主体证据被保留但多花一轮重发，记 B338。 |
| 2 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260808-013747 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 179s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 主根因仍严格来自已证链上 threadpool-400，logger 保持背景且未参与根因归因；但模型把 11ms io_wait+1ms runnable 写成 12ms D/IO 等待，并把 logger 的 7ms 折算背景值写成实测 iowait，和 typed 19.5ms 状态墙钟冲突。记 B339 上下文口径 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Human correctness：**1 PASS / 1 PARTIAL**。B336 v2 已生产闭环；Trace 链上/背景权限正确，但模型正文的状态区间与折算值口径仍不精确。
- `EVAL-B336=S37bkm2-production-positive/closed`：图表 explorer 一次 completion 即通过，repo_map/source lens/总时长均显著下降；finalizer 零拒绝，Mermaid 可渲染。
- `EVAL-B338-EVIDENCEMETADATAFRAGMENT1=P2-confirmed`：合法 JSON 数组中，模型把 item 级 `salience` 发成紧邻的 metadata-only object。可以仅在“紧邻前项、字段闭集、无冲突”三条件同时成立时无损合并；其它形 fail loud。
- `EVAL-B339-TRACESTATEINTERVALCALIBER1=P1-confirmed`：finalizer 同时看到分析期未经证据确认的 12ms sub-topic 描述和 typed 11ms io_wait；又看到 background effective=7ms 与 cumulative state=19.5ms，最终混淆。最优修点是 typed/soft 上下文口径：sub-topic 仅作覆盖计划；wakeup 结束 sleep/io_wait、其后到 sched-in 是 runnable；effective attribution 不等于实测状态墙钟。
- Trace 主因仍是链上 threadpool-400；logger 的 19.5ms/7ms 两种量都保留但只能作为背景。系统没有替换、删除或改写模型结论；finalizer reject/repair 均为 0。
