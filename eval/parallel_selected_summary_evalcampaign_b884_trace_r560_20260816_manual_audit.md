# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T09:55:16Z
- sweep_start_ts: 20260816-025515
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-025516 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 158s | 40 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | B888 生产生效：同窗 CPU4 的 target-running 与 policy 独立对齐，CPU12 不再借 CPU4 上限，四态 157.248/5.604/70.338/0 和 67.4% 正确，结论明确“policy 存在、目标绑定未证”；runner 只是限定词序的假 FAIL。仍不能签产品通过：Analyzer 无请求依据多发 recorded_reason，系统据此在模型正文后注入 50 条 blocked_reason 口径块；finalizer 还逐字泄漏 target_policy_binding 内部 enum。67 条 cpu_frequency 是该 exact event_search 的完整 census，不是 capped preview 误称。 |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-025516 | answer_regex,answer_contains | none | 509s | 37 | read=11,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=7/0,fin_reject=3,unavail=0,prune=0 | fail | 首轮读取已命中 FilterToolSchemas，B884c 使后续强制补读窗口推进，B887 的虚假 JSON-placement 前缀为 0；但 7 次 completion 降级中 3 次是同一 member_set 的“标识符:说明”无法与正确 positional support_ref 对齐。最终图只保留 Name()→工具 literal 的 return 边和无关 canonicalParameters call，真正的 finalizer full/patch 选择关系缺失；自动 PASS 只命中词面。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
