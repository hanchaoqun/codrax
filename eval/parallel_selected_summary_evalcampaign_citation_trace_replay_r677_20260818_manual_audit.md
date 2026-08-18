# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T09:39:57Z
- sweep_start_ts: 20260818-023956
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-023957 | answer_regex,answer_contains | none | 103s | 25 | read=3,repo_map=3,list=0,trace=0,source_lens=2 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 平台实现与 `cmd_sleep` 结论正确，但模型本轮没有输出带位置 cell 的结构化表格，B1062 新 binder 因输入形缺席而未获生产正证。模型仅提交 declaration、三个实现定义中的两个和 handler 定义共 4 个 citation；系统修复后 3 个平台 item 均无引用，最终只剩 `src/handlers.c:29`，第 32/34/38 行调用体也未覆盖。Typed handoff 已给完整四成员 row，但每个平台成员仍只有一个定义锚，缺少“分支定义 + 该分支实际 API 调用行”的组合支持权威，记 B1064，不能靠扫描答案文字或系统代写补齐。 |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260818-023957 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 159s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式主窗 5.000000..5.007000、2 次窗内查询和 1 个最终因果投影完整保留。目标四态对账为 running 1.200ms、runnable 0.800ms、sleep 5.000ms、D/IO 0；0.800ms runnable 作为链上调度供给可消席，worker-200 的 4.000ms running 与 5.000ms VerifyClass 同时保留为真实占时/确定性业务优化方向，邻近 sleep 与背景压力未晋升主因。终稿明确 frame/deadline 因果未证；无系统覆盖模型结论、无空答案、无固定 4ms 活跃流降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap disposition

- `B1062-MULTIOPERATIONCITATIONCOVERAGE1`: implementation remains `pending-production-replay`. This run did not contain an exact structured location cell, so absence of a binder event is not a negative result.
- `B1064-PRINCIPALMEMBERCOMPOSITESUPPORT1`: newly confirmed P1 design gap. A principal member can require several separately grounded implementation facts, while the current principal row exposes only one citation key/support anchor. The finalizer therefore receives a correct member slate but no authoritative member-local citation set for all visible subclaims. The repair must originate from typed evidence/support identities, not request or answer prose.
- `B1063-PATCHREPLACEMENTFIELDRETENTION1`: independently confirmed and fixed at the shared teaching source. `replace_blocks` is now explicitly described as a whole-block replacement whose omitted fields are deleted; patch apply semantics remain unchanged.
- Trace regression: explicit-window projection, deterministic auto-supplement, on-chain-only ranking, actual-occupancy versus rule-eliminable dual axes, semantic business clue, and frame-causality caveat all passed.
