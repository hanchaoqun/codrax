# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T23:00:01Z
- sweep_start_ts: 20260815-160000
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260815-160001 | answer_regex,answer_contains | none | 495s | 37 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=6,unavail=0,prune=0 | fail | B860 正证：required diagram 没有再被降为 optional，也没有被允许删除。B857 严重复现：结构拆分产生 `table-stage-detail_diagram` 后，模型不知道当前 rejected draft 的完整 block/派生 ID 清册，连续 patch 新增/保留到 3 个 diagram block；六次拒绝均要求降到 1 个，最终降级答案仍含三图和一组未证调用关系。系统没有代删图，但 patch 教学缺精确活动 block roster，修补无法收敛。 |
| 2 | qf_sequence_analyzer_gate | TIMEOUT | eval/results/qf_sequence_analyzer_gate-20260815-160001 | answer_regex,answer_contains | none | 1201s | 42 | read=12,repo_map=1,list=0,trace=0,source_lens=0 | midloop=12,inv=6/1,fin_reject=3,unavail=0,prune=0 | fail | 不是模型/网络波动。第 3 轮 patch 已提交 17 条带完整 endpoint identity 的 typed call anchor；本地 `normalizeDiagramEdgeAnchorIdentitiesByUniqueTypedTopology` 仍对 `buildAnalysisIR` 的 12 个同签名叶节点枚举同构，单核约 100%、RSS 约 1.4GB，1200s 超时。sample 栈稳定落在 `diagramComponentIsomorphisms` 递归。确认 B862：可选身份修复缺“无缺口跳过”和穷举预算。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Machine: `0/2` (`1 FAIL + 1 TIMEOUT`); human: `0/2`, both fail.
- `B860-DIAGRAMREQUIREDCONTRACTCONSISTENCY1` received production-positive evidence: required intent survived analyzer retry and the finalizer did not authorize diagram deletion.
- `B857-ANSWERDOCVALIDATIONWATERFALL1` is upgraded from observation to P0 confirmed across the read-combo replay. The generalized missing carrier is the exact active rejected-draft block roster, including normalization-derived IDs; the system should expose that typed patch base to the model, not delete/rewrite blocks itself.
- `B862-DIAGRAMTOPOLOGYISOMORPHISMBUDGET1` is P0 confirmed. Optional topology metadata recovery must skip fully identified components and discard all partial mappings when a deterministic search budget is exhausted.
- No Trace path ran in this batch. Explicit-window causal projection, automatic supplement, typed on-chain root-cause authority, actual-duration/business clues and rule-priced eliminable quantities remain unchanged.
