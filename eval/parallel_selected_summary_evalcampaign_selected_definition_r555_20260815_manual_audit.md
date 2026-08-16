# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T06:24:18Z
- sweep_start_ts: 20260815-232417
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | FAIL | eval/results/read_combo_loose_multi_question_units-20260815-232418 | answer_regex,answer_contains | none | 198s | 37 | read=9,repo_map=0,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | B879e 的 selected-definition 精确信号已出现，但整体 `scenario=config_trace` 使 generic mechanism forced-read boundary 返回 false，因而零 semantic-descent read；只读到文件头后，模型仍把旧注释中的“失败原样保留”当成当前普通 REPL 机制，并把 degraded-draft sanitizer/emit gate 混入同一主链。Runner FAIL 是跨段正则不匹配的次级 oracle 假阴性；正文机制错误足以独立判 fail。另有一个已被替换的 ungrounded `MergeSettings` 行仍留在 evidence buffer，completion 却直接签高置信。 |
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260815-232418 | answer_regex,answer_contains | none | 679s | 35 | read=35,repo_map=0,list=0,trace=0,source_lens=0 | midloop=29,inv=10/0,fin_reject=5,unavail=0,prune=0 | fail | B880 持续：typed recipe 只有六条互不连通的实现边，无法证明“谁选择完整 emit/patch”的主关系；终稿仍无证据声称 NewFinalizerAgent 统一注册、answerDocumentEvaluator 决定工具。5 次成文拒绝、7 次 patch、679s 后才退成孤立代码边，且系统重复附上被拒第一稿和两份相同“输出维度核对”，显著稀释答案。B881 的既有非法 pipe/quote 归一化风险仍未修。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
