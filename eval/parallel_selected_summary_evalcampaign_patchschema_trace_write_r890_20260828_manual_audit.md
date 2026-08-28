# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T15:25:33Z
- sweep_start_ts: 20260828-082531
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260828-082533 | write_apply,write_patch_oracle,answer_contains | none | 119s | 27 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确把 `retrun` 改为 `return`，仅改 `main.go`；应用 diff、changed-path coverage、真实 `go test -json ./...`、恢复引用和 clean worktree audit 全部闭环。Analyzer 首次误用未发布字段、planner 首次给出畸形 JSON，均收到精确可执行的结构化修向并在下一次成功；没有扩大改动或跳过写模式风险/验证门，记为过程噪声。 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-082533 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 215s | 41 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、四节点三边唤醒链、11ms 链上 IO 第一席、三个独立 1ms 优先级候选、实际占时/规则可消双账户、背景隔离、自动补齐与完整 `Trace 因果投影` 均在，且无定时降级。B1384 无自然坏字段触发，仅证明 no-regression。系统先以 probe 跑 4 个完整 Trace 视图，随后把三个子题 evidence 目标覆盖为同一句并再次跑相同四视图，确认 B1383 的 Trace 生产形；perf pre-stage 还把其局部工具清单误述为全局无 `trace_query`，形成新的上下文权限边界 gap。模型最终仍把调用点扩写为具体缓存机理，并从跨 CPU 唤醒推到迁移/亲和性下钻；typed caveat 没有支持这些确定性结论，保留为模型遵循观察项，不扫描或改写正文。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
