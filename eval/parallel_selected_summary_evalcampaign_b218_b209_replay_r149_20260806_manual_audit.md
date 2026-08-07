# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T05:32:09Z
- sweep_start_ts: 20260806-223207
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-223209 | answer_regex,answer_contains | none | 101s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B233 已在生产提示中发布 `Registry.resolve returns cls()` 与 `expression_form=call_result`，模型首轮正确判为实例且零成文拒绝；但系统随后把模型给 `@register("json")` 的精确 line 17 引用机械改绑到 class 定义 line 18，并丢弃原引用，违反证据修复单调性。另把模块加载期注册前置条件排列在运行时 resolve 之后，跨阶段表达仍不够清楚。 |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-223209 | answer_regex,answer_contains | none | 121s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 虚调用、`unique_ptr` 所有权、stderr 与最终 Mermaid 均正确；一次拒绝成功删除未证虚分派 call 边，sequence 展示参数未污染 endpoint identity。仍把 4 个 typed disconnected components 叙成工厂产物已注入 Logger 的完整链；样例没有 `Logger(make_sink(...))` 连接证据，现有 component soft note 未能支撑跨阶段结论边界。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
