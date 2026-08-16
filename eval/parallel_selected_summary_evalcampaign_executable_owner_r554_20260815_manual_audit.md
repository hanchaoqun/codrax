# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T05:57:38Z
- sweep_start_ts: 20260815-225736
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260815-225738 | answer_regex,answer_contains | none | 108s | 28 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B880 + B881：正文无证据地声称 NewFinalizerAgent 注册/选择两个工具，并称 patch 无公开 Execute；关系图同样以无 typed registration/selection authority 的边表示该主链。更严重的是输入图含 pipe/quote 高风险标签，兼容改写产出明显非法 Mermaid 语法，却仍以 mermaid fence 交付而非 L7 text+可见原因，runner 仍 PASS。 |
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260815-225738 | answer_regex,answer_contains | none | 167s | 28 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B879e：JSON 双轴教学已生效，模型正确发出 EvidenceMechanism+definition；但本轮没有 executable guard/return/assignment，且 completion member_set 省略显式 role，B879d 精确无误触但未下钻。最终仍把 rejected-draft sanitizer 当普通 REPL 失败链，并未读取/解释 maybeReplaceMermaidFence 与 mermaidFallbackFence。需要 Explorer-selected callable mechanism-definition seed，不能扫描 summary/答案文本。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
