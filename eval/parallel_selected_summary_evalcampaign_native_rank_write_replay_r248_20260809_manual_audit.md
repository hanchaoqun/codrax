# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T08:59:52Z
- sweep_start_ts: 20260809-015951
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260809-015952 | write_plan,write_patch_oracle | none | 76s | 21 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只读 main.cpp 两次并生成一条 `retrun`→`return` patch；old/new text、行号、验收条件和 owner anchors 一致。plan-only 没有应用或伪称已执行 g++，本批 structured JSON 改动未污染 write mode。 |
| 1 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260809-015952 | log_regex,answer_regex | none | 388s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 终态 failed，无用户答案。B425 两次在 owning schema 精确拦住越 rank 原生 JSON，证明修复生效；但 typed state 同轮允许 assemble_answer，执行 guard 又因无 reconcile 拒绝，随后允许 reconcile 又因无 contributions 必败。3 repairs/4 action failures 后预算耗尽，立案 B426 typed projection reachability conflict。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

- `patch_cpp_typo`: human PASS。计划精准、未越权 apply，证明 B425 的 REPL structured-tool 公共解码改动没有改变 write/plan 的拥有方合同。
- `data_json_strict_ids`: human FAIL。系统最终只返回 workflow failure，没有 JSON 答案。B425 确实把原生 `derive_rules`/未来-rank action 在 schema 边界变成 bounded repair，而不是晚到 workflow execution；但这使后续既存合同冲突更清楚：`allowed_next_actions` 发布了当前必被 upstream-ledger guard 拒绝的动作。
- B426 是系统确定性红线，不是模型波动：同一 typed 状态不能同时宣称 `assemble_answer` 已允许，又要求它必须先有尚不存在的 reconcile；同理不能在 contributions=0 时发布 `reconcile_artifacts`。继续加 JSON 教学会增加模型心智，无法使矛盾合同变得可满足。
- 最优根修应令 stage、allowed contracts、ledger graph、decision advisory、schema enum 与 guard 共用 capability facts。direct custom projection 可用时只发布该直接路径；custom 因 typed 风险被禁用时，将 contributions/reconcile 作为投影的操作前置，按 `prepare -> reconcile -> assemble` 逐 rank 开放。
