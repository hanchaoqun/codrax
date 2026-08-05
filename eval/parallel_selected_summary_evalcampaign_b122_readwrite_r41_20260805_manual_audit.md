# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T16:47:38Z
- sweep_start_ts: 20260805-094737
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260805-094738 | answer_regex,answer_contains | none | 288s | 23 | read=4,repo_map=1,list=1,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | Runner 的宽松字符串 oracle 命中，但答案把 `@register("json")` 错说成与 `content_type="application/json"` 关联，把 `resolve` 的 `cls()` 实例化错说成返回类引用，并把继承来的 `TimestampMixin.handle` 伪定位为 `JsonPlugin.handle`。动态注册/绑定链被强塞进纯静态 call endpoint 合同，造成 16 轮探索；final patch 的非空错误 citation quote 又绕过 quoteless-only 校验。 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260805-094738 | log_regex,write_apply,answer_regex,answer_contains | none | 1155s | 30 | read=12,repo_map=4,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 首次 patch 的五换行正例可通过，但把单个换行也折叠，缺少负向边界。更直接的失败是模型自写 probe 用 `(104,101)`（`he`）测试输入 `#el`，却期望 `[35,400,108]`；系统把这个无请求/测试依据的错误 comparator 整体标为 authoritative verification failure，replan 围绕伪 oracle 推理至 1155s 超时。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
