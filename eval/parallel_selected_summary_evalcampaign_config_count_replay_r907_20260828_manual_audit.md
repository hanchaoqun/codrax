# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T22:10:16Z
- sweep_start_ts: 20260828-151015
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-151017 | answer_regex,answer_contains | none | 157s | 28 | read=15,repo_map=1,list=0,trace=0,source_lens=1 | midloop=9,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass | 答案正确给出代码默认值 50、`codrax.yaml` 的 `pipeline_max_steps`、CLI 显式覆盖以及最终 `SetMaxSteps` 消费链。r906 中同一补充备注多次重复的 B1412 症状消失，系统没有替模型改结论。过程仍有 15 次 read、18 个 explorer iteration；Decode/merge/Changed 的深层实现引用仍不充分，B1409/B1408/B1396 保持开放。 |
| 2 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260828-151017 | answer_regex,answer_contains | none | 450s | 36 | read=6,repo_map=6,list=0,trace=0,source_lens=6 | midloop=12,inv=4/1,fin_reject=3,unavail=0,prune=0 | partial | 事实表完整保留 3 个 type、5 个 function、30 个 Kind constant，共 38 行且逐行有引用；B1414 的完整 `replace_blocks` 教学生效，最终 table 携带 `facet_ids=[enumeration_item,member_set]` 和全部编译器行 ID。但终稿仍追加“部分项证据支持稍弱”的错误 caveat。日志显示 `members=30 principal_items=0 missing=30`：同名空 section 被旧标题匹配选中，实际 38 行共享表被排除。该问题确认为 B1415，不是成员或引用真的缺失。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- B1412 获得生产正证：重叠摘要 atom 已去重，配置答案没有重复补充块。
- B1414 获得部分生产正证：模型能按发布能力完整重发块并保留全部 38 行；首个 `source_inventory_family` 修补仍先尝试未发布的 field-edit，且总计 3 次 finalizer reject，过程优化继续观察。
- 新确认 `B1415-EXACTROWSETTITLESHADOW1/P1`：exhaustive review 以标题先选载体，在“同名空小节 + 多成员集共享表”形下会把真实行数判为 0，并发射虚假不完整 caveat。最优修复是优先按每行现有 `source_inventory_row_id` 的精确 set identity 计算对应集合；仅旧文档无 row ID 时回退标题匹配。禁止扫描请求、答案 prose 或渲染文本来消警。
