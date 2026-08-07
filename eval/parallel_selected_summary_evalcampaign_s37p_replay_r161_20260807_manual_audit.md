# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T11:39:24Z
- sweep_start_ts: 20260807-043923
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-043925 | answer_regex,answer_contains | none | 218s | 29 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=1,unavail=0,prune=0 | fail | S37p 的 reverse-edge 交接生产生效：Explorer 发出 `gate.Run -> RunWith @ gate.go:135`，最终答案保持真实方向并诚实披露 no-directed-path；finalizer 只因无 typed return edge 删除一条 reply，1 次 patch 后通过。仍未读取 `analyzerSymbolResolver` 函数体，漏掉 `analyzerGraphForNormalize`，所以 B262 未闭环。 |
| 2 | sr_rust_cross_module_chain | FAIL | eval/results/sr_rust_cross_module_chain-20260807-043925 | answer_regex | none | 254s | 24 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=3/0,fin_reject=7,unavail=0,prune=0 | fail | 调查证据与核心链正确：`main -> run -> collect_files/walk` 及 `run -> index_file -> is_match`。成文把节点显示为 `walker::collect_files`，而 typed inner caller 为 `collect_files`；validator 未消费已有 inbound `run -> walker::collect_files` 与唯一 source-local definition 来绑定两种身份，连续 7 次把真实 `collect_files -> walk` 拒为 unproven，最终降级。模型反复改 node id 无效，证明是显示限定名污染 typed identity，不是 Mermaid 语法或模型边方向错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap disposition

- `EVAL-B261-REVERSEEDGEHANDOFF1`: production-closed。真实 reverse call 已独立发证，最终方向正确。
- `EVAL-B262-TRANSITIVEHOPDEPTH1`: open/P1。软教学仍不足以让模型沿 `analyzerSymbolResolver` 进入下一层；不得继续按 helper 名堆 prompt，后续评估 typed frontier/context 方案。
- `EVAL-B264-DISPLAYQUALCALLER1`: confirmed/P0-P1。严格关系门把 reader-facing `module::function` 与 parser-owned short caller 当作不同 identity，尽管 evidence pool 已有精确 inbound qualified endpoint、唯一 source-local definition 与 inner direct call。应在这三个 typed 条件唯一汇合时绑定显示别名；多定义、跨源同名、冲突 owner、缺 inbound 均 fail-closed。
- `sequence-display-parameter-identity`: regression pin green，但本轮仍无带参数 sequence message 的生产 witness，保持 open/production-unverified。
- `all-language-flowchart-relation-anchor`: 本轮是 labelled Rust call 的 false reject，不是“无标签逻辑箭头绕过 strict anchor”的开放项，后者继续 open。
