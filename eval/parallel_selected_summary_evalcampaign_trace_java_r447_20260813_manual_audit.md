# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T18:07:40Z
- sweep_start_ts: 20260813-110738
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260813-110740 | primary_answer | none | 120s | 24 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial / model variance | 首稿即通过成文且 Mermaid 合法；5 条 typed 调用边完整，capacity check 位置正确。最终正文明确写 `AuditLog.record` 调用 `System.out.println`，但又用“审计落库”概括，未复述 typed `effect_scope=exact_call_only` 的“stdout 非数据库/持久化”边界。finalizer 上下文已经含精确边界和禁止升级指导，未发现系统缺证或矛盾；本轮不增加正文关键词硬门、不由系统改写，按模型遵循波动留档。 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-110740 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 173s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail / system context gap | 显式 233.190ms 窗、157.248/5.604/70.338/0/0 状态分区、Trace 因果投影、链上根因和“实际占用/规则折算可消量”双轴均保留；但正文把超大核簇 thermal rail 2.34GHz 改称 policy ceiling，并与 CPU4 direct policy limit 2.10GHz 混叙。根因是引擎已有 `GovernanceCapClusterClass`，rich-note/投影链未携带，模型只见无身份的“该簇”。B734 补齐簇身份 carrier，并让 thermal/policy 的互斥机制边界与数值同一 typed 行发布；不扫描或改写模型正文。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
