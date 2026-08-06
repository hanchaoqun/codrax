# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T19:22:26Z
- sweep_start_ts: 20260806-122225
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260806-122226 | dimension_substring,answer_contains | none | 66s | 20 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 完整列出 1 extend、1 foreign func、3 public class；每行 package/path/symbol 与 5 条 row-local citation 一一对应，无虚假 caveat。说明 source_inventory composite-row authority 正常，Rust 弱引用不是 Cangjie 图层缺失。 |
| 1 | sr_java_handler_impls | PASS | eval/results/sr_java_handler_impls-20260806-122226 | typed_inventory_rowset,answer_regex,answer_contains | none | 130s | 20 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 三个实现与三条 route 事实均正确，但系统越权改坏引用：模型 JSON 原本把每项绑定到 @Route 行 7/13/9；label-only definition fallback 把 citation_ref 改为 class 行 8/14/10、删掉原引用，再由错位生成 3 条 coverage advisory 和“证据支持稍弱”虚假 caveat。属于精确信号优先级反转，不是模型漏证据。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
