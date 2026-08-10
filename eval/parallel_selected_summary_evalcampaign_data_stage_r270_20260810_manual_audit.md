# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T15:57:46Z
- sweep_start_ts: 20260810-085744
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260810-085746 | log_regex,answer_regex | none | 284s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 输出 `17,5`，遗漏权威 reference 全集中的 T2 零值槽。typed terminal 同时记录 reference_key_count=3、answer_item_count=2，却仍 status=satisfied；reconcile 只对已经产生 contribution 的 T1/T3 两组自洽，未覆盖 T2。不是模型数值波动，而是终态 reference 基数/成员闭包合同缺失。 |
| 2 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260810-085746 | answer_regex,answer_contains,mermaid_edge_count | none | 460s | 41 | read=12,repo_map=2,list=0,trace=0,source_lens=0 | midloop=14,inv=5/0,fin_reject=8,unavail=0,prune=1 | fail | B462 正证：纯 `mutable *types.MutableState` assignment 伪证不再出现在最终关系。B463 首次生产到达后暴露自冲突：模型按提示把 analyzer/explorer/extractor/finalizer 留作合法 Mermaid 裸节点并提交 unproven boundary，但 visibility parser 只识别带括号/方括号的声明，不识别合法 bare node statement，连续 4 次同类拒绝后降级。前段 dotted endpoint 还经历 source repair 与 edge-anchor 同步修复，B464 仍需独立结构审计。最终图只有旁支 Orchestrator 调用，四个请求主体无关系，且正文含 extractor 误引，人工失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- r270 runner `0/2`，人工 `0/2`；两项均为确定性系统 GAP，不归因于模型波动。
- `EVAL-B465-REFERENCEUNIVERSECLOSURE1/P0`：complete terminal 必须让最终输出成员与 typed reference candidate 的完整、有序成员集同构。`reference_key_count=3` 与 `answer_item_count=2` 不得签 `satisfied`；零贡献成员必须由 typed reference carrier 留在 reconcile/final projection，不从 prose 或期望答案猜值。
- `EVAL-B466-MERMAIDBARENODE1/P0`：B463 的“断开可见节点 + unproven boundary”合同必须接受 Mermaid flowchart 的合法 standalone bare node statement；识别只基于 Mermaid 语法并排除 reserved statements，不能扫答案文字或把 node membership 当 relation evidence。
- `EVAL-B464-DIAGRAMENDPOINTALIAS1/P1` 保持开放：本轮 dotted code identity 经 Mermaid source repair 改写为 `codraxNodeN`，关系 metadata 同步发生过 17 次机械修复。需以 repair provenance 的精确 alias map 统一 body 与 anchor，而不是放宽 identity/evidence gate。
- Trace 车道在 B463 的语义视图、pre/post checker 均明确旁路；本轮未改 Trace 时间窗、系统补齐、因果投影、链上主因/业务线索或背景分层。
