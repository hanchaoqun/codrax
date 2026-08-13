# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T17:03:48Z
- sweep_start_ts: 20260813-100347
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-100348 | answer_regex | none | 146s | 24 | read=3,repo_map=3,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 关系正文完整；首稿仍把 guard/self 与绑定载体误画成 call，精确门拒绝。第二稿这次直接采用系统 typed skeleton，合法保留 Python→native、fallback、Rust wrapper→core、module→registration 四条关系图，B688/B723/B729 carrier 获生产正证。图中文字仍偏技术但没有内部 pipeline 术语。 |
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260813-100348 | write_apply,answer_regex | none | 448s | 23 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=6,prune=0 | pass (honest unverified) | B731 正证：runner 只剩 `write_final_verdict:unverified:verification_proof_incomplete`，虚假 durable ref 错误消失。新确认 B732：source extension 仅证明 JS probe 与 TS 兼容，Node 实际不在 PATH；controller 仍强制创建 `changes=[] + verification_probe_required=true` 批，而 planner schema 又说不可执行时应省略 probe，形成不可满足合同，耗 448s/多轮 unavailable。最优方案是在 typed runtime availability seam 创建 proof batch 前判定，缺 runtime 直接以 source_static_only 诚实结束。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
