# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T12:38:15Z
- sweep_start_ts: 20260816-053814
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_libgit2_foreach_worktree | FAIL | eval/results/github_issue_libgit2_foreach_worktree-20260816-053815 | write_apply,write_patch_oracle | none | 160s | 25 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 单计划、单文件、测试和 controller finish 均正常，B902 保持闭环；但第一处把原有 callback 任意非零终止语义从 `!= 0` 收窄为 `< 0`，只因现有测试仅覆盖负数而绿。Runner 正则不是假阴性，揭示了语义保持与正返回值验证缺口；本轮未覆盖 B903 多计划累计证明。 |
| 1 | read_combo_answer_document_tools | FAIL | eval/results/read_combo_answer_document_tools-20260816-053815 | answer_regex,answer_contains | none | 1225s | 62 | read=20,repo_map=1,list=0,trace=0,source_lens=0 | midloop=15,inv=6/0,fin_reject=25,unavail=1,prune=0 | fail | Name literal 和两工具文字说明大体可用，但 required flow diagram 与 typed relation authority 形成不可满足合同：finalizer 明示 `explicit_typed_directed_relations=0`、只有两个 unary guard 注释，同时 validator 要求图至少一条有 typed owner 的边。模型在 25 次拒绝中反复删图、改关系枚举和重连，最终降级旧稿且图无关系。Explorer 曾成功补发 `NewFinalizerAgent -> NewBaseAgent` call_edge，但同位置早期弱形在 handoff 中占位，最终 authority 仍报 `grounded_callsite_facts=0`。这是系统合同/证据接线 GAP，不是 JSON 畸形或模型随机波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
