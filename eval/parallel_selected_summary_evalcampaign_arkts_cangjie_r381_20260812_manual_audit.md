# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T11:03:26Z
- sweep_start_ts: 20260812-040324
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260812-040326 | typed_inventory_rowset,dimension_substring,answer_contains | none | 116s | 24 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 2 个 extend、2 个同名 foreign func、8 个 public class 全部一次成文通过；两个 `native_add` 由 package 括注和不同精确 row_id 区分，`Cart` 的 extend/class 两个位置也未合并。每条可见 bullet 均带文件:行引用及 package，答案直接、无图、无系统结论代写。模型未在 item text 重复文件路径，但 renderer 把精确 citation 放在同一条可见行，用户要求的路径维度实际可见；不为重复展示增加 prose 硬门。 |
| 1 | arkts_repomap | PASS | eval/results/arkts_repomap-20260812-040326 | typed_inventory_rowset,answer_contains | none | 127s | 24 | read=5,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 4 个 Entry 与 2 个 Builder 均保留精确 row_id，reader-facing `Index` 等基名不再触发 row-id 拒绝；每条正文显式含完整文件路径、行号和业务说明。唯一拒绝是模型从 `@Entry` 标题猜出未在 typed rows 暴露的 `source_inventory_family=@entry`，现有精确提示要求省略，第二轮局部 patch 成功；与 B635 无关，单次可行动偏差，不立新硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
