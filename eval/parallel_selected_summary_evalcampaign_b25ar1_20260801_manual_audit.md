# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T21:17:58Z
- sweep_start_ts: 20260801-141757
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260801-141758 | write_plan,write_patch_oracle | none | 98s | 20 | read=2,repo_map=0,list=2,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan-only；ChangePlan 只含 Main.java 的 retrun→return 单行 patch；Main.java 无工作树 diff，未进入 apply/verify |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-141758 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 204s | 43 | read=1,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 单一主要占用/因果投影/可消除量/代表窗/关键指标板；伪跨工件关系已消失；显式 114.940ms 窗、双维根因、唤醒链、frame unproven/absent 与系统自动补采均保留；全文 928 行 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
