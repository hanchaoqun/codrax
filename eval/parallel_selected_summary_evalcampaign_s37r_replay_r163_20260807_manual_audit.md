# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T12:24:01Z
- sweep_start_ts: 20260807-052400
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260807-052402 | write_apply,write_patch_oracle,answer_contains | none | 149s | 20 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Auto Pilot 只把 `main.go` 的 `retrun` 改为 `return`；applied-tree diff 无旁改，2 个测试通过，报告 `passed=true`，最终状态为 verified。计划期有 3 次 schema/probe 修正，但没有成文拒绝、载体降级或模型答案所有权问题。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260807-052401 | answer_regex,answer_contains | none | 179s | 25 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | S37r 生产闭环：首稿 required diagram 被 typed call-edge 门拒绝后，当前 repair turn 收到同源 copy-ready Mermaid + 完整 anchors，模型一次 patch 即通过；相比 r162 的 6 次拒绝/501s 降至 1 次/179s。人工仍 FAIL：模型自己的有序列表、表格和总结只写 analyze → dispatch/apply → finalize，漏 `StageExplore` 与 `StageExtract`；二者仅存在于底部系统阶段绑定补充，不能冒充模型回答正确。B266 关闭，B267 保持开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B266-REQUIREDRECIPECARRY1`: production-closed. The precise producer-owned diagram carrier was replayed in the same repair turn and accepted after one patch; no system prose or conclusion replacement occurred.
- `EVAL-B267-PIPELINESTAGEROSTER1`: open/P1. A post-answer stage-binding supplement contains the missing stages, but the model-owned explanation and requested table omit them.
- Write-mode control: human-pass; exact one-line patch, tests green, no unrelated mutation.
- Trace runtime family was not selected in this batch; no Trace capability claim is inferred from these two cases.
