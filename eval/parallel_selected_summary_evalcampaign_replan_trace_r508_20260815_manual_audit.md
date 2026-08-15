# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T13:32:58Z
- sweep_start_ts: 20260815-063257
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | FAIL | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-063258 | write_apply,write_patch_oracle | none | 174s | 24 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail-system | Patch and behavior are correct: apply succeeded and both inline probes plus all four project tests passed (6/6). Final delivery was nevertheless downgraded to `impact_targets_unverified`: the changed-symbol obligation was verified, while two noisy `non_ascii_source_comment_added` advisories were minted as unknown semantic effect-followups that no test/verify-only replay can close. B832 is a terminal-authority gap, not a code/test failure. |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260815-063258 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 245s | 29 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | fail-system | B830 data path is production-positive: target `app-100` runnable 0.800ms is primary; `worker-200/VerifyClass` raw 5.000ms remains an on-chain business/optimization clue but publishes effective 0.000ms, rank 0 and no badge. The model still claimed the target slept waiting for VerifyClass completion, contradicting the typed caveat, because the generic Trace skill still taught every on-chain semantic row to compete equally. B831 aligns that soft teaching and the final typed handoff; no prose gate or system-authored conclusion. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. B830 passed its production data-layer acceptance. Explicit window, `Trace 因果投影`, automatic typed supplementation, raw semantic occupancy, business identity and zero effective attribution all survived; the deterministic projection did not promote the relation-only semantic row.
2. The Trace answer contradiction is system-induced rather than a single unexplained model fluctuation. A stale generic skill sentence granted all `chain_relevance=on_chain` semantic rows equal root-cause/mechanism authority even after B830 introduced the narrower typed basis. B831 removes that conflicting teaching and adds a typed-basis-only final synthesis boundary while leaving explanation, prioritization and conclusion ownership with the model.
3. The write patch is behaviorally verified. The actual report marks the changed path covered, the changed-symbol finding verified, and all six tests passed. The terminal false negative comes from noisy source-comment convention heuristics being represented as unknown semantic impact targets; repeating the same verifier cannot prove or disprove comment-language convention. This is B832 and must be fixed at authority classification, not by weakening project tests or string-matching the answer.
4. Both cases produced complete outputs. No fixed-age 4ms/4s/4m active-stream degradation, empty answer, stale-answer replacement or system-authored model conclusion was observed.
