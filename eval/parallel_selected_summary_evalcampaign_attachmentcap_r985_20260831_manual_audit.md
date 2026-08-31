# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T18:16:15Z
- sweep_start_ts: 20260831-111613
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-111615 | write_plan,write_patch_oracle | none | 57s | 27 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确生成 `Main.java:16 retrun -> return` 单行 patch 计划，未修改源码。planner 首稿无视已发布枚举，额外提交 `language=shell` 的 verification probe，被工具拒绝后删除；最终计划正确，但 runner 的 `fin_reject` 不统计该 write-plan 重试，记为独立软稳定性观察。 |
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-111615 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 197s | 44 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=3,inv=2/2,fin_reject=1,unavail=0,prune=0 | fail | B1502 生产正证：analyzer 的 unavailable reader 调用 1→0。显式 10ms 窗、链上根因、实际占时/规则可消双账户、VerifyClass 关系边界、背景隔离、自动补齐和最终 Trace 因果投影均在；但首稿把 principal-summary 专属 caliber 重复放入 section，仍触发一次真实拒绝，证明 B1501 不能按 r984 偶然零拒绝关单。可见摘要又把 NetworkService runnable 与 CookieMonster sleep 调度状态称为“确定性工作”，与同页 typed VerifyClass `relation_unproven` 结论冲突，B1499 类别边界再次复现，人工判 fail。`155` 条指的是唤醒事件的 `target_cpu` 字段全为 0，不是目标线程运行 CPU 记录；最终 caveat 的“目标 CPU 记录”措辞易误解，但其证据本身来自 typed integrity advisory。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
