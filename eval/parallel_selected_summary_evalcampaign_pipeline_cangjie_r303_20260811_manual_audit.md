# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T09:04:02Z
- sweep_start_ts: 20260811-020400
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260811-020402 | typed_inventory_rowset,dimension_substring,answer_contains | none | 192s | 24 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | B514 获生产正证：本轮 cells-only/row-id 路径不再出现 mismatch，也没有退回上一版草稿。新失败来自独立的 typed 集合语义冲突：Explorer 发出的 set_label 为 `public class（不含 abstract/sealed）`，但同一强制 roster 又包含 `Animal` 与 `Service`；最终答案同时列出它们并声明不计入，标题/成员/计数互相矛盾。第二轮 patch 还曾漏 `kind`，但随后合法 patch 已接纳，不能把主要失败归为 JSON 波动。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-020402 | answer_regex,answer_contains | none | 412s | 43 | read=15,repo_map=2,list=1,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=2,unavail=0,prune=4 | fail | B515 获生产正证：JSON-string blocks 被结构恢复，3 次 Mermaid source repair 后 dotted/call display alias 均安全加引号；B510-G 也把 analyze→explore→extract→finalizer 三条 precedence 保进最终 sequence。runner 虽 PASS，人工仍 fail：模型正文错误声称 Finalizer 不调用 LLM，图中数据载体全部断开且消息只写内部词 `precedence/call`，412s/24 explorer 轮/2 次成文拒绝说明关系教学与边界修补仍有高心智成本。系统没有代写边或答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
