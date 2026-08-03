# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T07:57:50Z
- sweep_start_ts: 20260803-005748
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-005750 | answer_contains | none | 95s | 20 | read=1,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 模型正文正确列出 2 个 caller；系统把首项引用错配到第二项 line 321，并追加仅含 1 项的“清单完整性补充”，形成 2/1 矛盾。根因是 member_set 的 2 个实体与 3 个 call-site observations 未做轴归一。 |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-005750 | primary_answer | none | 111s | 20 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 全语言端点修复已生效：4 条限定调用边全部 exact，finalizer reject/patch 均为 0。答案仍把 side branch 当串行 hop，声称内存 append/println 是持久化，并有“5跳/6行”与冒号源码误写；属于模型对用户前提和调用图拓扑的解释质量，不是图硬门回退。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B52b-CROSS-LANGUAGE-CALL-IDENTITY` 已由同 pair 复放验证：Java 见证不再发生
  `call_edge_unproven`，且没有 rejected draft/thinking 泄漏。
- 新增 `B52c-MEMBER-OBS-AXIS`：系统必须在 typed/grounded 数据层把“答案成员轴”与
  “同一成员的多个观测点”分离，再向逐行引用合同提供一成员一主引用；不能截断数组，也不能
  在最终正文中猜修。
- Java 剩余错误先记为 soft quality residual；不得由系统替换模型结论。若跨用例复现，应以
  typed graph topology 和 source-kind authority 提供软引导，不能扫描问题/答案关键词做硬门。
