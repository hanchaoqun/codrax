# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T09:55:15Z
- sweep_start_ts: 20260801-025513
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-025515 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 146s | 38 | read=2,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B19h-a 只覆盖了前两种 analyzer 标签变体；本轮同一请求又发出 `intent=root_cause / scenario=root_cause / kind=conditional / diagnostic=true`，系统再次补跑并发布全量根因报告。主值 3 次/0.635ms 正确，但 model 把 `prev_state=D` 错解为“被 Hilogd 强制抢占”；`prev_state=D` 表示被切出任务进入不可中断等待，不是 still-runnable preemption。记 `EVAL-B19-SCHEDPROSE1/P1`。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-025515 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 160s | 39 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗的占用双轴、可消除量、根因排序、唤醒链、投影与补齐全在；但 principal 再次将候选写为“卡顿的直接原因”和低优先级线程“持续持有 CPU”，与 typed `frame_causality=unproven / frame_evidence_status=absent` 冲突。`EVAL-B19-CAUSAL1/P1` 第三次复现，不再视为单次模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
