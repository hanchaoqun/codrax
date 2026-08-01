# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T10:19:44Z
- sweep_start_ts: 20260801-031942
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-031944 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 115s | 38 | read=3,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | `runtime_question_profile=bounded_fact_set` 成功压过 `root_cause/diagnostic` legacy 噪声；主答案完整列出 3 段与 0.635ms，且无 Trace 因果投影/根因榜/背景大盘。仍有两个非阻断残余：analyzer 将明确 PID 误报为 `no_named_target`；窄答案末尾仍收到未执行钻取/未定位唤醒者的通用 caveat，均登记到统一台账。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-031944 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 45 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗的两轴占用、可消除量、根因排序、唤醒链、投影与自动补齐全部保留；但 principal 第四次把 `causal_conclusion=unproven / frame_evidence_status=absent` 写成“直接根因”，并把低优先级唤醒者本身当成反转原因。analyzer 还把明确时间窗错报为 `full_artifact`，虽本轮模型查询恰好使用正确窗，typed scope authority 仍不可靠。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
