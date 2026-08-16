# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T19:43:33Z
- sweep_start_ts: 20260816-124332
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_gson_lazy_number_symptom | FAIL | eval/results/github_issue_gson_lazy_number_symptom-20260816-124333 | write_apply,write_patch_oracle | none | 171s | 25 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | Patch 仅为 `LazilyParsedNumber` 增加基于原始字符串的 `equals/hashCode`，未改 Number 转换路径；静态检查通过。环境缺 Java runtime，唯一动态 probe 精确报告 `runner_missing`，因此保持 unverified 正确。B928 生产闭环：只有 `batch-1`，无 `batch-1-proof-repair`，也没有重复执行同一 probe；输出只保留一个局部“未完全验证”说明和一个不同职责的最终交付状态。Runner 将 unverified 计 FAIL 符合 oracle，不是代码失败。 |
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-124333 | answer_regex | none | 190s | 26 | read=4,repo_map=3,list=1,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | B927 软教学生产生效：模型首批主动发 registration，但粗写成 `_fastlex registers tokenize_bytes`；typed repair 后只重发 `m.add_function(wrap_pyfunction!(tokenize_bytes,m))` 精确行。最终文字覆盖入口、原生模块、Rust 核心及 fallback，引用有效。但 relation authority 仍只有三个断开的 verified components；终稿却通过 ordered-list `edge_anchors` 声称 `_fastlex.tokenize_bytes -> py.tokenize_bytes -> Rust tokenize_bytes -> best_merge`，其中这些跨分量边未出现在 typed edge recipe，且精确 registration 行未显示。新记 B929：非 Mermaid 的结构化列表关系没有复用统一 edge-authority 校验；B930：注册表达式与公开模块/导出 callable 的 typed identity bridge 本轮未铸成。不得让系统猜边，应从 parser-owned module declaration + exact registration tuple 建立可审计 non-call handoff，或诚实保留分量边界。Analyzer 本轮 requested_dimensions=0，故未生产触发 B926。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
