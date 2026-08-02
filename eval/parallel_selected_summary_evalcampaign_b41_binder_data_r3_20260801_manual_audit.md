# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T11:26:04Z
- sweep_start_ts: 20260802-042602
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260802-042604 | log_regex,answer_regex | none | 38s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终值 `2` 正确。初始 terminal script 虽声明两个材料却只读取 notes；AST 消费证明在执行前拒绝并直接进入 repair，修复脚本实际读取 instructions+notes 后一次执行成功。`data_rounds=1`、`repair_rounds=1`、`action_failed=0`；evaluator 未再用 planner success_criteria 幻觉值阻断。DATAPREFLIGHT1/DATAEVAL1 的本 witness 关闭。 |
| 2 | trace_query_binder_ipc_peer | PASS | eval/results/trace_query_binder_ipc_peer-20260802-042604 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 95s | 27 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 模型正文对三个有限事实均正确：同步请求、目标 PID100/TID101、transaction=42、直接 waker=binder:100_1-101；analyzer 也首次接受 `bounded_fact_set`。但 pre-finalize floor 仍错误要求 heavy causal drill，assembly supplement 又执行 `root_cause_rank+critical_blocking_calls`，在 3 次模型查询外注入系统状态/根因墙与补采披露。最终约 15.3K，虽较 r2 25.3K 收窄，仍违反有限事实宽度。登记 SUPPBREADTH1/FLOORBREADTH1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner: 2/2 PASS；human: 1/2 PASS。
- Data 修复已生效，且拒绝发生在执行前，不再制造一次失败动作。
- Binder 的模型层与 report materializer 已正确收窄；剩余问题是两个旧 control-plane
  旁路没有消费共享 typed breadth authority。它们必须按结构化权限修复，不能按 Binder
  关键词或答案文本特判。
