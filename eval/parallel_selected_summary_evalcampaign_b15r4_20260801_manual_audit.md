# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T02:35:44Z
- sweep_start_ts: 20260731-193542
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_config_absent_present_mix | PASS | eval/results/read_combo_config_absent_present_mix-20260731-193544 | answer_regex,answer_contains | none | 170s | 19 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=2/1,fin_reject=0,unavail=0,prune=0 | uncertain | XR1 covered：上一轮错误的 document-level “未找到完全一致目标”横幅已消失，另一个 target 的 value=0 和源码锚点保留。该轮模型没有形成 grounder-accepted negative EvidenceItem（evidence ingest=0），所以 NEG1 按设计 fail-closed 不发布；accepted aggregate 仅证明 “production Go files excluding test/docs” 零命中，footer supplement 也只转录这一 scope，但模型正文仍扩成 YAML/CLI 三层。事实很可能正确，系统权限链却不足，记 NEG2，不以 machine PASS 收账整体答案。 |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | FAIL | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-193544 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 198s | 37 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 完整显式窗 Trace 因果投影未回退（projection_blocks=2）；根因榜、窗内可消除量、VerifyClass/类校验 0.285ms、最晚相关边 34579.496810s、直接裸边均在。模型本轮也正确把该工作计入链上 #2。FAIL 仅来自 runner 在 LC_ALL=C 下不能用 UTF-8 bracket expression 匹配中点，oracle 改为正式字面中点，不改生产逻辑。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B15-XR1`：真实回放 covered。
- `EVAL-B15-H8O1`：生产与人工均通过，仅 runner UTF-8 字符类不兼容；case
  固定到正式系统板字面中点。
- `EVAL-B15-H8MV1`：本轮未复现，继续保留为 model variance，不施工。
- `EVAL-B15-NEG2`：新 P2。accepted aggregate negative-search 与 verified
  negative evidence 是两个 authority source；后者缺席时，系统只在 footer
  发布 aggregate scope，无法用统一 lead 明确禁止模型扩写到未证配置层。
