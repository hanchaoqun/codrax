# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T19:05:27Z
- sweep_start_ts: 20260807-120525
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260807-120527 | answer_regex | none | 149s | 21 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | 文字和引用正确；copy-ready/最终 Mermaid 仍把 `walker::collect_files` 与 `collect_files` 拆成两个节点。模型尝试补一条无证据桥后被正确拒绝，patch 最终复制了系统断图。定位为 auto-pair doc-comment 说明行误入 declaration census，已驱动 S37am 修正；本次不是 positive replay。 |
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-120527 | answer_regex,answer_contains | none | 300s | 24 | read=4,repo_map=3,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 两阶段边界、字符串选择、构造注入、虚派发和 stderr 结论基本正确，但无图，2 个条目引用被移除并出现泛化 coverage caveat。300s 主要是 Analyzer 6 轮 `call_chain_endpoints`/sink mode 纠偏与模型延迟，Explorer 另有 1 次 completion repair；不是 Finalizer 重试。无图意味着本轮没有覆盖 C++ 无标签 flowchart 箭头绕过观察项。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

- Runner 的 `PASS` 只覆盖声明 oracle；人工审计两案均为 partial，不能据此关闭图层完整性或引用完整性问题。
- Rust 暴露的是 typed producer role 与 declaration identity 的系统缝，不是 Rust/`::` 特例；修复须继续使用共享、跨语言、fail-closed resolver。
- C++ 的长耗时发生在分析分类阶段：用户提到“console 后端”但未逐字给出 `ConsoleSink` 类型，模型在 `discover + non-empty sink` 与 `exact + pre-scan candidate` 间反复。当前 contract 和错误提示本身明确，先登记为异构观察项，不为该样例放松精确端点门。
- 本批未触碰 JSON 教学或 Trace 实现；显式时间窗、Trace 因果投影、系统补齐、根因排序、唤醒链、窗内可消除量与双维根因能力不受本批影响。
