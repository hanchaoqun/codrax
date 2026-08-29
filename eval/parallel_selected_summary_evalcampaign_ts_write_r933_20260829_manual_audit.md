# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T09:12:59Z
- sweep_start_ts: 20260829-021259
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260829-021259 | write_apply,write_patch_oracle,answer_contains | none | 97s | 26 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只修改 `main.go` 的 `retrun -> return`；真实项目 `TestGreet` 与 `go test -json ./...` 通过，changed path 为 target_behavior covered、worktree clean；controller 首次终局决策即 `finish/all_verified`，最终交付状态完整。planner 本轮遵循 probe-optional 教学，`verification_probes=0`，因此没有自然激活 B1447 的非权威失败探针分支；本轮证明正常路径无回归，不虚记该精确分支 production-positive。 |
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260829-021259 | answer_regex,answer_contains | none | 257s | 29 | read=9,repo_map=1,list=1,trace=0,source_lens=0 | midloop=5,inv=3/0,fin_reject=3,unavail=0,prune=0 | partial | 主链、paths 映射、`<500` 直接返回、`>=500` 重试、nextDelay/sleep 与 Mermaid 时序主体均有源码证据。B1446 的 grounded `addition_ref` 在生产出厂，模型最终用 `failure_ref + addition_ref + attach` 修复 `Send -> Dispatch` 并通过；但仍有 3 次拒绝：首稿重复同一调用边并画了错误回包方向，第二轮后模型又违反已发布原子权限尝试 whole-block replace。终稿首段错误写成“状态码小于 500 时调用 nextDelay/sleep”，与后文及源码“<500 直接返回”自相矛盾，故不能判 human pass。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `patch_go_typo`: runner PASS / human PASS. r932 的 532s blocked 降为 97s verified finish；无固定时限降级、无无效 replan、无系统代写。B1447 精确 probe reconciliation 仍是 full-suite/controller-pin positive，等待带同一 typed 形状的自然生产触发。
- `sr_ts_workspace_chain`: runner PASS / human partial. B1446 已获得“candidate 出厂 + attach 执行成功”的生产证据；3 次拒绝不再是“系统不给 attach”，而是初稿结构错误与模型一次无视 atomic lease。最终答案的 retry guard 自相矛盾是模型对精确源码上下文的遵循波动，本轮不新增请求/模型/答案正文扫描硬门，也不由系统改写模型结论。
- `B1445` 的 identity-missing prior-anchor 形与本轮 zero-anchor list 不同；zero-anchor 没有可原子重绑的既有 metadata carrier，模型通过 bounded whole-block replacement 补齐，当前不据单 case 扩权为系统自动加边。
