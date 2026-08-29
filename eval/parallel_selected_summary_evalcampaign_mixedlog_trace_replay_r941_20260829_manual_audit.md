# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T11:59:17Z
- sweep_start_ts: 20260829-045916
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260829-045917 | log_attachment,answer_contains | log_triage | 140s | 27 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 两语言第一帧、路径与各自调用者正确；内部外部源占位符和 JVM 臆造均消失。analyzer 仍用 4 次 emit 才从 root_cause/call_chain 收敛到有限枚举，确认场景与关系类型教学仍冲突。正文先声明两个错误栈是未证 peer，后又称 ArkTS 桥接层“收到仓颉异常后向外传播”，属于模型跨栈因果遵循漂移，不由系统改写或按正文关键词硬拒。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260829-045917 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗、4 跳唤醒链、链上优先级/调度/算力/D/IO、VerifyClass、实际占时与规则可消双轴、完整因果投影及自动补采均在；邻近/背景未进入主因。模型导语把 84.358ms 全部 sleep 概括为由四个排序方向共同解释，而 typed 投影只证明部分链上归因，记为总结口径过宽的软教学观察；无固定 4ms/4m 或活动流降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
