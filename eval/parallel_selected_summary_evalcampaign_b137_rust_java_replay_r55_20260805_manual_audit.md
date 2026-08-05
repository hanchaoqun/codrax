# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T22:17:44Z
- sweep_start_ts: 20260805-151742
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_trait_impls | PASS | eval/results/sr_rust_trait_impls-20260805-151744 | answer_regex | none | 78s | 19 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Analyzer 正确铸 `predicate_axis=implement`；主清单严格只有 `LiteralMatcher`、`RegexLikeMatcher` 两项，分别给出 `--fixed` / 默认条件与真实 `impl Matcher for` 行。无 unavailable、completion/成文拒绝或系统重复成员表。末尾 owner 补充因正文提到但未引用 `src/main.rs` 而合法触发，但同一路径倾倒 8 个嵌套/修订锚点，确认 `EVAL-B137-OWNERSUPNOISE1`。 |
| 2 | sr_java_handler_impls | PASS | eval/results/sr_java_handler_impls-20260805-151744 | typed_inventory_rowset,answer_regex,answer_contains | none | 104s | 20 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | Analyzer 未再误发 `field_value_profile`；模型首轮正文自然形成 3 条 `实现类 → route → 文件` 行，relation pair 完成门与 authority 均闭合，Finalizer 零拒绝且没有系统重复 registration roster。Explorer 第一次 completion 漏逐成员 `support_refs` 后一次修复，属可恢复 carrier omission，继续观察而不增硬门。未引用的 `Handler.java` 被追加 2 个同路径 owner 行，纳入同一补充降噪修复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
