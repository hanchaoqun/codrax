# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T00:22:38Z
- sweep_start_ts: 20260731-172237
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | PASS | eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-172238 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 181s | 47 | read=5,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | AK 生效：系统主值现完整发布 11 个 D-state occurrence、36.757ms、单一 dma_fence caller、231.794/233.190ms 与 1.396ms typed-unaccounted，显式窗因果投影/根因榜/唤醒链/可消除量均在。模型正文仍把 12 条 blocked_reason census 当主回答，并另报 8 次入口，且把窗内 tail-open 说成超出用户上界；同答三口径冲突。 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-172238 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 191s | 45 | read=3,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 系统 leading authority 正确声明 binder wait 仅为 >=1 次、1.409ms 的 capacity-truncated 下界，且 15 IPC requests 与 blocking occurrences 分量纲；模型正文仍写“只有/唯一一次”和“其余 4 个未产生等待”，越过 completeness。完整 Trace 因果投影无回退。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

1. AK1/AK2 已覆盖：同一 trace 的 artifact identity 能正确 join complete
   occurrence rowset，H2 的系统主值从 12 条 caller-linked record census 中
   分离出 11 个 scheduler-state occurrence；状态账同时披露 1.396ms
   unaccounted，未把 tail-open 重复相加。
2. 两例共同暴露成文时序 gap：完整 typed 主值在模型成文后才由系统 materialize
   到答案顶部。finalizer 的早期 Observation Ledger 虽含 blocking lower-bound，
   但缺少新 11-row summary 的紧凑末端 recap；长提示后的模型正文仍选择旧
   census/探索叙事。
3. 下一批应给 finalizer 增加 typed-only principal-value recap：只读取
   RequestModel、ObservationLedger、system supplement meta/results 与确定性
   authority builders；complete rowset 才授权 exact N/sum，capacity-truncated
   只授权 >=N/lower-bound。它是软成文指导，不读 raw request、thinking 或答案
   原文，不作 hard reject/删除/改写。
4. `4次(3.774~16.064ms)` 仍是四个 per-CPU aggregate group 被写成 occurrence
   次数的较低优先级债；当前系统主值已提供真实 11 次，后续另批增加 typed
   merge caliber 并把显示改成“4组 CPU 汇总”。
