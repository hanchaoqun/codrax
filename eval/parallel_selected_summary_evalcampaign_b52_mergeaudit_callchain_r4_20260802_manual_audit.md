# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T08:14:12Z
- sweep_start_ts: 20260803-011410
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-011412 | answer_contains | none | 106s | 20 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 正确且完整列出 2 个 production caller；Build... 的两处 observation 归一为 line 294 主引用，Typed... 为 line 321。没有 1 项系统补充或 2/1 矛盾，B52c 生效。 |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-011412 | primary_answer | none | 359s | 24 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=4/0,fin_reject=12,unavail=0,prune=0 | fail | 真实 call 边均已证，但模型加入 `VisitService.schedule -> Reject` typed guard 分支后，call-DAG authority 仍把每条 flowchart 边当 invocation，连续拒绝 guard_condition，最终泄漏 21KB raw thinking。全语言混合调用/控制流图合同 gap，不是 Java endpoint 回退。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B52c-MEMBER-OBS-AXIS`：covered。
- 新增 `B52d-MIXED-CALL-DAG-RELATION`（P1）：call-DAG 的 body-edge call authority 必须尊重同
  `(from_node,to_node)` 的 typed `relation_kind`。`guard/import/precedence/observe/contain` 不能被二次
  解释成调用；无 typed anchor 的有向边仍按 call fail-closed，sequence invocation 也保持原红线。
- 该修复点位于语言无关的 AnswerDocument 图合同层，覆盖 Go、Java、ArkTS、Cangjie 等全部
  extractor 语言；不需要也不允许按语言关键词分支。
