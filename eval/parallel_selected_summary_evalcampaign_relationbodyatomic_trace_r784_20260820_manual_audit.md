# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T19:18:06Z
- sweep_start_ts: 20260820-121805
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-121806 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 271s | 37 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000–2.020s 用户窗、自动补采、Trace 因果投影、11.000ms 链上 IO 首席、三个 1.000ms runnable 调度席、主要占时/规则可消双轴和背景隔离均保留；成文一次成功，活动流无时间降级。模型正文仍把上游 IO 候选写成“直接决定后续延迟”“阻塞整个链路”“目标阻塞完全来自上游”，而同页 typed authority 明确 wakeup path 不蕴含目标直接阻塞、目标直接阻塞关系未建立；B1253 再次稳定复现。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260820-121806 | answer_regex,answer_contains | none | 1530s | 65 | read=23,repo_map=2,list=0,trace=0,source_lens=0 | midloop=12,inv=3/0,fin_reject=20,unavail=0,prune=6 | fail | 模型形成了可读的一图一表草稿，但系统连续拒绝 20 次并最终降级发布旧稿。B1254 的单例 body-only 路径命中，但同端点存在另一种已证 anchor 时，执行器以 pair-level any-anchor 错误否决无锚坏 relation；模型即使给出 body_occurrence 仍收到 exact-prior-anchor 错误。其余轮次还反复加入未携带 canonical identity 的候选边，被 lease 正确拒绝。首轮另有 4 个表格行使用位置型 citation index，局部修复却携带 80k+ 上下文。Explorer 两段、23 reads、约 198 条共享证据，Finalizer 上下文由 66k 膨胀至 121k；失败是系统合同/上下文问题，不是模型无答案或活动流时间降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
