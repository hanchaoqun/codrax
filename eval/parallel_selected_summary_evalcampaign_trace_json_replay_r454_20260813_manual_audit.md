# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T20:15:54Z
- sweep_start_ts: 20260813-131553
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-131554 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 186s | 39 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail / passive-binding classification gap | B738 获生产正证：三个 required dimension 即使存在中英混排空白差异也全部保留，不再出现 provenance 丢维度。但 Analyzer 仍把“目标 CPU 频率有没有受到限制及证据”标成 `evidence_source + bounded_fact_set`，未运行 root/wakeup view，投影为 0。现有 SSOT 只清楚教 active `condition constrained outcome`，没有覆盖 passive `target was constrained by condition`，是 B739。Finalizer 已收到 B737 的 `policy_limit_status=present / target_binding_status=unproven` 和 supply-fold 独立边界；模型仍把 floor presence 升级成“线程被限制在最低档”，是证据服从性错误，系统未替换结论。 |
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260813-131554 | log_regex,answer_regex | none | 335s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass / workflow-efficiency gap | 最终严格 JSON 为 `{"ids":["u1","u3"]}`，材料、规则、贡献和 reconcile 均闭环；没有畸形 JSON 恢复或答案降级。但 8 轮/3 修复/335s 暴露 B740：先漏 instructions.md 消费，材料已覆盖后又重做 `extract_records`，随后发出 schema 不支持的 `derive_fields.operation=preserve` 并重复陈腐 deferred edge。应从 typed action schema/next-stage capsule 降低心智，不把 JSON 正确结果误判为无 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
