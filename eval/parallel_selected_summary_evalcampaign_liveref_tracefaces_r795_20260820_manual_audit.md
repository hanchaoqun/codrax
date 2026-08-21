# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T02:29:43Z
- sweep_start_ts: 20260820-192943
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-192943 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 331s | 41 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=3/1,fin_reject=1,unavail=0,prune=0 | partial | 指定 2.000..2.020s 窗、自动补采、四跳唤醒链、11.000ms 链上 IO 主席、三个独立 1.000ms 调度候选、实际占时/规则可消双账与 Trace 因果投影均保留，邻近/背景未加冕，跨 CPU 限定已进入可见答案。B1271 软边界仍不足：模型第四次仅凭 fscache_page_wait_on_page_bit 扩写为文件系统页缓存后端并建议 inode/page_cache；可见表还泄漏 blocked_reason_caller、kernel_callsite、waker_cpu/wakee_target_cpu。首次成文另因 candidate_role=thread 不在源码角色枚举而拒一次。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-192943 | answer_regex,answer_contains | none | 591s | 49 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=14,inv=3/0,fin_reject=13,unavail=0,prune=0 | partial | 最终四阶段、执行者边界、共享状态说明和三条 precedence 图均可读；但 13 次确定性成文拒绝不可接受。live failure_ref 已被模型采用且不再 unknown/stale，随后暴露两处系统自冲突：校验解析出的精确身份与 rejected anchor 身份不同，导致 ref 错报“无旧锚”；typed normalizer 补成 Analyzer→Explorer，而 lease 只允许 analyzer→explorer，导致同一 receipt 产物被租约反拒。模型在事务回滚后又误判旧 patch 已生效，级联改写整图。最终表头仍为“列2…列5”，B1263 开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
