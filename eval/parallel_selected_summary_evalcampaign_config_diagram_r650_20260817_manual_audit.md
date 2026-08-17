# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T23:10:31Z
- sweep_start_ts: 20260817-161029
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_config_two_knobs_precedence | FAIL | eval/results/read_combo_config_two_knobs_precedence-20260817-161031 | answer_regex,answer_contains | none | 184s | 30 | read=8,repo_map=0,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B1024 的 finite-target 修复已使 `exact_resolution_present=true`，但正向配置映射仍按“全局见过一个层级”收口：`defaultMaxSteps=50` 错误替 sibling key 满足 default 层，未读 `cmd/root.go:3147 MaxRetriesPerStage: 3`。终稿把示例注释 2 当实际默认；这是 typed target×role coverage gap，不是模型波动。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-161031 | answer_regex,answer_contains,mermaid_edge_count | none | 431s | 41 | read=24,repo_map=9,list=0,trace=0,source_lens=4 | midloop=9,inv=7/1,fin_reject=2,unavail=0,prune=5 | partial | Mermaid 最终可渲染，四阶段顺序有 grounded precedence 边，validator 两次拒绝未证边是正确的；但用户要求的 Mutable/BusContext 数据流最终只剩孤立分组和“未证关系边界”。日志把 analyzer/explorer/finalizer/Mutable 降为 request_visible_boundary_only，仅 extractor/BusContext 进入 source_operation_required，说明跨语言/大小写/角色身份到源码操作的统一关系解析仍未闭环，不能因 runner PASS 关账。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- Config：确认 P1 `B1025-CONFIGTARGETROLEMATRIX1`。最优方案不是锁定数字 3，而是让 finite named config mapping 在 analyzer 层保留 typed exact targets/roles，并在 resolved completion 前逐个检查 `target × requested role`。只读 schema-validated carriers 与 grounded evidence，不扫描用户/模型正文。
- Diagram：`B1011-ENCLOSINGCALLABLERELATIONNAV1` 仍为 partial，新增生产 witness `B1026-DIAGRAMPARTICIPANTOPIDENTITY1`。后续应统一修复 requested participant → source operation identity 的解析/别名/大小写与 carrier handoff 证明，不增加 case-specific Mermaid 边，也不由系统代画关系。
- 安全边界：本批未触及 Trace 查询、显式窗、因果投影、自动补齐、链上根因席位或流式响应；没有固定 4ms 降级，也没有系统替模型改写结论。
