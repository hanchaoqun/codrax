# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T10:27:14Z
- sweep_start_ts: 20260829-032712
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260829-032714 | log_attachment,answer_contains | log_triage | 114s | 27 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Runner 的字面 oracle 通过，但答案把 ArkTS 首个观测帧 `NativeBridge.invokeOhSum:33` 错换成调用者 `HomePage.computeTotal:54`，把两个无显式关系标记的顶层栈扩写成 native 传播关系，并泄漏 `peer error occurrence`、`cross_error_relation=unproven`、`<unverified-external-source>`。根因是仓库解析失败后系统清空了附件内的精确路径，且最终成文缺少按 occurrence 分组的第一帧/调用者 typed 交接。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260829-032714 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 46 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 精确 114.940ms 用户窗、四跳唤醒依赖链、链上优先级反转/调度供给/算力供给/D-state/IO、确定性 `VerifyClass` 业务线索、实际占时与规则可消量双轴、邻近/背景隔离、完整 Trace 因果投影和自动补齐均在；帧因果未证也被如实限定。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
