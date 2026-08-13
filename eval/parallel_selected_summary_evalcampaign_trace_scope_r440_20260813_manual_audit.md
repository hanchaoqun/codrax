# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T15:46:20Z
- sweep_start_ts: 20260813-084619
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-084621 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 185s | 44 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B728 production-positive：六次 query 均显式携带同一 target/window；根因榜首为目标 running 65.912ms effective/74.915ms actual，模型与系统投影均保留实际占用、规则折算、业务 span、链上/邻近边界。无 4ms age 降级。 |
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results/real_trace_h2_dstate_dma_fence_triform-20260813-084621 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 209s | 47 | read=3,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | 旧 oracle 要求 full causal tree，与已裁 bounded_fact_set 窄答冲突；但正文也确有新 GAP：engine typed leaves 为 complete 11/11、36.757ms，普通 prompt preview 却为 incomplete 8/11、26.953ms，且 analyzer 漏发 target_wait_occurrences，导致 uncapped final recap/发布块未接通。模型把 4 个 per-CPU 候选、11 段状态区间、12 条 caller census/39.157ms 错拼为同一清单，并把 callsite 升级为 GPU/显示等待对象。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## r440 结论

- H7 人工通过，确认 B728 的 target-scoped family presence 已在真实链路生效；显式时间窗、因果投影与系统补齐未受影响。
- H2 runner FAIL 首先是历史 oracle 漂移：有限事实问题不再应被强制扩成全量因果树；oracle 已改为核对 11 段完整状态清单、12 条 caller census、36.757ms/39.157ms 两种独立口径及 typed 内核调用点，同时禁止 `Trace 因果投影`。
- H2 同时确认 B729，并非只属模型波动。底层 `target_window_states.wait_occurrence_status=complete`、`emitted=11/11`，11 条独立 typed leaves 均在；普通 observation 为节省 prompt 仅携带前 8 条并诚实标 `incomplete`。Analyzer 又漏发 `target_wait_occurrences` family，使 uncapped typed-leaf authority 不获准进入 final recap/答案块。模型因此把 preview 完整性误当 engine 完整性，并跨口径拼接。
- 修复只消费 schema-validated `fact_families`：显式 `target_wait_occurrences`，或精确合取 `target_scheduler_state + count_or_duration + (recorded_reason|occurrence_time)`，才授权完整 typed roster。`recorded_reason + count_or_duration` 另只授权 blocked_reason census 的独立事实块。它不扫描用户/答案关键词，不扩展 state-only/count-only/direct-waker 问题，也不发布全量因果报告。
- Final typed recap 新增 cap 分层说明：`incomplete 8/N` 只属于 compact preview；`exact_complete_rowset` 来自 uncapped leaves。bounded census 块把 caller 明确写成“内核调用点/符号”，同时披露它不能证明实际等待对象、资源持有者或子系统机理，且不能与状态段逐条配对。系统继续只提供事实与口径，不改写模型主结论。
- 两个 active stream 均持续 3 分钟以上并生成模型答案；没有因 4ms、4s 或固定总年龄降级，也没有 JSON 恢复/最终成文拒绝。
