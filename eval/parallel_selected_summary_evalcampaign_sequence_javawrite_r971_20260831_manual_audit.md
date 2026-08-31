# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T12:52:57Z
- sweep_start_ts: 20260831-055255
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-055257 | write_plan,write_patch_oracle | none | 74s | 27 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确定位 `Main.java:16`，计划仅把 `retrun` 改为 `return`，状态保持 `pending_approval`，仓库源文件未修改；写前分析与验证探针仍有软性返工，但未造成范围扩张或错误计划。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260831-055257 | answer_regex,answer_contains | none | 226s | 32 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=3/0,fin_reject=2,unavail=0,prune=0 | pass | 最终答案准确表达 `buildAnalysisIR -> gate.RunWith <- gate.Run`，明确否定不存在证据的端点直达关系，时序图与清单均完整。首拒后 additions-only lease 真实命中：只向既有关系块追加两个隐藏锚；随后 `add_facet_id=member_set` 只补清单归属，均未改写模型可见内容。仍有一次可避免的整块替换拒绝，见 B1488。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- B1487 production coverage: covered. `diagram_edge_edits[action=add,addition_ref=…]` 在非 diagram 的 `ordered-list-1` 上一次性追加两条模型已选证据的隐藏关系锚；执行日志为 `partial unchanged=4 replace=1`，最终可见列表、文字与 Mermaid 保持首稿内容。`block_field_edits_v1[{field:add_facet_id,value:member_set}]` 随后独立成功，未再整块复制清单。
- B1488-METADATAREPAIRTEACH1: confirmed P1 efficiency/contract-teaching gap. 首次零锚拒绝已经铸造 additions-only lease，但错误正文仍要求“copy complete recipe into edge_anchors”，首轮 patch 教学又先介绍通用 `replace_blocks`；模型因此连续第二次选择整块替换并被同一个 lease 拒绝，直到新的提示才明确要求 `diagram_edge_edits`。r970 与 r971 同形复现，不能归为单次模型波动。最优修向是让首次精确拒绝直接发布当前可执行的原子动作名称与 refs；通用整块替换只在没有原子分支时作为后备。该修向只改变工具教学，不扫描用户/答案原文，也不写入任何关系或结论。
- No fixed elapsed/round/context degradation and no answer recovery fallback occurred. Trace projection and trace completion paths were not touched by this batch.
