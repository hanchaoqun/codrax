# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T12:12:24Z
- sweep_start_ts: 20260816-051223
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_libgit2_foreach_worktree | FAIL | eval/results/github_issue_libgit2_foreach_worktree-20260816-051224 | write_apply,write_patch_oracle | none | 200s | 25 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | B902 生产闭环：plan 1 修 line 12 后验证发现 line 16；同 BatchID replan 的 plan 2 只修 line 16，测试通过，controller 直接 finish/all_verified，没有再 append 或虚构第三次修改。实际累计 diff 正确。但终稿仍称“未完全验证”：累计 proof ledger 已把旧失败命令降为被终态同命令通过覆盖，却保留旧 plan 在同一 repository.c 上的 changed_file=unverified 义务，随后 enforceTerminalCumulativeProofAuthority 把同页 loop.truth=covered/completion_verdict=verified 降成 unverified。冻结 B903。 |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-051224 | answer_regex,answer_contains | none | 394s | 38 | read=16,repo_map=2,list=0,trace=0,source_lens=1 | midloop=7,inv=7/0,fin_reject=3,unavail=0,prune=0 | partial | Name literal、首次 full/retry patch 和两条 Execute 路径大体正确，runner 仍为宽正则 PASS。B900-v2 未触发：Analyzer 合理地把“适用时机”标成 function_or_purpose，同时仍把埋在 call_chain_endpoints 内的 runtime_selection_required 置 false；说明把选择义务复用为 presentation role 是错误抽象。Explorer 24+ 轮反复被“必须找两个工具间 direct call”推动，终稿图没有画 finalizer 的条件选择关系，末句又错误声称两条路径最终汇聚到 executeAnswerDocumentV2，与前文“patch 不走该函数”矛盾。冻结 B904：独立 required runtime_selection_profile。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- `B902-WRITEREPLANBATCHGOALAUTHORITY1=production-closed-r566`：旧的错误 append/第三次虚构修改消失。
- `B903-CUMULATIVEPATHPROOFRECONCILIATION1`：只有 controller-owned cumulative scope 同时精确绑定旧 PlanID 和 path，且终态 report 对该 exact path 为 covered 时，旧 changed_file 未覆盖义务才可转 covered；相同路径但未绑定的历史记录不得消账。
- `B900v2` 的 presentation-role 方案被生产否证并退役。`B904-RUNTIMESELECTIONPROFILE1` 将条件化工具/实现选择从 call-chain endpoint 和 generic purpose 中拆成独立、必答的小 profile；系统只校验 typed boolean/confidence 与逐字 quote 来源，再投影到既有下游 carrier，不扫描关键词、不生成关系或答案。
- 本批未改变 Trace 显式时间窗、因果投影、自动补齐、链上-only 根因和 off-chain support 边界。
