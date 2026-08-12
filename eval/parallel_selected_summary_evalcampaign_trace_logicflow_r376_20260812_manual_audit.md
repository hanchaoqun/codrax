# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T08:55:09Z
- sweep_start_ts: 20260812-015507
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-015509 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 286s | 49 | read=12,repo_map=0,list=0,trace=2,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=8 | pass | B629 正向生效：tail_open 不再以无 support 的模型 scalar 与最终账本竞争；正文只把 1.396ms 标为未归账。显式窗、目标状态、on-chain-only 根因、PI/调度与算力供给/D/IO/确定性语义、业务线索、占用与可消双轴、因果投影和自动补齐完整。邻近席仍仅作背景。效率较差：模型为“全部小来源”反复分页 33720 行 payload，17 轮/12 次 read_file。 |
| 2 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260812-015509 | answer_regex,answer_contains,mermaid_edge_count | none | 421s | 37 | read=8,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=9,unavail=0,prune=0 | fail | B630 containment 教学进入 prompt，但 participant coverage 把精确 endpoint node id `Mutable` 与业务 label `MutableState` 分裂处理：身份存在门认可 node id，incident 门却只认 label/group。模型已画正确 typed call endpoint，仍连续 9 次被同一合同拒绝，最终恢复旧稿并标 degraded。BusContext containment/直接流边界方向正确；失败不是模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- 两案均在活跃模型流超过四分钟后继续工作；没有按 elapsed age 触发旧稿、空答案或降级。逻辑图案的降级来自同一精确结构校验连续失败，和 4 分钟无关。
- r376 新确认 `B630b-DIAGRAMNODEIDENTITYDISPLAY1/P0`：稳定 Mermaid node id 已是结构身份，但 participant incidence 又要求可见 label 重复 typed 名称，和“业务 label + 技术 edge_anchors”教学自冲突。
- r376 同时立案 `B631-TRACEPAYLOADNAVIGATION1/P1`：root_cause_rank 已提供 typed payload_ref，却缺少面向 compacted rank/section 的结构化分页入口；“全部小来源”问题迫使模型遍历 33720 行 JSON。只优化导航，不削减 trace 数据、因果权限或完整性。
