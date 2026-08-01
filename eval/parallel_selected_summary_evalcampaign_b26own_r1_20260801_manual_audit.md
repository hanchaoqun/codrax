# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T22:26:11Z
- sweep_start_ts: 20260801-152610
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-152611 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 91s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | bounded fact 保持 1 个模型 summary；系统只追加完整 target-wait roster，无 Trace 因果投影、无模型替换。3 段/0.635ms 与 caller 清单正确。`sync_buffer_read_wi`/`sysmgr.elf` 的功能性解释略强于 trace 直接权限，但不影响主值。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-152611 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 142s | 41 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | OWN 修复通过：模型原始 6 blocks 全部保留，系统投影为 sibling，未再生成 replacement summary。新独立 GAP：同一附件被 `attached_trace.txt` 与原始 basename 分成两份 projection，整套系统报告重复；模型又把 RT 主线程的 runnable 延迟解释成“CFS 优先调度 CFS 任务”，并给出把中间节点移入 RT/priority_inheritance 的越权修复建议。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
