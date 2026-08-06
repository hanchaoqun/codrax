# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T07:43:40Z
- sweep_start_ts: 20260806-004338
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dayjs_duration_nan | FAIL | eval/results/github_issue_dayjs_duration_nan-20260806-004340 | write_apply,answer_regex | none | 159s | 20 | read=6,repo_map=2,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Patch is correct and narrow (`Number(value || 0)`), and the repository declares both `make check` and `node tests/duration.test.js`. Verification ran only `make check`; its Python checker supplied `source_static` authority, while the same-root Node behavior candidate was discarded by directory-only queue deduplication. The controller correctly refused `all_verified` with `production_verification_source_static_only`; no false green, but an available behavior test was never run. No JSON recovery/repair event occurred. |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260806-004340 | typed_inventory_rowset,dimension_substring,answer_contains | none | 268s | 28 | read=11,repo_map=2,list=1,trace=0,source_lens=1 | midloop=5,inv=3/1,fin_reject=0,unavail=0,prune=0 | fail | Analyzer correctly carried exact construct phrases (`extend 块`, `foreign func 声明`, `public class`) and completeness, but confidence=0.75 disabled both completion authority and the safe lens-first scheduler. Generic exploration then widened `public class` to all 14 public class/struct/interface/enum declarations instead of the exact 8 classes, and the finalizer fabricated several package names despite citations containing the declarations. Existing parser-backed SurfaceTerms already distinguish these constructs; the generic gap is coupling read-only navigation eligibility to completion authority. No aggregate-string or answer-block recovery event occurred, validating the native JSON-array path. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
