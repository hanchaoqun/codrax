# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T03:09:31Z
- sweep_start_ts: 20260821-200930
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260821-200931 | write_apply,write_patch_oracle,answer_contains | none | 124s | 26 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exact one-line `retrun buf;` → `return buf;`; `make test` passed and the untracked compiler output was disclosed. The analyzer softened both behavior contracts to planning-only, so this run did not exercise S1319 `source_contract_refs`. |
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260821-200931 | answer_regex | none | 170s | 27 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Six grounded call edges and walker responsibility survived, but the answer falsely says collection and processing advance in parallel although source control flow is sequential. Analyzer also typed the path dimension as `member_set` despite typed non-enumeration signals, producing a contradictory forced patch that tried to combine roster and path ownership. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit findings

- `B1319`: no production authority witness in this sweep; unit/full-suite closure remains valid, replay remains pending.
- `B1320`: canonical `replace_blocks` teaching path converged in one accepted patch; the misroute compatibility arm was not exercised in production.
- `B1321/P1`: add a typed relation-path requested dimension and reject analyzer IR that labels a call path as a member roster while all typed enumeration/completeness predicates are false.
- `B1322/P2-observe`: one unsupported parallelism claim; keep as model-variance observation unless repeated, and do not add a raw answer-prose keyword gate.
