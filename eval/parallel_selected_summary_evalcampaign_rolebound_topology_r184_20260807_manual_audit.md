# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T20:14:38Z
- sweep_start_ts: 20260807-131437
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260807-131438 | answer_regex,answer_contains | none | 158s | 21 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | S37an production positive：Analyzer 猜出的 `discover(source=run)` 被 typed provenance 归一为 `discover_path`，没有 required `main` 锚或 runtime-selection 补证；真实 `run`、精确 `@app/core -> packages/core/src/index.ts` 和可见 `dispatchOnce -> fetch` 均恢复，耗时由 r183 的 512s 降到 158s。仍有两类残余：Explorer 虽读到 `send` 第 23 行，却未发 `send -> dispatchOnce`，且 `client.fetchUser` 与 `ApiClient.fetchUser` 缺 receiver-binding，typed relation graph 仍是 3 components；模型在上下文明确披露断图后仍称完整链。正文还把 `status<500` 说成成功、把循环结束说成抛最后错误，源码实际是 4xx 立即返回且 `return lastError`。一个可选 Mermaid 因虚构调用/return 边被正确拒绝并删除；`blocks` 字符串 JSON 被系统恢复，答案未丢失。 |
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260807-131438 | answer_regex | none | 168s | 20 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 调用边、限定身份、引用和 Mermaid 均正确，Finalizer 零拒绝；但 S37ap 的 typed topology 限权句已逐字进入 Finalizer 上下文（明确 `fan_out_present` 不证明 parallel/concurrency/join），模型仍写“run 分叉出两条并行路径”。这是软上下文充分后的首次 production negative；不通过扫描“并行”正文、系统改写或新硬门追求本例答案，继续作为模型遵循度观察项。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
