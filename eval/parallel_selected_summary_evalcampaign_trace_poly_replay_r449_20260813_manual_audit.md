# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T18:50:36Z
- sweep_start_ts: 20260813-115034
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-115036 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 115s | 39 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail / contradictory analyzer teaching | B736 生产闭环：完成门一次通过，不再要求源码 operation row，也没有把 trace 行重发给 emit_evidence。B735 第一层合同虽在场，Analyzer 仍把“有没有受到限制”铸成 bounded；日志证明同一 analysis prompt 的 runtime-scope 段教 yes/no→causal_attribution，后面的 requested-dimensions 段却仍只教 root-cause/ranking。模型遵循旧段不是波动。bounded 下 full causal supplement 仍未发布，最终虽看到 root_cause_rank 探索行却无 Trace 因果投影，并继续把 policy floor 误写成 governor 压制。B735-2 需把两段改成同一常量并钉 schema/prompt 两面。 |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-115036 | answer_regex | none | 142s | 24 | read=4,repo_map=3,list=0,trace=0,source_lens=1 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 模型首稿图包含多条无 metadata 的自绘边，validator 正确拒绝；第二轮直接使用系统提供的 typed skeleton，四条真实调用边与两个注释均保留，Mermaid 合法且正文完整。B733/B672 图自愈生产正证；系统只交付 typed skeleton，不替模型写结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
