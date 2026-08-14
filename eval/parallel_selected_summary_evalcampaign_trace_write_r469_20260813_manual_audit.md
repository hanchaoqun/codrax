# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T05:12:16Z
- sweep_start_ts: 20260813-221213
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_chrono_duration_min | FAIL | eval/results/github_issue_chrono_duration_min-20260813-221216 | write_apply,write_patch_oracle | none | 352s | 24 | read=13,repo_map=1,list=3,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail / honest unverified | B765 生产正证。模型在第二次静态 `make check` PASS 后请求 `finish/all_verified`，typed normalizer 把它精确降为 `accept_unverified/production_verification_source_static_only`；attempt/batch/final report 与用户终稿一致，没有恢复捷径假绿。补丁虽删除重复函数，但 Rust 未编译执行，`const fn` 中派生比较是否合法仍未证，故任务本身不能判 pass；runner FAIL 是诚实保护。 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-221216 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 376s | 45 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | B764 生产正证：终稿明确 running=157.248ms 是全窗全核，runnable=5.604ms 是离 CPU 就绪等待，不再相加成 162.852ms CPU 占用；CPU12/CPU4 与 policy-ceiling/target-binding 两轴均保留。FAIL 的 132.041 是模型把仅有的两个 target-owned top_running 桶相加后主动声明“非总量”，不再冒充完整 running，但暴露 B766：完整 trace/完整 running census 存在，系统仍未发布目标线程逐 CPU 的完整名册，只给全局 top-8 幸存桶，逼模型猜未展示核。另有 B767：有限效应判断的 Analyzer 在 bounded/full-diagnostic 组合间被拒六次，最终可满足但耗时 376s；应以 typed JSON 合同收敛修，不做 H4 关键词硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
