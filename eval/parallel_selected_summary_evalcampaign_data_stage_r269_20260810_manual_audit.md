# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T13:50:27Z
- sweep_start_ts: 20260810-065025
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260810-065027 | log_regex,answer_regex | none | 357s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B461 production 正证：终值精确为 `17,0,5`；terminal audit 为 complete，3 条 contribution 与最终投影一致。稳定 source-row identity 未被派生 artifact 覆盖。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-065027 | answer_regex,answer_contains,mermaid_edge_count | none | 619s | 43 | read=15,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=6/1,fin_reject=7,unavail=0,prune=2 | fail | B460 production 正证：Analyzer 明确发射 6 个 `incident_required` participants；但图只剩 Orchestrator 两条调用和四条注册边，BusContext/MutableState/四阶段之间的数据流仍断开。上游把 `mutable *types.MutableState` 纯字段声明误铸为 assignment，Finalizer 照 recipe 画边后又被严格 validator 拒绝，7 次 patch 后删边签绿。runner 的任意边计数是假阳性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- `B461=production-closed`：值、资格决定、贡献与终态投影一致；本轮没有发现系统替模型作业务选择。
- `B460=production-closed`：required diagram 的 participant slate 已在真实 Analyzer 输出中完整到达，后续缺图不再由字段省略导致。
- `EVAL-B462-TYPEDASSIGNMENTSHAPE1/P0`：grounder Tier 1 只因精确 token 命中，就把纯字段/类型声明授予 `assignment_fact`。同一 typed recipe 随后被 answer diagram 的 assignment 语义门拒绝，形成确定性跨层合同冲突。修复必须在证据铸造源头要求 assignment/initializer 的结构形状；不得放宽成文关系门。
- `EVAL-B463-DIAGRAMPARTICIPANTCOVERAGE1/P1`：6 个 `incident_required` participant 中 5 个没有 incident relation，最终结构仍可签绿；现有 completion guidance 是软的，runner 只数任意 Mermaid 边。需要模型填写的 typed coverage/boundary 载体，让“有关系”或“明确 unproven boundary”成为结构化决定；系统不得猜边、扫描答案 prose 或代写结论。
- `EVAL-B464-DIAGRAMENDPOINTALIAS1/P1`：raw identity `explorerEvaluator.mutable` 直接作为 Mermaid node id 会被点号截断，而 copy-ready alias 与 body/anchor 的同源约束未稳定到达。它与 B462 分开：先消除伪 assignment authority，再以结构化 alias 修复合法复合 identity 的表达，不能针对当前字符串打补丁。

本轮未触碰 Trace 查询、显式时间窗、自动补齐、因果投影或链上根因选举。邻近/背景证据仍不得进入主因席；该约束不通过用户原文或模型答案关键词硬门实现。
