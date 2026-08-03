# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T08:29:50Z
- sweep_start_ts: 20260803-012948
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-012950 | primary_answer | none | 111s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B52d 机制验收通过：mixed call-DAG 含 3 条 typed call 与 1 条 guard，零 reject/patch；但正文把 3 条 call edge 称为 4 hops，误写 config.getMaxVisits（源码为 resolveMaxVisits），并把 AuditLog.record 的 println 称为落库且引用停在调用点，故整答人工判 FAIL。 |
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-012950 | answer_contains | none | 169s | 21 | read=5,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 直接 caller 主集 2 项及 line 294/321 正确；第二轮另发 omitted-role 的间接 upstream member_set，旧默认把它也铸成 principal，finalizer 与两个系统 carrier 重复发布并扩大 direct called-by 边界，登记 B52e。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

- B52d 的关系图机制已经闭环：Java 图中的 guard 边不再被 call authority 误拒，三条真正调用边仍需 typed call evidence。Java 的剩余错误属于模型对现有精确证据的解释波动；系统不改写正文、不按词面加硬门。
- called-by 暴露的是确定性结构化接线 gap：`system:typed_relation_principal_member_set` 已给精确 direct roster 确权，但同一 completion 中另一个未显式声明 role 的辅助 member-set 仍按默认规则取得第二 principal 席，进而触发两套系统清单。修复应在 aggregate role/provenance 层完成，不删除模型辅助分析。
