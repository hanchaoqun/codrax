# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T13:15:19Z
- sweep_start_ts: 20260814-061518
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-061520 | answer_regex,answer_contains | none | 223s | 30 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=1,unavail=0,prune=0 | partial | 最终正确给出 `buildAnalysisIR -> gate.RunWith <- gate.Run`，并明确两端无定向调用路径；图和中间函数清单均保留。Analyzer 本轮自行发出完整 source/sink，走既有 exact 归一化，未实际命中 B797 空 participant fallback，因此该项仍待同形生产正证。正文仍有 shared-callee 等内部术语；4 次 completion 与 1 次成文拒绝偏重。 |
| 2 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260814-061520 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 243s | 32 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 完整 Trace 因果投影和自动补齐均保留；链上主因是 app-20 runnable 5ms，rival/CPU pressure 仅作背景，Running/Runnable/S/D/IO 与 20 fragments/19 switches 基本齐。缺点是导语把 20 个片段写成 20 次切换、prio=53/RT 却称 CFS，并由 S/D/IO=0 过度排除全部 IO/锁等待。Analyzer 前两次因已被降为 optional 的图 participant 来源瑕疵而整轮拒绝，确认 B799；第三次 causal_diagnosis 与 fact_families 冲突的拒绝正确。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

1. `B797-REQUIREDRELATIONSCOPEENDPOINTPARITY1` 的代码修复没有回归，但 qf 本轮由模型直接补齐两个 endpoint，
   所以只能记为机器/人工结果转正，不能冒充空 participant 分支的生产闭环。
2. Trace 案例对用户关心的边界给出正证：显式时间窗的 root-cause 请求仍触发完整因果投影；系统补齐没有被
   有限事实分流吞掉；主因只从 typed on-chain 行选出，供给压力与 rival 仍是背景支持。
3. Trace Analyzer 前两轮的 sequence 图已经被 out-of-band authority 降为 optional，却仍因 participant 的
   `source_quote` 不逐字含 identity 而 hard reject。图不是用户必需输出，这类展示 carrier 不应压倒正确的
   runtime scope，也不应消耗整轮 JSON 重发。登记并修复
   `B799-OPTIONALDIAGRAMPROVENANCERETRY1/P1`：required 图保持严格；optional 图清空未锚定 relation scope、
   丢弃单个无效 participant 并留 warning，不改调查、答案、关系或结论。
4. Trace 第三次 reject 是精确 typed cross-field 矛盾：`causal_diagnosis` 不应同时携有限 `fact_families`；保留
   fail-loud，不以“减少重试”为由降低正确合同。
5. 两案均没有畸形 JSON 降级、空答案、旧稿恢复或 active-stream 的 4ms/固定总年龄降级。
