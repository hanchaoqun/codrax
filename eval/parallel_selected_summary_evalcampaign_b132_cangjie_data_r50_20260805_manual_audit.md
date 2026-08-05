# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T21:17:23Z
- sweep_start_ts: 20260805-141721
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260805-141723 | dimension_substring,answer_contains | none | 77s | 19 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | typed source_inventory 给出 1 个 extend、1 个 foreign func、3 个 public class，答案 5/5 完整；package 分别保留 demo.cart/demo.bridge/demo.app，来自 Cangjie parser 的 package 声明而非路径猜测。残余是 5 个合法空 quote 被自动填充后仍显示“系统降级披露”，正文与事实未受影响。 |
| 2 | data_join_entity_reconcile | PASS | eval/results/data_join_entity_reconcile-20260805-141723 | log_regex,answer_regex | none | 163s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格输出 `30`；3 个材料完整消费、2 条 Alpha contribution、5 条 decision、rule coverage=1、reconcile=pass。无 JSON/参数 repair，但模型两次把本批前序 action 输出当同一 actions[] 后序输入，系统拆成 8 批并把原候选记 failed；结果可靠、过程合同含糊且低效。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
