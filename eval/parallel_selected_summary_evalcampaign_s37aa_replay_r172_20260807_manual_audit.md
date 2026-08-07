# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T15:44:28Z
- sweep_start_ts: 20260807-084427
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260807-084428 | answer_regex,answer_contains | none | 136s | 25 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 最终表格与 Mermaid 正确保持 `StageAnalyze -> StageExplore -> StageExtract -> StageFinalize`，4 个成员均有当前源码引用，一次成文、零拒绝。独立 P1：这是非严格 DiagramFlow/QFEnumeration，模型发出的 3 个 `relation_kind=precedence` 当前可直接成为边 authority；日志虽有有序 aggregate/member_set，但系统尚未把该 typed 顺序编译成关系 authority，也没有拒绝无来源的逻辑关系 enum。不能直接把全部 flow 图升级为 strict source-call 门，否则会与合法概念流程冲突；应先建立有序 typed aggregate/source relation authority，再统一验证 guard/import/precedence/contain/observe。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260807-084429 | answer_regex,answer_contains | none | 151s | 29 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | runner 的字符串 oracle 误绿。Explorer 已读到真实方向 `buildAnalysisIR -> gate.RunWith` 与 wrapper `gate.Run -> RunWith`，但 completion reason、最终摘要和列表反写成 `gate.RunWith -> gate.Run`。图证据门正确拒绝并由 patch 删除伪边，系统没有扫描或改写模型正文，因此错误 prose 留存。根因在更早的 typed IR：Analyzer 发出 `question_kind=call_chain` 和精确 ordered endpoints，却漏发 `predicate_axis=call`；归一化器静默丢弃 endpoint profile，导致既有 exact reachability/no_directed_path 门失去输入。记 `EVAL-B283-CALLAXISLOSS1`，应在 typed classification 一致性层修复遗漏轴、拒绝显式冲突，不能新增答案原文硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
