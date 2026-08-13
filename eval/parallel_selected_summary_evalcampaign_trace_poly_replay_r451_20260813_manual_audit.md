# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T19:19:56Z
- sweep_start_ts: 20260813-121955
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-121956 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 157s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail / causal role bypass | B735-3 重试提示本轮未触发：模型首个实际 emit 直接把“频率有没有受到限制”标成 role=other、scope=bounded，绕过 typed causal consistency。完整投影仍为 0。Explorer/终稿又把 CPU4 actual=floor 解释成“受限而非自愿节流”，把 direct policy row 的 ceiling-presence 权限与 binding-performance 权限混同；runner FAIL 是实质失败。B735-4 需用软分类流程规定先判每个 required dimension，causal verdict 不得以 other 兜底，混合请求由 causal breadth 主导。 |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-121956 | answer_regex | none | 166s | 24 | read=2,repo_map=1,list=1,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass / optional diagram omitted | 正文完整给出 Python guard、_fastlex、PyO3 wrapper、Rust 核心和纯 Python fallback 的有序关系，引用准确。模型本轮未选择可选图；请求没有强制图，因此属于允许的作者波动，不能由系统硬塞图。无成文拒绝、无 Mermaid 语法错误、无关系丢失。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
