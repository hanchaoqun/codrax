# Selected Eval Manual Audit

- date: 2026-08-28T18:03:15Z
- sweep_start_ts: 20260828-110313
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- binary revision: `81945fad2201`
- results_root: eval/results

The runner records declared oracle surfaces; human correctness below also audits prompts, typed authority, and final user-visible answers.

| # | case | runner | result_dir | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|--------|------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | log_path_question_multi_runtime_files | FAIL | eval/results/log_path_question_multi_runtime_files-20260828-110315 | none | 130s | 26 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass-with-model-caveat | B1391 is production-positive. The finalizer ledger contains two producer-stamped `read_file` direct observations (both complete 1..8 line spans); the runtime aggregate/closure prose is no longer elevated or repeated. The final answer visibly preserves both error messages, all four key frames and locations, and direct observed trigger sites. Runner FAIL is a measurement false negative: `EXPECT_MATCHES_REGEX` is line-oriented while each error and frame are intentionally in separate paragraphs/list rows. The case is moved to the existing folded `EXPECT_MATCHES_TEXT_REGEX` surface; production validation is untouched. The model still adds receiver/uninitialized-field and downstream/network/timeout-setting hypotheses despite the same generic evidence-caliber guidance being present in every explorer/finalizer prompt. This is model non-compliance, not missing context or a system contradiction; leave it as a soft-observation item instead of adding a prose keyword gate or system rewrite. |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-110315 | perf_triage+trace_query | 137s | 42 | read=0,repo_map=0,list=0,trace=11,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | Full non-regression: explicit 34579.472865..34579.587805 frame window, five typed dimensions, ThreadPoolForeg→NetworkService→CookieMonsterCl→target dependency, on-chain priority/scheduler, D/IO, compute supply, VerifyClass deterministic business clue, actual-time versus rule-eliminable ledgers, business identities, representative windows, adjacent/background separation, and complete Trace causal projection all remain visible. The model keeps the first ranked on-chain candidate and repair directions; system supplements remain evidence/checking material and do not replace model conclusions. No fixed 4ms/4m/stream-age downgrade occurred. |

## Decision

- `B1391`: production-positive/core-closed.
- `B1392-EVALMULTILINECOOCCURRENCE1`: confirmed and fixed by selecting the already-supported folded-text oracle for a cross-paragraph co-occurrence contract.
- Runtime mechanism overstatement: continue as model-variance observation. The context already carries precise producer authority plus generic “observation does not prove mechanism” guidance four times; no hard gate, keyword scan, retry, or answer mutation is authorized.
