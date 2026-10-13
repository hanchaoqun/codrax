# Selected Eval Manual Audit — HMC237

- date: 2026-10-13T13:26:14Z
- sweep_start_ts: 20261013-062612
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_event_time_scope_20261013

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_lossless_sampling_tree | PASS | eval/results/hmc_event_time_scope_20261013/trace_lossless_sampling_tree-20261013-062614 | successful_tool_result,trace_attachment,answer_contains | perf_triage+trace_query | 224s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | event_search 显式 TID11 获得5条；随后 tree/perf_stats 三次继承错误父进程号10而为空。终稿按次数20%代替采样权重，未提供35/7/100分母、未知5或完整路径，并把trace文件/行号说成DSO/源码行。未自然触发typed sampling repair。 |
| 1 | trace_gzip_sqlite_late_jank_reports | PASS | eval/results/hmc_event_time_scope_20261013/trace_gzip_sqlite_late_jank_reports-20261013-062614 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 297s | 44 | read=0,repo_map=2,list=0,trace=2,source_lens=1 | midloop=7,inv=3/0,fin_reject=3,unavail=0,prune=0 | FAIL | 原生inventory及selector正确为5条，旧evidence_fact与模型叙述仍携右界第6条；终稿6条/27599=3，删除用户请求概览图，100ms归属正文也错。迟报字段保留正确，未加payload时间条件，无自然过滤repair→重试。 |

机器2/2不代表业务验收；完整答案人工0/2。原始输入、结果、草稿、日志和失败原件保持不变，不追加第三例追绿。两个独立审计及原始凭证随本批归档至 `results/hmc_event_time_scope_20261013/verification-receipts/`：`codrax-hmc237-event-live-audit-v1.md`、`codrax-hmc237-sampling-live-audit-v1.md`。实际结果属于修订 `8db257e81` 的固定二例；后续确定性补救不得倒签此轮人工结论。完整能力新增0，79唯一任务仍21交付/58开放。

人工后的共享成员名册与目标来源补救见账本§237.6–237.9。最终`c9576d81e`全仓正式exit0（90测试包PASS/13无测试包）、race及构建通过；上一轮全仓两根旧fixture失败、name-only探针失败及全部修正过程保留。它们证明代码回归状态，不证明本表两个失败答案已经重新自然生成通过。当前synthesis污染、请求图被删除、采样自然retry和全部业务量尺仍按原人工结论待验。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
