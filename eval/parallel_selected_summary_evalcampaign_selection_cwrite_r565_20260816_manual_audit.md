# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T11:49:28Z
- sweep_start_ts: 20260816-044927
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_libgit2_foreach_worktree | FAIL | eval/results/github_issue_libgit2_foreach_worktree-20260816-044928 | write_apply,write_patch_oracle | none | 369s | 25 | read=14,repo_map=0,list=2,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Plan 1 修正 line 12 后 verify 失败；同 batch replan 的 plan 2 正确修正 line 16 且测试通过。随后 replan 产物摘要覆盖 durable batch goal，controller 又把陈腐的 line-16 摘要当作剩余义务，错误 append 第二批；planner 读到代码已正确，空 changes 又被 schema 拒绝，最终虚构无意义括号改动并耗尽预算。根因是状态权威混淆，不是模型单轮波动：同一 BatchID 的 durable goal 可被 replan 叙事改写，context-pack artifact goal 又伪装成 controller 的 remaining-work authority。冻结 B902。 |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-044928 | answer_regex,answer_contains | none | 400s | 38 | read=9,repo_map=0,list=0,trace=0,source_lens=0 | midloop=11,inv=5/0,fin_reject=4,unavail=0,prune=0 | partial | 名称字面量和 full-vs-patch 的局部说明基本正确；但 Analyzer 在请求明确询问“首次完整输出 vs retry patch”时仍铸出 `runtime_selection_required=false`，Explorer 无选择关系载体。终稿图只列两个 Name 和 patch recovery 边，没有回答实际 finalizer 如何选工具；表中还称 evaluator 对 full emission“不干预”，与源码/正文冲突。Runner 的宽正则形成假阳性。B900 仅修缺字段，不覆盖字段存在但 typed 语义自相矛盾；冻结 B900-v2。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- `B900-RUNTIMESELECTIONMISSINGFIELDREPAIR1` 的 presence 修复已生效，但不能闭环 typed value。最优泛化修复是新增闭集回答维度 `runtime_selection`，并在 Analyzer 内只做 typed cross-field coherence：该维度 required 时，`call_chain_endpoints.runtime_selection_required` 必须为 true 且继续满足既有 verbatim source quote。它不扫描 request/final prose，也不替模型生成关系或答案。
- `B902-WRITEREPLANBATCHGOALAUTHORITY1` 是写模式高危状态 gap。同一 BatchID 的 replan 只能更新计划/探测/编辑叙事，不能改 durable batch goal；controller 应显示 active workflow 的 durable goal，并把 context-pack artifact summaries 明确降为 evidence，而非 remaining-work authority。append/split 新批次仍可铸造新 goal。
- 本批未发现需要收窄 Trace 显式时间窗、因果投影、自动补齐或链上根因合同的理由；也没有用关键词扫描用户输入或模型答案实施硬门。
