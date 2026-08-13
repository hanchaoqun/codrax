# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T18:26:20Z
- sweep_start_ts: 20260813-112618
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-112620 | answer_regex | none | 139s | 24 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial / honest boundary | B733 获生产正证：`py.tokenize_bytes -> tokenize_bytes` 即使显示 `(Rust)` 也未再被 endpoint identity 污染。模型另画 `_fastlex.tokenize_bytes -> py::tokenize_bytes`，但 typed evidence 只有 registration/binding、没有执行 call；validator 正确只拒该伪边。模型随后删除 optional 图，而正文诚实区分“注册边界≠执行桥”。保图仍属软 authoring 遵循，不由系统生成边或图。 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-112620 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 158s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail / analysis-contract gap | Analyzer 首次正确发出 `causal_diagnosis`，但把“CPU 频率有没有受到限制”只标为 `evidence_source`；旧合同不承认 explain/yes-no 的因果判定形并拒绝，模型按提示退成 `bounded_fact_set`。系统遂只补 `window_stats`，整份答案合法地丢失 Trace 因果投影与 B734 carrier，并把 actual=policy floor 误写成受限。另因 invalid exclusion 被 fail-open 为 allow，通用 flow 完成门无视 route 的 current_source=optional，要求补源码 operation row，与“trace_query 不需 emit_evidence”教学直接冲突。B735/B736 均为 typed 合同根修，不是模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
