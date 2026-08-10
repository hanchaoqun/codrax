# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T19:26:47Z
- sweep_start_ts: 20260810-122646
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260810-122647 | answer_regex | none | 213s | 21 | read=8,repo_map=1,list=2,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | B472 的 guard-state 生产分支命中：finalizer 同时收到 True/False，摘要不再称“硬编码 True”，也不再称绕过 PyO3；但最终 ordered list 把 wrapper/core 引用对调，wrapper 还把源码 `Vec<u8>` 写成 `Vec<Vec<u8>>`，registration 行引用错到 `best_merge`。注册 subject 被接受为装饰形 `_fastlex (pymodule)`，exact registered-export join 因而诚实 fail-closed。runner regex 未覆盖逐成员角色—引用一致性。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260810-122647 | answer_regex,answer_contains,mermaid_edge_count | none | 776s | 38 | read=7,repo_map=4,list=0,trace=0,source_lens=1 | midloop=9,inv=4/0,fin_reject=13,unavail=0,prune=0 | fail | B470 enum/validator 已进生产并正确拒绝无 exact assignment tuple 的 conceptual data-flow；但所求六 participant 没有 source-derived data_flow recipe。删除未证边后，`participant_boundaries` 在 typed identity 与 diagram alias 两套解析间振荡：一形报 not-visible，另一形报 unknown/context-only，共 13 次 reject、两次 finalizer dispatch，最终 degraded 为零边图并保留未经证实的 prose 数据流及重复第一稿。确定性系统合同冲突，不是模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Runner=`1/2`，human=`0/2`。B470/B471/B472 均有生产消费见证，但都不能据此销账。
- `B472` 关闭了 r273 的两个原始语义错误；残余不是 PyO3 专名拟合问题，而是通用 registration endpoint 精度和 ordered member→citation 绑定问题。
- `B470` 的 typed relation 与 hard validator 行为正确；QF 仍缺 requested producer/transfer/consumer 的 exact assignment/return carrier，且 boundary alias 合同先把答案拖入降级。
- 新施工顺序：`B473/P0` 让 participant boundary 与 diagram 共用唯一 alias→typed identity 解析；`B474/P0` 精确核对 registration 两端；`B475/P1` 保持 typed member role 与逐项引用同源；随后再回放判断 `B476/P1` operation evidence supply 是否仍独立存在。
- 以上均不得从 participant roster/request/answer prose 造边，不得系统改写模型结论；Trace 显式窗、自动补齐、链上根因与背景分层未进入本次 source-diagram 车道。
