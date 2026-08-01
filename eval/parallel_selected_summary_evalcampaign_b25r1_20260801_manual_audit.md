# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T20:48:29Z
- sweep_start_ts: 20260801-134828
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260801-134829 | write_plan,write_patch_oracle | none | 54s | 19 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan-only；ChangePlan 仅含 Main.java 的 retrun→return 单行 patch，Main.java 无工作树 diff，未进入 apply/verify |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-134829 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 194s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 114.940ms 窗、主要占用、可消除量、根因排序、唤醒链、代表窗、因果投影和自动补采均完整；但同一附件被原始路径与内部 attached_trace.txt 识别为两份工件，整套 projection/evidence 重复发布，且伪造 cross-artifact relation=unproven |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
