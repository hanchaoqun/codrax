# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T05:19:41Z
- sweep_start_ts: 20260812-221939
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260812-221941 | answer_regex,answer_contains | none | 333s | 35 | read=15,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=3,unavail=0,prune=0 | partial | B706 生产正证：route 明确 `diagram_required=true`，required 图未再被 optional 删除。三次拒绝分别钉住未证边、patch 缺 kind、第二稿未证调用，均正确；终稿保留 Mermaid+表。但图把 stage precedence 与两个函数调用片段并列成断开 sequence，且页面披露“必答面硬转软 ×1”，关系解释仍不够自然，列为 required-boundary 展示审计，不降证据门、不由系统补边。 |
| 2 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260812-221941 | log_regex,answer_regex | none | 437s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B708 P0：规则 authority=`output_field=ids`，内部 group identity=`active_user_ids`，值/顺序/ledger/reconcile 都正确；`assemble_answer` 只能用 group_key 作外部键，typed action 无 rename，Evaluator 要求修复而执行器无表达能力，custom_transform complete-stage 禁用正确。4 次修补后仍输出错误键并 complete，属于确定性合同自冲突，不是模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 任务状态

- `B706=production-positive-r422`。
- `B707=implemented/four-surface-pins-pass`：ready_to_plan 隐藏/拒绝 apply+verify；待提交。
- `B708=confirmed/P0`：外部 output field 与内部 group identity 分离，下一批施工。
- `required-diagram-boundary-display=partial`：继续异构图 case 审计，禁止系统代画/补边与答案关键词门。
- `active-stream-4ms-degrade=forbidden/not-observed`；Trace 显式窗/因果投影/自动补齐未改。
