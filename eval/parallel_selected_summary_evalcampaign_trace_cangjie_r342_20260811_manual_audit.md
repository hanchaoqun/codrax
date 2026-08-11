# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T22:37:04Z
- sweep_start_ts: 20260811-153702
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260811-153704 | typed_inventory_rowset,dimension_substring,answer_contains | none | 103s | 24 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 2 个 extend、2 个 foreign func、8 个 public class 与 typed inventory 完全一致；每行均含真实文件/行号和 package 声明，未从目录猜 package，零成文拒绝。 |
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-153704 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 168s | 27 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 窗口、四态、VerifyClass 5.000/4.600ms、runnable 0.800ms 与两轴投影正确；但模型把唤醒前 sleep 误称 5ms wakeup 延迟，在 typed direct-block/completion/wait authority 均 absent 时宣称 app 被 worker 强制等待，并把低优先级关系直接定为优先级反转。另有系统确定性重复：同一 app sleep 物理段经 E1/E2 占两个自身席。上下文仅 27%，非预算不足；先复跑判断模型波动，禁止 prose 扫描硬门或系统改写结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
