# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T07:43:17Z
- sweep_start_ts: 20260901-004315
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260901-004317 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 193s | 43 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗、链上根因排序、实际占时/规则可消双账户、业务 span 线索、确定性补齐与 Trace 因果投影均在场。主因只取链上 NetworkService-60595 优先级反转候选，有效量 5.951ms；VerifyClass 只作为业务优化线索且明确未把语义完成臆断成唤醒因果。邻近 I/O/压力保持背景席；无固定 4ms/4m 或活跃流降级。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260901-004317 | answer_regex,answer_contains | none | 827s | 50 | read=70,repo_map=2,list=0,trace=0,source_lens=0 | midloop=32,inv=14/4,fin_reject=4,unavail=2,prune=1 | pass | B1532 正证：最终仅复用 SB→SE→SX→SF 一套阶段参与者，无重复 stage actor。B1533 正证：关系编辑先暂存，下一轮只发布完整 2 行 orphan roster；模型自行选择 retain EM/MU，系统未代删。答案阶段表/引用正确。流程仍偏重（70 reads、73 explorer iterations、4 finalizer rejects）；删除 forward call 后原本获结构豁免的 reply 变为悬空，只在下一轮才被发现，确认 B1534 编辑后成对关系依赖闭包 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
