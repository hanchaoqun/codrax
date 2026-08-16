# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T10:17:45Z
- sweep_start_ts: 20260816-031744
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-031745 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 174s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | B890/B891 生效：Analyzer 只选 count_or_duration+target_scheduler_state+frequency_residency，非请求 recorded_reason 与 50 条 blocked_reason 系统块消失；正文不再泄漏 binding enum。四态账和按 CPU running roster 正确，但模型仍把明确属于 CPU12 的 2075MHz representative frequency 拿去与 CPU4 2100MHz policy 比较，并据此宣称 CPU4 “未触发限制”；正确边界只能是 CPU4 policy 存在、对 target 的 slice binding/impact 未证。另有 2 次结构表重试：四个 cells 中尾部空备注被 normalization 裁掉后又被校验器判成三列，属于系统 schema/validator 自冲突。Runner 仍是限定词序假阴性，但人工因跨 CPU 频点错配判 fail。 |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-031745 | answer_regex,answer_contains | none | 357s | 39 | read=7,repo_map=0,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=4,unavail=0,prune=0 | fail | B889 生效：completion 4 次调用零拒绝，`标识符:说明` 不再造成不可收敛 support-ref 循环。新瓶颈转到图层：4 次 finalizer reject 后，终图只保留 ParametersFor/canonicalParameters/patch Execute/Name return 等局部旁支，并把 finalizer 主关系诚实标为未证；用户要求的首次 full emit→失败后 retry patch 选择关系仍未形成。Analyzer 首次正确把 finalizer 作为 context_only，却因错误提示提供“改 incident_required”捷径而升格，且 runtime_selection_required=false，没有把 initial-vs-retry 工具选择作为选择门调查。自动 PASS 只命中词面与图块存在，不能收账。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
