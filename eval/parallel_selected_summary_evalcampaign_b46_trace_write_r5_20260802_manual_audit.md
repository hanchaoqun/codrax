# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T20:39:46Z
- sweep_start_ts: 20260802-133945
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h5_smr_multirow_disposition | FAIL | eval/results/real_trace_h5_smr_multirow_disposition-20260802-133946 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 244s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Runner 只缺旧固定词形“等待对象 dma_fence_default_w”；typed 内核调用点实际完整存在。真正人工失败是关系服从性：root-cause tool head 已在第 20–24 行给 Explorer 精确 two-ruler（#4+#13=5.149ms、#10=1.648ms、跨尺禁加）、row-local state scope、fix-direction 非加法权限和 blocked-reason census 边界，Explorer closure 仍先写 #4+#10=5.604ms、CompThread running+D-state=11.892ms；Finalizer 收到同源准确 handoff 后继续沿用，并额外把 JankManager/keva 同线程异行相加。显式窗、5 次 windowed query、自动补齐、两维占用/可消、排序、唤醒链和完整 Trace 因果投影均保留。证明 transport 已准确但自然语言软提示不足，应让模型提交结构化 relation claims 并 against typed carrier 精确校验后自修复；系统不得改写正文。 |
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260802-133946 | log_regex,write_apply,write_patch_oracle | none | 265s | 20 | read=10,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 仅改 `memoclaw/client.py`，sync/async 均为 POST `/v1/search` + JSON body；verification probe 与真实 `make check` 都通过，后者输出 `python text search contract ok`。ChangeReport=`passed`，Python changed path=`covered`，runner/source=`make`/`declared_coverage_test_surface`，写模式无回归。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
