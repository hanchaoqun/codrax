# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T09:49:51Z
- sweep_start_ts: 20260818-024950
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-024952 | answer_regex,answer_contains | none | 106s | 26 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | 三个平台实现和 `cmd_sleep` 三处调用的正文、关系列表、12 条 citation 池均正确产生；但模型把 Windows/macOS/POSIX item 的索引分别指向 `handlers.c:30/32/34`，系统没有 member-local composite support set 可用于安全纠正，最终三项仍引用错误，B1064 稳定复现。第一次关系 patch 只给 edge anchors，既有 typed metadata-only 保留器恢复了 items/claim_uses；第二次拒绝是首轮只报告 visible_label、未同时报告已缺失的 endpoint identities，确认 B1065。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-024952 | answer_regex,answer_contains | none | 641s | 39 | read=30,repo_map=1,list=1,trace=0,source_lens=0 | midloop=20,inv=13/0,fin_reject=4,unavail=0,prune=0 | partial | 终稿结论与图正确：`buildAnalysisIR → RunWith ← gate.Run`，不存在到 `gate.Run` 的有向调用链；七个关键内部调用及逐行引用保留，Mermaid 合法。过程却严重失控：调用链请求被机制语义下钻反复扩展 helper body，每次 completion 后又排新读集，累计 45 Explorer 轮、13 次 completion、30 次 read、一次 Explorer 重启，确认 B1066；成文再因关系可见标签/identity/图 body anchors 被函数内早返回串行报告，产生 4 次 patch，确认 B1065。无空答案或降级发布。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap disposition

- `B1063-PATCHREPLACEMENTFIELDRETENTION1`: production behavior preserved all prior fields in both cases. QF's first replacement copied the full prior block (although it incorrectly string-wrapped the array and compatibility repair decoded it); C omitted prior fields, but the existing typed relation-metadata-only preservation seam restored them. The teaching is present and no field loss recurred, but the C witness shows prompt text alone is not the safety owner.
- `B1064-PRINCIPALMEMBERCOMPOSITESUPPORT1`: reproduced. The platform answer had every source line in the citation pool but still attached handler lines to platform rows because the handoff lacks a typed per-member support set spanning definition/branch/API calls.
- `B1065-RELATIONVALIDATIONBATCHFEEDBACK1`: confirmed by r678 and implemented in the next small batch. `preCheckDiagramCallEdgeEvidenceAlignment` now aggregates independently precise standalone visibility, endpoint-identity, and diagram-body ownership hints from the same draft; a production-wire pin requires all three issue classes in one response. Evidence gates and model relation ownership are unchanged; replay remains pending.
- `B1066-CALLCHAINMECHANISMDESCENTFRONTIER1`: confirmed P1/high-ROI. A typed call-chain request is admitted to mechanism semantic descent; Explorer-authored executable rows seed `followAllCalls`, each completion attempt expands a new bounded frontier, and a redispatch loses the prior dispatch's scanned-set recognition while persistent pending reads survive. The fix must route call-chain closure to call-chain evidence gates, not lower evidence requirements or cap the model by request keywords.
- JSON recovery behaved as designed (`blocks`/`replace_blocks` string-wrapped arrays were recovered and disclosed), but teaching did not prevent the model mistake; retain recovery and continue reducing schema mental load.
- No Trace code changed; explicit-window projection and active-stream fixed-4ms prohibition remain intact.
