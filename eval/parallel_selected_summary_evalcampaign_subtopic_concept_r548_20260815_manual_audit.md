# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T03:45:15Z
- sweep_start_ts: 20260815-204513
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_loose_multi_question_units | FAIL | eval/results/read_combo_loose_multi_question_units-20260815-204515 | answer_regex,answer_contains | none | 155s | 29 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=6,unavail=2,prune=0 | fail | Analyzer 一次正确保留配置加载与 Mermaid 降级两个独立子题；首稿也同时包含配置优先级表与第二题说明。系统随后从两组 accepted principal aggregate facts 自动补出 2 个 `ordered_list`，但配置合同的 `table/ordered_list MaxCount=1` 只按 kind 全局计数，误把 1 个配置表和 2 个兄弟主题列表算成 3。模型连续 5 次删除列表，normalizer 又连续 5 次补回，形成确定性不可能修复环，最终以 `answer_document_retry_state_recovered` 降级发旧稿。根因为系统合同所有权自冲突，不是模型波动。 |
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260815-204515 | answer_regex | none | 168s | 25 | read=2,repo_map=2,list=0,trace=0,source_lens=2 | midloop=4,inv=2/0,fin_reject=0,unavail=1,prune=0 | pass | B875 生产正证：Analyzer 仅 1 次成功，不再因源码符号子题与 fallback 概念子题的 resolver hit/miss 不对称而重试。终稿完整保留 `FastTokenizer.tokenize → _fastlex.tokenize_bytes → PyO3 wrapper → Rust core`、`best_merge` 与 `_tokenize_slow` 回退，0 次 finalizer reject/patch；系统未代写关系或结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
