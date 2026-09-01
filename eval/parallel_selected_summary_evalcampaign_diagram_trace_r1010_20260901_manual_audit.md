# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T12:25:04Z
- sweep_start_ts: 20260901-052504
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260901-052504 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 197s | 43 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 长答案核心仍正确：精确 10ms 窗、链上 NetworkService-60595 首因 5.951ms、实际占时/规则可消双账户、链上业务线索、背景隔离、帧因果未证及投影/补采均在。但 B1544 第一批生产回放未闭环：模型同时把顶层版本写成字符串，并把完整 trace_root_causes 对象再次 JSON 字符串化；5 个候选 ID/顺序都在，系统因对象解码失败忽略可选报告，默认 `.root-causes.json` 未写。记 B1545，同根安全结构恢复，不是证据或模型选择缺失。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260901-052504 | answer_regex,answer_contains | none | 818s | 47 | read=44,repo_map=2,list=0,trace=0,source_lens=0 | midloop=29,inv=10/0,fin_reject=12,unavail=0,prune=12 | fail | 同一请求出现 12 次成文拒绝，最终图混入 n8/n9/n16、MergeEvidenceItemsIfChanged 等内部节点和“AnalysisIR/EvidenceItems 未证关系边界”，可读性与请求的四阶段时序明显退化。根因不是缺少 typed stage precedence，而是 analyzer 的 required diagram roster 含多条无请求锚的别名行；闭面判定先按这些原始 alias 计数，得到 0，随后虽删除非法 alias，却让 sibling 表格中的 AnalysisIR/EvidenceItems incident_required 逃过关系面隔离。记 B1546：当 required roster 没有至少两个精确关系面参与者时，非法推断行不得静默删除后放行外面实体；应在 analyzer 阶段精确 fail-loud 促使模型重交，避免把错误义务推迟到 finalizer 12 轮。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
