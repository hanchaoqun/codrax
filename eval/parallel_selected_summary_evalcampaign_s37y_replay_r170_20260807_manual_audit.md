# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T15:06:03Z
- sweep_start_ts: 20260807-080602
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-080603 | answer_regex,answer_contains | none | 164s | 25 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=0,unavail=0,prune=0 | partial | S37y 目标问题已转正：最终 prose 与 Mermaid 都把 `buildAnalysisIR -> gate.RunWith <- gate.Run` 表达为双入汇合，明确无 source→sink path；finalizer 仅一轮、零拒绝/patch，typed edge labels 直接承载 exact endpoints。runner FAIL 是关键阶段列表只含 `analyzerGraphForNormalize`/`analyzerRequiredFiles`，未覆盖任一中段 normalize/compile/plan/bind 阶段。根因是 235-call frontier 按关系序号分位取样，被前段调用密度挤占，并非方向合同回归。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260807-080603 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 182s | 41 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、5 次 typed trace_query、唤醒链、根因排序、实际占用/规则内可消除双轴和系统补齐均保留，零成文拒绝；但模型把 `frame_evidence_status=absent` 直接写成“未发生丢帧”，这是错误的 outcome 确权：absent 只表示未产出目标绑定 frame/deadline 证据，既不证明丢帧也不证明未丢帧。模型还再次把 priority-inversion candidate 扩写成资源持有/锁释放，尽管 typed holder/waiter authority 明确缺失；后者按重复模型波动留档，不加 prose 硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## r170 审计结论

- `EVAL-B277/B278` 获得正向生产结果：QF 双入拓扑不再被反向串联，typed single-edge item labels 不再触发冗余 exact-anchor repair，成文从 r169 的 2 rejects/2 patches 降为 0/0；模型答案所有权未变。
- QF 剩余 partial 不是“固定要求某个 helper”。真实 235-call source 的当前抽样按 relation ordinal 分位，前半段调用密集，导致源代码中部稀疏但重要的 phase 区间无法进入 24-row frontier。泛化修法应改成 source-line coordinate 均匀覆盖，同时保留 typed sink、有限 head/tail；不得写入 normalizer/compiler 名单或 runner regex。
- Trace 自动 PASS 是 false positive。正文具备用户要求的两个根因维度，并保留显式窗、因果投影、自动补采、根因排序、唤醒链和窗内可消除量；但 `absent -> 未发生丢帧` 违反证据口径。下一批从 typed status enum 单源发布明确语义，作为软上下文；不扫描模型答案、不替换结论、不新增硬拒。
- priority-inversion candidate 被扩写成“持有资源/等待释放”已是重复高影响模型波动，但现有 finalizer context 已精确给出 `holder_waiter_authority=not_provided`、`synchronous_blocker_authority=not_provided` 和 candidate-only 口径。本轮继续留档观察，避免靠关键词硬门或系统代写修成特例。
- JSON schema/教学未触发、未改动。两条开放守护 `sequence-display-parameter-identity` 与 `all-language-flowchart-relation-anchor` 继续开放。
