# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T09:19:54Z
- sweep_start_ts: 20260831-021952
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-021954 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 278s | 45 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail (model semantic overclaim; system structure pass) | 显式 10ms 窗、目标四态、typed 唤醒链、真实占时/规则可消双账户、链上排序、业务线索、背景隔离、自动补齐与 Trace 因果投影均完整。模型却在一处同时写“NetworkService 被 CookieMonster 唤醒”和“NetworkService 唤醒 CookieMonster”，并把 typed 边界明确限定为候选的优先级现象写成直接阻塞/锁方向；typed 上下文已明确关系方向与未证边界，不新增答案词面硬门或系统改写。 |
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260831-021954 | typed_inventory_rowset,dimension_substring,answer_contains | none | 507s | 34 | read=14,repo_map=2,list=0,trace=0,source_lens=2 | midloop=7,inv=5/2,fin_reject=4,unavail=9,prune=0 | fail (system contradictory authority) | 首次 finalizer 草稿已精确给出 2/2/8 共 12 行及文件/package。预发射却从并行 explorer 的陈旧 raw member_set 再要求 `extend String/extend Cart` 及错误 `Container/Vehicle/Machine`，经过 4 次拒绝、5 次 patch 变成 17 行。finalizer 教学使用 canonical typed roster，硬校验仍使用 raw merged roster，属于同轮双权威，不是模型波动。B1483 统一为同一 typed 投影。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
