# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T09:55:21Z
- sweep_start_ts: 20260817-025520
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-025521 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 155s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass-core/context-partial | B960 生产闭环：route=optional 且孤立 allow 被归一为 default，两次日志均明确跳过源码图，未再扫描 4,187 文件。显式窗、链上 worker-200、8.300ms 有效归因、9.000ms 累计、10.000ms target sleep、3.500ms 背景分流、Trace 因果投影和系统补采均完整，无 4ms 降级。B961 的 runnable 区间误绑本轮未复现；正文仍先称“典型优先级反转”后又承认只有 candidate，最终上下文已明确 `not_authorized_mechanisms=priority_inversion_occurrence`，记模型服从波动，不扫正文、不替写。新确认 B962：Router 把用户要求分析的状态/关系内容擅自扩成“时间线视图+状态表+关系链表”并置 `requires_diagram=true`，Analyzer 又据此制造 hard sequence 合同；用户没有展示要求，属于展示权限无来源。 |
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260817-025521 | log_regex,answer_regex | none | 242s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B959 生产闭环：首稿的串行 schema 错误经 1 次实际结构修补后进入执行，随后 10 个 typed batch 完成 4/4 材料、9 条规则、14 决策、5 实体解析、3 contributions、reconcile=pass，最终严格输出 `17,0,5`。没有重复参数/重复失败循环。242s/10 batch 是效率债观察项，不能以放宽 schema、猜业务值或跳过 reconcile 换速度。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
