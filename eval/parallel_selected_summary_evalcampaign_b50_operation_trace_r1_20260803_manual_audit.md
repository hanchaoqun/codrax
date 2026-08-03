# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T01:43:01Z
- sweep_start_ts: 20260802-184300
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260802-184301 | log_regex,answer_regex | none | 93s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 两次 curl 即闭环：首页 32,655 bytes/3,390 visible runes，随后由 typed href 定位 user_guide.html；手册 248,161 bytes/118,802 visible runes 被 20 个连续 material pages 完整覆盖，source/pages 均未截断并铸造 complete receipt。evaluator 只依据 receipt 判 complete，模型自行完成 8 章总结；没有 shell 抽取循环、假完整或系统代写。93s 较前次 141s 改善，但单样本不作为性能基准。 |
| 2 | real_trace_h5_smr_multirow_disposition | FAIL | eval/results/real_trace_h5_smr_multirow_disposition-20260802-184301 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 260s | 43 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=3/2,fin_reject=2,unavail=0,prune=0 | fail | runner 只因旧词形 `等待对象 dma_fence_default_w` 失败；typed kernel callsite 已在正文/投影。显式 233.190ms 窗、window_stats/root_cause_rank/wakeup_chain/critical_blocking_calls、自动 frame_root_cause_bundle、双维根因、可消除量及 Trace 因果投影均完整保留。人工仍 FAIL：模型先称“全部主要…完整覆盖”，但系统精确边界为 enumeration_status=incomplete（critical/root/span 均 capped）；又把 rank 有效影响 3.670ms 写成“6 段”，而 typed occurrence authority 是至少 6 段、union wall clock 至少 4.611ms。最终 prompt 对 target-blocking 已足量，后者留作模型波动；前者另查模型前是否缺 concise typed enumeration handoff。另发现 runtime-only 上下文同时写 current_source=false/0 与“runtime and current-source proof both present”，登记 CTXAUTH1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B49-HTMLBODY1`：live covered。完整材料 pages/receipt 贯通 command evaluator 与 final answer，模型拥有摘要和结论。
- `EVAL-B46-ORACLE1`：继续属于 eval debt；生产代码不得为固定中文前缀拟合。
- `EVAL-B50-CTXAUTH1`：confirmed/P1-small。runtime-only optional-source 的 typed snapshot 允许完成，但 prompt renderer 分支顺序误称双来源均在场；修复只调整 typed guidance。
- `EVAL-B50-ENUMCTX1`：confirmed/implemented/tests-pass。最终系统 appendix 原有精确 enumeration_status/boundaries，但模型成文前缺同等 concise handoff；现由共享 typed compiler 同时供给两面，模型仍拥有结论。
- `EVAL-B50-H5IO1`：model-variance/watch。模型已收到 `>=6 occurrences / >=4.611ms / lower_bound_capacity_truncated`，仍把 3.670ms rank impact 与 occurrence roster 混用；禁止用答案关键词扫描或系统重写处理。
