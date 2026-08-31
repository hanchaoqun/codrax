# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T20:04:11Z
- sweep_start_ts: 20260831-130410
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260831-130411 | primary_answer | none | 203s | 32 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=2,unavail=0,prune=0 | fail | 调用链与容量判断基本正确，且正文说明 `System.out.println` 输出到标准输出；但标题和主叙述仍沿用“审计落库”，没有明确纠正它不是数据库/持久化写入。runner 的语义边界 oracle 因此失败。先作为 typed terminal-effect 软引导观察，不从用户或答案词面增加硬门。 |
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260831-130411 | answer_regex | none | 207s | 32 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=4,unavail=0,prune=0 | fail | B1507 的分支事实已进入 finalizer typed handoff；本轮同时存在 `run` definition，故不是 call-row-only 生产正证。正文调用链正确，但首稿完整 Mermaid 在 fused-block 拆分后失去端点身份关联，被关系修补连续删除，最终只剩空 `sequenceDiagram`。这是系统关系修补退化，不是模型事实错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
