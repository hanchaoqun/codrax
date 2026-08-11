# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T12:39:36Z
- sweep_start_ts: 20260811-053935
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260811-053936 | answer_regex,answer_contains | none | 160s | 23 | read=9,repo_map=3,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 模型正常作答，但“完整调用链”只发布 run→fetchUser→send→dispatchOnce 三条 typed call；已读 transport.ts 中 dispatchOnce→fetch、send→nextDelay/sleep 未进入证据。答案把 sleep 定义行 :45 写成调用位置，并以不完整 typed 图宣称完整。B530。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-053936 | answer_regex,answer_contains,mermaid_edge_count | none | 243s | 38 | read=11,repo_map=6,list=0,trace=0,source_lens=1 | midloop=9,inv=2/0,fin_reject=3,unavail=0,prune=1 | fail | 模型正常作答，无四分钟降级。Analyzer 将用户要求连接的复合 `Mutable/BusContext` 错标 context_only；Explorer 因而只证明四阶段 precedence，未读真实 writer/reader，B529a 无触发机会；终图把 Mutable/BusContext 作为孤立节点。三次成文拒绝还包含一次漏 kind 和一次 stale boundary 修补。B529b。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- runner: 2/2；human: 0/2。两个答案均由模型正常产生，不是 JSON 恢复、流式超时或系统代答。
- B527b 没有在本案触发跨展示面删除；决定性问题是 Analyzer 自己把 `Mutable/BusContext` 合并成一项并标为 `context_only`，与其同轮 `predicate_axis=flow`、required diagram 及用户所求数据流相冲突。
- B529a 的精确分类修复没有生产触发：Explorer 只发了字段/类型定义，没有把已读范围推进到真实赋值、writer/reader 操作。该批保持正确但只能算 partial，需要 B529b 先守住 participant role，才能让既有 participant completion repair 驱动 operation-site 补读。
- TS 反例说明“member_set 叫完整”不等于 typed call graph 完整；定义行 support_ref 也不能替代调用点。B530 应从已读 parser/source relation 的遗漏发现入手，不得靠最终答案关键词或系统补写调用链。
