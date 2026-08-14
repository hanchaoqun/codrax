# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T01:54:42Z
- sweep_start_ts: 20260813-185440
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-185442 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 131s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B757 正证：Analyzer 前两稿跨字段冲突均 fail-loud，第三稿收敛为 bounded_effect_verdict；系统未补采/发布完整 Trace 因果投影。新 B758：Explorer 仍自行调用 root_cause_rank，Finalizer 又收到 Root-Cause Board、rank authority、rank Observation/Handoff，模型遂把 #1/修向/58.320ms 当有限问题结论，并把低于上限误写成“显著频率限制”；末尾却又承认 target binding 未证，内部矛盾。四态数值正确；65 次被唤醒也把状态/片段计数错当 wakeup census。Runner 正则不接受现有未证句式是次级 oracle gap，当前人工 fail 不能靠扩正则掩盖。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-185442 | answer_regex,answer_contains,mermaid_edge_count | none | 191s | 38 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | uncertain | B754 再获生产正证：第一稿不实的 Analyzer/Explorer/Extractor/Finalizer→BusContext/Mutable 边被精确拒绝；patch 按 typed recipe 保留 3 条 precedence + 1 条 call，并恢复 4 对 exact edge identity。BusContext/Mutable 缺 request-scoped incidence 证据，终稿以断开节点 + unproven boundary 诚实披露，关系未丢但用户所求完整数据流仍只能判 partial/uncertain，不能由系统造桥。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgement

- r462 证明 B757 已在生产链生效：有限 target-effect 不再因 Analyzer 冲突而被系统静默改造成 causal_diagnosis，且没有系统接管/改写模型答案。
- r462 同时确认 B758 是独立的 scope-aware context gap：系统虽然不 materialize 全量投影，却仍把探索期 rank board 作为高显著“权威顺序”喂给 Finalizer。软提示无法抵消同页相反的强上下文。
- 最优修向不是答案关键词门或删改正文，而是 typed runtime_question_profile 驱动的 prompt projection：有限/关系/overview 车道不注入 rank board、rank authority、rank observation/handoff；直接状态、频率、等待/关系（仅被对应 fact family 请求时）继续保留；causal_diagnosis 车道不变。
- 两案均无畸形 JSON 降级、旧稿恢复、空答案或 active-byte-stream 4ms 降级。
