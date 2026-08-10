# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T11:07:01Z
- sweep_start_ts: 20260810-040700
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-040702 | answer_regex,answer_contains,mermaid_edge_count | none | 206s | 29 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=3/1,fin_reject=1,unavail=0,prune=0 | fail | runner 只看“任意 Mermaid 边”而假绿。Analyzer 的 `diagram_hint` 只有 `kind=flow,required=true`，漏掉当前请求明确点名的 analyzer/explorer/extractor/finalizer/BusContext/Mutable participant slate；B452 逐 participant 操作补证与 B457 typed stage authority 的 participant 臂因此都没有启动。Finalizer 第一稿有 15 条无 typed anchor 的臆造数据流，关系门正确拒绝；补丁只保留一条有凭证的 `StageLogTriage -> StagePerfTriage`，主四阶段与两类 context 关系全部缺席。B457 的 checkout-verified stage authority 已进入 prompt，producer split 正确；B459 因上游无 participant 而未获得 production witness。新立 B460。 |
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260810-040702 | log_regex,answer_regex | none | 571s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B458 正证：`filter_active_observations` 从 6 行得到 5 行，typed diagnostics 明确 `true/false`，r3(false) 已排除。但后续 `apply_entity_resolutions` 从原始 observations 重新生成 6 行支路，`eligible_contribution_records` 只按 resolution 状态过滤，贡献项又以派生 artifact 的 `item_id/source/locator` 重铸身份，无法与已有 r3 exclude decision 对齐。最终五条 contributions 把 r3 的 3 计回 GroupA，reconcile 对错误账签绿并输出 `20,0,5`，期望 `17,0,5`。新立 B461：typed 派生链必须保留稳定 origin row identity，使既有 decision↔contribution 一致性门可发现排除行复入。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- human pass: `0/2`。
- `EVAL-B458-BOOLFILTERDOMAIN1` 的生产值语义修复已经生效，但本 case 仍被更深的资格 lineage 断裂击穿，不能误判为 B458 回归。
- `EVAL-B457-STAGEAUTHORITYREACH1` 的 prompt 到达生产关闭；`EVAL-B459-TYPEDPARTICIPANTALIAS1` 保持代码完成、待真正携带 participant 的生产回放。
- `20260810-024101.766-24756.md` 与 `20260810-041025.835-45551.md` 都存在关系丢失，但机制不同：前者是 B459 修复前的装饰 participant identity 阻断，后者是 Analyzer 根本未发 participant slate。前者当前代码已根修但待同形正证；后者由 B460 新立案。
- 两个新件均不读取用户/模型 prose，不生成边、不改模型答案；Trace runtime/显式窗/自动补采/因果投影路径未进入本轮代码面。
