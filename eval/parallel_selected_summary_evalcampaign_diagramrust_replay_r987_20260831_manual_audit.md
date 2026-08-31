# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T19:36:34Z
- sweep_start_ts: 20260831-123632
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260831-123634 | answer_regex | none | 99s | 28 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 主链与 walker 职责正确，且没有再写反 fixed 分支；但 B1506 预期的 `if fixed -> LiteralMatcher` / `else -> RegexLikeMatcher` 没有进入 typed handoff。生产 evidence batch 只发出 run 的 call rows，没有 run definition；当前 enrich 只接受 definition row，所以只出现 matcher/walker 内部 branch_effect。另有独立 JSON 恢复缺口：模型把 blocks 发成 JSON string，brace-balanced recovery 只恢复 2/3 结构块，ordered_list 丢失，图只能作为“系统保留内容”显示。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260831-123634 | answer_regex,answer_contains | none | 564s | 45 | read=27,repo_map=3,list=0,trace=0,source_lens=1 | midloop=18,inv=4/0,fin_reject=3,unavail=0,prune=4 | pass | 最终主图完整保留 `analyze -> explorer -> extractor -> finalizer`，不再混入 Phase1/Dispatch 碎片；表格和正文覆盖输入、输出、AnalysisIR/EvidenceItems/AnswerDocument/Mutable/BusContext。B1505 核心闭环。仍有 3 次修补：首次 principal alias 与当前节点 id 双表述使模型误加 visible-label 字段；完成主链替换后，初始 delta 未一次暴露 3 条同端点 Orch->BC 无锚边，产生级联修补。记为协议收敛 P2，不否定答案正确性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner: 2/2 PASS; human: 1 pass / 1 fail.
- B1505: production-positive and core-closed. The remaining three repair rounds are protocol-efficiency debt, not relation loss.
- B1506: targeted tests passed but production admission is partial. A call-chain-selected callable represented by exact call-edge rows is not admitted unless the model also emits a separate definition row.
- New P1 `B1507-CALLERSELECTEDBRANCHHANDOFF1`: derive the selected callable only from an exact, citable parser call row whose call site lies inside one unique already-read parser callable; project that callable's parser-owned branch effects. No request/prose/Mermaid scanning.
- New P1 `B1508-ANSWERDOCSTRINGRECOVERYCOVERAGE1`: nested-string recovery may publish a partial structured document while relegating a recoverable diagram to a display attachment and silently losing a sibling list. Preserve recovery accounting and surface an explicit incomplete-structure status; prefer complete block recovery when boundaries are independently valid, without inventing content.
- New P2 `B1509-RELATIONDELTAFIXPOINT1`: initial relation repair delta did not expose every duplicate unsupported visible edge, forcing a later same-endpoint cleanup round. Audit producer fixpoint/exhaustiveness before changing validation authority.
