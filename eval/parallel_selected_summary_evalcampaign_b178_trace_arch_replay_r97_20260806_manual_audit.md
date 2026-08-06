# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T12:44:13Z
- sweep_start_ts: 20260806-054411
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_architecture | PASS | eval/results/qf_architecture-20260806-054413 | answer_regex,answer_contains | none | 137s | 24 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 最终 7 阶段、条件前置/无条件主链、职责、权威文件和 Mermaid 均正确；首稿却把流程顺序边声明为 call，遭一次可避免的 hard reject 后才改为 precedence。prompt 同时给了正确 EDGE DECISION FIRST，却又把 architecture 的 component_relation/diagram_spine 证据形限定为 import/call/registration；canonical JSON schema 还遗漏合法 registration_edge enum，并漏写 edge_anchors[].claim_form 字段，属于系统教学自相矛盾。另有健康 quote hydration ×6 被页脚统称“系统降级披露”，沿用 QUOTEDEGRADE1 观察项。 |
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260806-054413 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 140s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、状态账、唤醒路径、根因排序、双轴与完整 Trace 因果投影都在；但模型第三次把 23.994ms 与 19.041ms 写成合计 43.035ms。finalizer 作答前已先看到 safe aggregation group：两席 overlap=95.156ms、independent=false、addition=forbidden、max-member-only、comparison=23.994ms。因此问题不再是上下文缺失，而是模型未遵从精确 typed 决策输入；禁止再堆提示、扫最终 prose 或由系统代写结论，应评估一次有界、模型所有的 typed-input 语义复核/修订。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B178-TRACEGROUP1`: precise context delivered, model adherence still failed across three replays; move from prompt shaping to a generic typed-input semantic-review design audit.
- `EVAL-B178-DIAGRAMTEACH1`: confirmed contract conflict. Keep call-edge evidence validation strict, but make the projected JSON shape and architecture relation choices describe the same accepted enum/field set.
- No evidence of regression in explicit-window Trace causal projection or deterministic supplementation.
