# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T04:08:57Z
- sweep_start_ts: 20260815-210855
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260815-210857 | answer_regex | none | 119s | 25 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=8,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B875 保持生产闭环，Analyzer 1 次，native 与 fallback 均在。但终稿把 `core-rs/src/lib.rs:40` 的同名 PyO3 wrapper 写成 Rust core；真正 core 定义在 line 10，虽然 line 42 的 typed edge 已证明 wrapper 调用 core。它还把跨平台原生扩展写死为 `.so`。根因是同名 scoped endpoint 只有 call-edge object、没有把 exact definition identity/line 送入最终 path carrier；归入 B871b 的 operation-chain/endpoint-definition 缺口。可选图第一次因把模块/函数/核心强接成未证边而正确拒绝，patch 后拆成两个有证子图。
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260815-210857 | answer_regex,answer_contains | none | 167s | 33 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B877 的 P0 死循环已消失：无 `table/ordered_list` 三块误计、无 normalizer 删除/补回循环、无旧稿恢复，6 reject/5 patch 降为 1/1。残余 reject 来自 generic mechanism carrier 的 facet-aware 但仍全局 `ordered_list MaxCount=1`；两个独立子题各自合法携带 mechanism path，模型只得把第二个改成 bullet。更重要的是人工内容审计发现 Mermaid 部分答偏：Explorer 只读 `renderer.go` 的调用点与 runtime gate 注释，未打开 `mermaid_render.go` 的真实失败重写，终稿把提交前 off/soft/strict 校验策略当成 REPL 显示降级。配置部分正确；整体只能 partial。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
