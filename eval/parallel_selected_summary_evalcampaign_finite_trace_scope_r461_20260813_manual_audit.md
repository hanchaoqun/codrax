# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T01:31:14Z
- sweep_start_ts: 20260813-183113
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-183115 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 103s | 36 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 四态/运行时长正确，但 Analyzer 自相矛盾的 causal scope 被系统静默“修复”后触发全量补采与完整因果投影；模型又把 CPU0/CPU4 的直接 limit witness 写成 CPU12/CPU4 并交换 16/28 条计数。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-183115 | answer_regex,answer_contains,mermaid_edge_count | none | 198s | 37 | read=7,repo_map=3,list=0,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | uncertain (partial) | B754 正向生效：业务 node id 下补回 5+3 个 exact identity pair，data_flow 与 3 条 precedence/4 条 call 均保留；漏 kind 由 exact previous block id 安全继承。两轮拒绝来自模型先画未证边、后未把 unproven participant 画成可见断开节点，终稿关系完整但 BusContext/Mutable 只能显示未证边界。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Case 2 — finite Trace scope

- Analyzer emitted `scope=causal_diagnosis` together with three finite `fact_families`, while the frequency-constraint answer dimension remained
  `observed_value`; `intent=explain`, diagnostic booleans were all false, and only the broad `scenario=performance_bottleneck` label suggested
  diagnosis. These typed fields disagree about answer breadth.
- The pre-decode compatibility repair deleted `fact_families` and accepted the causal scope. This erased the precise conflict instead of asking the
  model to choose a coherent shape. The deterministic supplement then ran `root_cause_rank + critical_blocking_calls`, produced 326+38 extra
  observations, and the render layer materialized a 50+ item Trace causal projection for a three-part finite question.
- Finalizer context reached about 72k tokens. Its principal model block kept the correct 233.190ms partition
  (`157.248 + 5.604 + 70.338 + 0 + 0`) and the correct bounded binding conclusion, but miscopied exact limit witnesses:
  typed authority was CPU0=1.53GHz/16 rows and CPU4=2.10GHz/28 rows; prose claimed CPU12=1.53GHz/28 and CPU4=2.10GHz/16.
- Runner FAIL is narrower than the human failure: its regex did not accept “策略频率上限记录…没有独立…证明” as the existing-limit / target-binding-
  unproven pair. Do not solve only that wording oracle; fix the typed breadth conflict so the irrelevant root-cause report and context pressure disappear.
- General repair: non-bounded `fact_families` must fail loud rather than be discarded; `causal_diagnosis` must carry required
  `causal_attribution` plus a typed full-diagnosis carrier. `scenario=performance_bottleneck` alone is insufficient. A single finite effect verdict remains
  `bounded_effect_verdict`. The system does not infer or rewrite the model-owned scope from request/final prose.

## Case 1 — architecture relation graph

- The topology-based receipt repair restored five exact identity pairs on the initial business-labelled graph and three more on each patch graph.
  It preserved one data-flow edge, three precedence edges, and four independently cited call edges without inventing a bridge.
- Exact replacement-block kind inheritance fired once on the third attempt. Unknown ids, add blocks, explicit invalid kinds and non-exact ids were not
  involved; the production witness matches the narrow safety contract.
- First rejection correctly removed unsupported synthetic pipeline bridges and repeated one call row three times. Second rejection correctly required
  `BusContext` and `Mutable` unproven boundaries to be visible as disconnected participant nodes. The third answer passed with all verified edges.
- Remaining partial is evidence coverage, not a receipt-mapping failure: the requested high-level BusContext/Mutable incidence was not emitted as one
  request-scoped typed edge, so the validator honestly kept the two business participants disconnected. Do not manufacture that relation in the
  renderer; a later evidence-planning improvement may seek the exact field/assignment edge.

## Active-stream and authorship audit

- Both runs completed without malformed JSON fallback, prior-draft recovery, empty answer, or active-byte degradation.
- No 4ms total-age gate fired. Active byte delivery remains governed by caller cancellation/deadline, no-first-byte, byte-stall, transport or decode
  failure only.
- B757 changes Analyzer contract validation only. It neither rewrites the model answer nor changes Trace query values, projection ranking, or chain
  membership. A coherent `causal_diagnosis` retains full on-chain supplementation and projection; finite scopes remain narrow.
