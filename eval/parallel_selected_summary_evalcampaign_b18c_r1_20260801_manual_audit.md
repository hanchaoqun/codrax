# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T04:42:45Z
- sweep_start_ts: 20260731-214243
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260731-214245 | answer_regex,answer_contains | none | 160s | 22 | read=14,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=3/0,fin_reject=0,unavail=0,prune=0 | pass | 12 个 production 实现只出现于一个主表和 Mermaid；没有 accepted-checklist 重复清单，3 个 test 实现继续作为排除范围披露。typed roster 保持 12+3。 |
| 1 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260731-214245 | answer_contains | none | 212s | 20 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 模型主列表正确列出 2 个调用者函数；系统又发布 3 个调用点并标成“生产代码函数（3）”，同一 BuildTypedRelationQueryWithResolvedSources 因 line 290/291 重复。typed graph 清册实际为 2 个 caller member。根因是 completion 把 observation/call-site 轴误铸为 relation member 轴，另立 EVAL-B18-AXIS1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### implements

- 最终 `emit_answer_document` 为模型主表、summary、diagram；归一化后仅再加
  通用 caveat，未出现 aggregate member supplement；
- 表格与图均为 12 个 production 类型，测试类型没有进入 principal；
- 证明 B18c 对“同一结构化主键列表已经完整”的去重有效。

### called-by

- typed graph 明确给出
  `called-by appendTypedRelationKinds -> 2 member(s)`：
  `BuildTypedRelationQueryWithResolvedSources` 与
  `TypedRelationKindsForRequest`；
- explorer 的 `member_set` 却包含 3 个 observation rows：
  前一函数 line 290、前一函数 line 291、后一函数 line 317；
- completion 把该 fact 标成 typed relation authority，最终系统清单按
  `value=3` 忠实发布；模型按用户所问的 function axis 列出 2 项，答案因而
  同时声称“2 个函数”和“3 个函数”；
- B18c 不能在显示层吞掉这份集合，因为它与模型 2 项列表并非同一 typed
  member-set。需要在 completion authority 铸造时按 exact candidate identity
  规范化：同一 caller 的多个 call-site 留在证据明细，不重复成为成员；
- analyzer 的 files-only prescan 一度把“只有定义文件命中”叙述成零调用者，
  但 typed graph 和源码读取随后纠正，未污染最终主结论。按模型波动/软引导
  观察，不为它增加关键词硬门。
