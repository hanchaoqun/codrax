# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T05:23:32Z
- sweep_start_ts: 20260817-222332
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-222332 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 170s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 精确保留 1.000000..1.010000 用户窗、Trace 因果投影与自动补齐；worker-200 的 8.300ms 链上优先级反转候选为首席，app-100 的 10ms sleep 保持症状，邻近/背景未升主因，实际占用与规则可消除双轴均在。无固定 4ms 降级。正文/审计附录仍出现 coverage_status=complete、priority_inversion_runnable_wait 等 raw enum，属 B756 展示债，不改本轮数值与链上权限。 |
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260817-222332 | answer_regex | none | 356s | 38 | read=3,repo_map=2,list=1,trace=0,source_lens=1 | midloop=9,inv=2/0,fin_reject=7,unavail=0,prune=0 | pass | B1043 生产正证：首次 completion 被要求补 `_tokenize_slow` 函数体，Explorer 从已读 24–36 行重发实现证据后闭环；终稿不再误称实现缺失。B1044 生产正证：可见图含 `_fastlex.tokenize_bytes -> py.tokenize_bytes` 注册绑定边。7 次成文拒绝中有确定性合同自冲突：普通列表所需 edge_anchors 被 orphan normalizer 因混合 call/register 关系全删，随后 presence gate 又报 empty。最终补维度提示还把内部 role 以 `标签 (member_set)` 形交给模型，终稿逐字泄漏三枚举。两项分别登记 B1045/B1046，并按 typed carrier 根修；不归咎模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
