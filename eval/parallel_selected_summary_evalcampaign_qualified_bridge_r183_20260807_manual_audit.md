# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T19:21:41Z
- sweep_start_ts: 20260807-122140
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260807-122141 | answer_regex | none | 137s | 21 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=3/0,fin_reject=0,unavail=0,prune=0 | partial | S37am production positive：copy-ready 与最终 Mermaid 只有一个 `walker::collect_files` 节点，真实 `run -> walker::collect_files -> walk` 连通，Finalizer 零拒绝，B299 关闭。但正文再次把源码中的“先收集、再逐文件匹配”说成“两条并行调用路径/在 run 汇聚”；调用图 fan-out 不证明并发，B268 获得第二个 production witness。 |
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260807-122141 | answer_regex,answer_contains | none | 512s | 27 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=7/0,fin_reject=3,unavail=0,prune=0 | fail | runner oracle 假绿。声称“完整链”却停在 `HttpTransport.send`，漏掉已读源码中的 `send -> dispatchOnce -> fetch`；把精确 `@app/core` 映射误写为并不存在的 `@app/core/*` 通配符；系统因 Analyzer 猜出的 `main` discover source 强制结构锚，最终把真实 `run` 行标签改成 `main`。Analyzer 10 轮、Explorer 20 轮、Finalizer 4 轮/3 reject；discover-sink runtime-selection 与 member_set form 在已有证据下反复重开，最后才 low-delta 收敛并删图。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

- B299 获得第二次 production replay 的正见证并关闭：共享 resolver 对 Rust 限定身份、doc-comment companion 与真实 definition 的分层生效，且不是按语言或 `::` 特判。
- Rust 的“并行”是 B268 重复 witness。修复只能给 Finalizer 增加 typed topology 的软语义：fan-out/disconnected/component 不证明 concurrency/order/convergence；不得扫描或改写模型正文。
- TS 暴露 endpoint carrier 的结构缺口：当前 profile 只能表达“两个精确代码身份”或“精确 source + 待发现 runtime sink”，无法表达“用户只给角色边界，路径两端都由源码发现”。Analyzer 于是只能猜 `main`，而 discover source 又没有 current-request provenance 校验，猜测最终进入 required-anchor 硬门。
- 同一个 discover-sink selection lane 已 low-delta 收敛并留下 typed caveat 后，后续 member_set form 失败会再次打开该 lane。收敛状态没有成为同次 completion 链的单调 authority，造成重复补证。
- TS 已读取 `transport.ts` 并在 thinking 中识别 `dispatchOnce`，但没有为 `dispatchOnce -> fetch` 发 grounded call-edge；这是上下文/交接不足，先用角色端点新车道和通用 call-edge 软提示修复，不把 `dispatchOnce`、`fetch` 或 TypeScript 字样写进硬规则。
- JSON/schema 仍保持单源：新增能力应扩展既有 endpoint enum，不再增加一份自然语言业务 JSON 合同。Trace 实现、显式窗、因果投影、自动补齐、根因排序、唤醒链、窗内可消除量和双维根因不在本批改动面。
