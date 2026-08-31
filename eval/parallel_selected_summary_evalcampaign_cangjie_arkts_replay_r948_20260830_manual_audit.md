# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T02:53:00Z
- sweep_start_ts: 20260830-195258
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260830-195300 | typed_inventory_rowset,answer_contains | none | 190s | 28 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=0,inv=3/0,fin_reject=0,unavail=4,prune=0 | pass | 全量列出 4 个 `@Entry`（Index、ParentComponent、StyledPage、ListPage）和 2 个 `@Builder`（defaultHeader、GlobalCard），路径、行号和 fixture/production 边界准确；零 finalizer reject，未泄漏 synthetic collection label。Cangjie 行级 bucket 投影没有污染 ArkTS 多装饰器场景。 |
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260830-195300 | typed_inventory_rowset,dimension_substring,answer_contains | none | 262s | 28 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=4/0,fin_reject=0,unavail=7,prune=0 | pass-runner-false-red | 生产答案是一个读者友好的合并表，精确含 2 个 extend、2 个 foreign func、8 个 public class，12 个符号、文件坐标与 package 全部正确；零 finalizer reject，墙钟由 592s 降至 262s。旧 runner 把同一 12 行表分别作为三个 section 全量计数而报 12/12/12；离线用 `0f7dd92ae` 新 oracle 复验为空理由，属于评测器误报而非产品失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
