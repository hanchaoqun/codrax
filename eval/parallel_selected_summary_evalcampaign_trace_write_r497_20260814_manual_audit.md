# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T16:09:51Z
- sweep_start_ts: 20260814-090950
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260814-090951 | write_apply,write_patch_oracle,answer_contains | none | 94s | 24 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan contains one `kind=patch` hunk changing only `main.c:19` from `retrun buf;` to `return buf;`. Apply commit contains exactly +1/-1. `make test` compiled with `cc -Wall -O0`, ran both binary invocations, and returned PASS; changed-path verification is `project_runner/target_behavior`. New watch item B813: the test left an untracked `main` binary in the retained isolated worktree while final status still says fully verified. The binary is absent from the applied commit/ref, so this is disclosure/next-batch cleanliness debt rather than an incorrect delivered patch. |
| 1 | trace_query_perf_quality_raw_fallback | PASS | eval/results/trace_query_perf_quality_raw_fallback-20260814-090951 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 126s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B811 production-positive: the first completion succeeds with no source-flow repair/caveat. Final answer remains calibrated and reports only CPU 5. B812 is partial: perf triage and analyzer correctly consume the shared guide, but explorer's first pre-query thought still once says raw-21 ran on CPU 20; typed trace_query immediately corrects it and every closure/final surface stays CPU 5. Treat as model-salience watch, not authority for a prose hard gate or answer rewrite. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

- B811 closed the generalized contract-routing failure: finite runtime facts completed in one attempt, with no external-only waiver and no irrelevant current-source producer/transfer/consumer debt.
- The CPU identity guide is present in perf-triager, analyzer, and explorer prompts. It eliminated the analyzer error and all user-facing errors, but did not deterministically control one explorer pre-query thought. Since trace_query's typed CPU roster corrected the model before closure, further hardening from this single thought would be overfit; continue heterogeneous observation.
- The Trace answer still receives a verbose system supplement containing raw English `perf_quality` keys. It does not alter the conclusion but remains recurring customer-language/internal-term presentation debt.
- Write-mode verification has an untracked-output seam: `make test` generated `main` after the applied commit checkpoint. No source drift occurred and the recovery ref is clean, but future batches/status output should distinguish a clean applied commit from a dirty retained worktree and disclose typed verification side effects. Automatic deletion needs a narrow safe policy; arbitrary untracked customer files must not be removed.
- Neither case used malformed JSON salvage, prior-draft recovery, missing-answer fallback, finalizer retry, or system-authored conclusion replacement. No fixed short-age/4ms active-stream degradation occurred.
