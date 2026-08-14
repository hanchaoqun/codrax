# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T10:32:58Z
- sweep_start_ts: 20260814-033257
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260814-033258 | answer_contains | none | 104s | 26 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 正确枚举 2 个生产 caller、3 个调用点，测试文件未混入；图中 3 条边均为同向直接调用。Analyzer files-only 预判一度错误地声称无生产 caller，但 Explorer 读源后纠正，未污染终稿。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-033258 | answer_regex,answer_contains | none | 246s | 31 | read=6,repo_map=2,list=1,trace=0,source_lens=1 | midloop=6,inv=4/0,fin_reject=1,unavail=0,prune=0 | partial | B785 修复生效，零 panic 且有完整答案；终稿正确识别不存在 `buildAnalysisIR -> gate.Run` 有向路径、两条可证边方向正确、Mermaid 合法。但系统自身一面警告 sibling 不能拼链，一面又发布 `parallel_convergence` 与含全部 sibling 的主 recipe/support lane，模型据此把 10 条同 caller 边列作“关键中间函数”并称“并行汇聚”。第一次成文还泄漏 `</parameter>`，严格拒绝后一次恢复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- Runner：`2 PASS / 2`；人工：`1 pass / 1 partial`。
- `qf_called_by_typed_relation_query` 是生产正证：调用者集合、调用点基数、测试排除和图边方向均正确。
- `qf_sequence_analyzer_gate` 证明 B785 的 support-plan 去重越界已闭环；同时确认 B688 只靠软教学不够，因为 typed no-directed-path 边界的另一系统载体仍向模型发布相反的 `parallel_convergence` 词源，并把边界外 sibling calls 放进 principal recipe。
- 根修编号 `B786-ENDPOINTBOUNDARYPRINCIPALSCOPE1`：精确 `no_directed_path` 下，主 support lane 与 copy-ready relation recipe 只消费 endpoint evidence capsule 中解释边界的真实方向边；其他已证边保留为 support-only。静态共享 callee 形改称 `shared_callee_boundary`，明确不证明并行、汇聚、join 或时序。
- 该修复只消费 typed endpoint disposition/status/call edges，不扫描请求、推理或终稿原文，不替模型写答案、结论或图；Trace 显式时间窗、因果投影、自动补齐、链上-only 主因均未改动。
