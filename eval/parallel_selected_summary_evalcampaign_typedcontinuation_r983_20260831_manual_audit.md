# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T17:31:12Z
- sweep_start_ts: 20260831-103110
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260831-103112 | typed_inventory_rowset,dimension_substring,answer_contains | none | 238s | 28 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=3/0,fin_reject=1,unavail=0,prune=0 | pass | 精确 12 行、extend=2、foreign func=2、public class=8，符号/路径/package 均正确；不可用工具调用为 0。首稿按“类别、符号、路径、package”排列表格，虽每行 row_id 正确且符号位于第二列，校验器仍要求第一可见值是符号，造成一次可避免修补。 |
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-103112 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 277s | 43 | read=0,repo_map=0,list=0,trace=12,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 显式 10ms 窗、12 次 typed 查询、唤醒链、链上排序、实际占时/规则可消双账户、业务线索、自动补齐与最终 Trace 因果投影完整；邻近/背景未升为主因。首稿把 summary-only caliber 冗余放入 section，造成一次可避免修补。内部枚举泄漏和把宿主 runnable 误写成 work 有效归因均未复现。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- B1498 获得生产闭环：同一 Cangjie 用例的不可用工具调用由 r980/r981/r982 的 6/3/1 次降至 0，且没有牺牲 12 行权威清单。
- 两路最终答案均人工通过；Trace 的显式窗口、链上根因权限、实际占时/规则可消双账户和系统自动补齐未退化。
- 两路各一次 final reject 都指向同一待审根族：投影 JSON Schema 是否完整表达运行时校验器的字段作用域与行身份约束。先核代码合同，再决定是否施工；不据单次模型输出加关键词门或 case 豁免。
