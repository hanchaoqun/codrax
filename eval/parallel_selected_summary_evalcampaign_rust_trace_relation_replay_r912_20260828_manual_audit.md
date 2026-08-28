# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T23:36:59Z
- sweep_start_ts: 20260828-163658
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260828-163700 | answer_regex | none | 74s | 27 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | B1420 生产转正：唯一模型摘要始终在场。五条跨模块调用关系、精确方向、调用点与 walker 的递归文件收集/匹配前置角色完整；一次 reject 只因模型漏填 endpoint identity，局部 patch 复制 exact typed alternatives 后闭合。B1421 没有触发零-anchor 分支，但候选顺序已保持本轮关系上下文，没有主链被字母序截掉。无图是模型可选表达，不是关系丢失。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-163700 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 193s | 47 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 显式 114.940ms 窗、四跳唤醒链、链上优先级/调度/D-IO/算力/VerifyClass 线索、真实占时与规则可消双账户、邻近/背景隔离及完整 Trace 因果投影均在。新 P0/P1 合同冲突 B1422：引擎已精确识别窗内 1697 条 wakeup 的 target_cpu 全 0 为退化，但 pre-finalizer 仍给每条边发 `cpu_relation=cross_cpu` 权威，模型据此声称 36 次唤醒全部跨 CPU；答案末尾系统附注又说该口径不可靠。同页前后相反。另有 raw `causal_conclusion=unproven`/`frame_evidence_status=absent` 内部枚举泄漏，记 B1423 soft-language。没有固定 4ms/4m/活动流降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
