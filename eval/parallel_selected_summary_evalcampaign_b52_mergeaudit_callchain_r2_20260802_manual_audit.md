# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T07:14:40Z
- sweep_start_ts: 20260803-001439
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-001440 | answer_contains | none | 117s | 20 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 完整核对 3043 行日志与 102 行答案；2 个 production caller、定义、调用点与结论一致，零 finalizer reject。一次 analyzer 把多成员关系写成单 function_name 后由软修复恢复，记为观察项，不新增硬门。 |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-001440 | primary_answer | none | 220s | 22 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=12,unavail=0,prune=0 | fail | 完整核对 2663 行日志与 238 行输出；reply `-->>` 已不再拒绝，但 12 次均为 call_edge_unproven。调用证据 Object 保留 service.schedule/config.resolveMaxVisits 等接收者表达，图端点为 VisitService.schedule/ClinicConfig.resolveMaxVisits，旧 unique-definition 补全臂只接受裸 operation，故仍不闭合。最终发出 rejected draft/raw thinking；另有 CREATE_VISIT、201 Created、stdout 落库等源码未证措辞，不能算正确交付。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

`bf6114879` 的 B52a 只覆盖“裸操作名 + 唯一定义”的窄形，r2 证明真实 extractor 还可能发射
`receiverExpression.operation`，因此状态从 pending 收窄为 partial。最优修点不在 Mermaid 模糊匹配：
repomap 应在静态类型可唯一确定时把调用关系接收者提升为声明类型，emit_evidence 再优先消费
Graph.ResolveCallTarget；动态语言或冲突绑定必须保留源码表达并 fail-closed。Java 是见证，不是修复边界。
