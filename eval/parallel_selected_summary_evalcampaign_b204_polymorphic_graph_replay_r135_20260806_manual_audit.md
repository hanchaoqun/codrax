# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T23:08:11Z
- sweep_start_ts: 20260806-160809
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-160812 | answer_regex,answer_contains | none | 124s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 最终类、注册时机与 MRO 主链基本正确，但 `plugin.handle(payload)` 实际在 `pipeline/runner.py:17`，系统把这条已读行证据迁到无关 sibling call `run_batch -> run_pipeline` 的 21 行，最终错误引用 21；analyzer 还把带 discover endpoints 的明确调用链归为 mechanism 并丢弃 endpoint profile，S33 未被激活。 |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-160812 | answer_regex,answer_contains | none | 164s | 20 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | S33 readiness/首次 completion repair 已接线，但同锚点补发完整 subject 时因旧 carrier 带 condition、补发 carrier 省略 condition，答案级合并保留旧版本，selection 始终不可见并重复降级；相同 caveat 发布两次。两次 diagram reject 后删图，正文还把源码中的 stderr 写成 stdout。严格 JSON 恢复计数全为 0，失败属于 typed 语义/图层，不是畸形 JSON。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
