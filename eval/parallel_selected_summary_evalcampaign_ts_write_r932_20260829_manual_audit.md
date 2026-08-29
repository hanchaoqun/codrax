# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T08:30:58Z
- sweep_start_ts: 20260829-013057
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260829-013058 | answer_regex,answer_contains | none | 194s | 32 | read=7,repo_map=2,list=1,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=3,unavail=0,prune=0 | pass | 最终答案完整、方向正确：CLI 到原生 fetch 的调用链、retry guard、nextDelay/sleep 与 `@app/core` paths/extends 均有源码引用；图和列表读者语义一致。过程仍有 3 次确定性拒绝：validator 能列出 6 个精确 typed 候选，却没有向同一 generation 发布 `action=attach`，B1445 生产未激活，模型只能整块重写/删除后重试。 |
| 2 | patch_go_typo | FAIL | eval/results/patch_go_typo-20260829-013058 | write_apply,write_patch_oracle,answer_contains | none | 532s | 26 | read=9,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | `main.go` typo 已正确修复，`TestGreet` 和真实项目 `go test -json ./...` 通过、changed path 为 covered、worktree clean；但 proof ledger 把已明确标记 `probe_non_authoritative` 的模型预探针失败保留为 hard failed capability，覆盖 controller 的 finish，触发无剩余源码义务的 replan。后续合同同时禁止空计划、重复旧 patch 和 no-op edit，最终 blocked 且没有有效完成答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `sr_ts_workspace_chain`: runner PASS / human PASS / process partial. B1445 的 schema、执行器与单元测试存在，但 production repair capability 没有拿到 validator 已知的 typed candidates；提示面与可执行权限面不是同一个候选提供者。
- `patch_go_typo`: runner FAIL / human FAIL / applied artifact correct. 失败根因是 proof authority 归并错误，不是源码、项目测试或用户目标不清。精确形为：同一 terminal report 中 `pre_suite_verification_probe` 失败，随后存在 `probe_primary_suite_continued + probe_non_authoritative`，真实项目 assertion 通过；旧 ledger 仍把第一行计为 hard failure。
- 不允许的修法：不能接受任意空计划、不能把所有 probe 失败忽略、不能扫描模型推理或最终答案找“测试通过”，也不能系统补写关系或结论。
