# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T16:50:08Z
- sweep_start_ts: 20260813-095006
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-095008 | answer_regex | none | 133s | 24 | read=1,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | Python `_HAVE_NATIVE` 分支、`_fastlex.tokenize_bytes`、PyO3 `add_function` 注册、Rust wrapper→core→`best_merge` 与纯 Python fallback 均逐跳可见。模型未采用系统已给出的三条 typed skeleton，反而扩画 `Native→Rust` 伪调用、把 guard/self edge 标成 call 并给无正向调用的 reply；validator 正确拒绝，模型随后只撤可选图，正文关系未丢。属低优先模型作图波动，不放松 call authority、不由系统代画。另有“性能显著下降”缺 benchmark，记软措辞债。 |
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260813-095008 | write_apply,answer_regex | none | 223s | 23 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=2,prune=0 | pass (honest unverified) | 生产改动与 false/0/空串、existing-default 回归均正确；Node 不在主机 PATH，JavaScript probe typed-unavailable，`make check` 只具 source_static 能力，proof ledger 保持 `verification_proof_incomplete`，没有被静态通过洗绿。Runner 额外报 `durable_apply_ref_missing` 是 B731：Go nil `[]Change` 序列化为 `changes:null`，eval 仅把 `[]` 识别为 proof-only 零改动，漏用前序唯一 durable ref。修复只统一 typed collection 的零基数，不降低普通 mutation 缺 ref 的红线。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
