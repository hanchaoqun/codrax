# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T17:18:47Z
- sweep_start_ts: 20260802-101846
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_web_manual_summary | FAIL | eval/results/operation_web_manual_summary-20260802-101847 | log_regex,answer_regex | none | 112s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | command_rounds=5 | fail | 首页正文可见但 HTML `href` 被 material text 化过程删除；模型先用 BSD 不支持的 `grep -P`，后猜 `/man` 得 404，最终虚构 `/doc`、`/trace`。runner regex 还遗漏了答案里的“使用手册”短语。产品修点是给模型有界 typed link target inventory，而不是 URL 特判或自动替模型选择页面。 |
| 1 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260802-101847 | log_regex,answer_regex | none | 210s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=10,repair=5,failed_actions=5 | fail | 系统已有 u1/u3 两条 target contributions，模型后续也生成正确 `{"ids":["u1","u3"]}`；但 single-group count=2 被误铸为 final-answer expected/actual，正确 JSON 被 reconcile hard reject，终态回退发布 `2`。另有 phantom param `include=id` 被 executor 静默忽略。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings and decisions

1. `operation` 不是“模型看见链接却没选对”，而是系统上下文只保留 anchor 可见文本，删除了
   `href=./user_guide.html`。因此模型只能猜路径。修复在材料上下文层新增 source-ordered、去重、
   总量/长度有界的 `html_link_targets` 与独立 truncation 位；模型继续拥有相关性判断和命令规划权。
2. `data` 的正确答案曾真实执行出来，但系统把贡献域的单分组基数当成答案域权威。这是 typed
   authority 冲突，不是 JSON 文案波动。`reconcile_artifacts` 现在只证明 group aggregate，并显式
   发布 `answer_comparison_status=not_evaluated`；只有 `assemble_answer` 或显式 answer-scope 报告
   才能绑定最终答案。
3. `compute_contributions.params` 是开放 map，未知键此前静默无效。r3 的 `include=id` 让模型以为
   已配置 member list，executor 实际执行 count。该 action family 现对完整支持参数表 fail-loud，
   并给出 `include/set/rank + value_field` 与 count 语义的 typed repair；没有读取 purpose、用户原文
   或最终答案。
4. runner operation oracle 的 `用户(使用)?手册` 不覆盖普通“使用手册”，属于看护自身过硬；仅
   调整 eval 同义词集合，不改变产品门控。
