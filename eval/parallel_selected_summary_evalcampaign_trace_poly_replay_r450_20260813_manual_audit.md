# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T19:02:54Z
- sweep_start_ts: 20260813-120253
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-120254 | answer_regex | none | 130s | 24 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 首稿图把 PyO3 注册/包装误画成两条 invocation edge，typed validator 正确拒绝；第二轮消费 copy-ready skeleton，仅保留三条已证调用边与两个业务注释，Mermaid 合法，正文与 fallback 完整。系统提供关系载体但没有代写结论。 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-120254 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 183s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail / typed retry self-defeat | Analyzer 首发 required causal_attribution 是正确方向，但把 scope 同时填 bounded 后，重试提示只说改 causal_diagnosis；模型将该枚举理解为预设“完整根因定谳”，于是主动删掉用户的因果维度并改成 evidence_source。accepted bounded 合法关闭完整因果投影（final_projection=0）。终稿虽说“上限存在不等于线程受限”，却虚构 CPU12 也有 direct limit witness，并把 CPU4 最低频点混成命中上限；因此不是 oracle 假阴，而是实质失败。B735-3 应明确 causal_diagnosis 只是调查/答案宽度，允许 yes/no/mixed/unproven，冲突重试必须保留 required causal dimension。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
