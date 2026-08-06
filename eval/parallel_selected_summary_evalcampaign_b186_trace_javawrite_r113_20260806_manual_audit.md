# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T17:38:50Z
- sweep_start_ts: 20260806-103849
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-103850 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 202s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=3/2,fin_reject=0,unavail=0,prune=0 | fail | Explicit-window projection, auto-supplement, wakeup path, rank, representative windows and both axes are all present. The model nevertheless promotes `udk-irq-3-65/io_latency` to the target main thread's “direct blocker”, while the typed final boundary says `target_direct_blocking_authority=not_provided_by_projection` and the row belongs to ThreadPoolForeg. This is a material relation overclaim, not a missing projection. Closure also burns two retries because trace teaching places non-schema `member_values/comparison_value` beside the optional `relation_claims` copy surface; one retry string-wraps aggregate_facts and copies those fields, proving a system-authored JSON-teaching contradiction. |
| 2 | github_issue_commons_lang_random_ascii | FAIL | eval/results/github_issue_commons_lang_random_ascii-20260806-103850 | write_apply,answer_regex | none | 268s | 20 | read=10,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | pass | The cumulative applied tree has the generalized `end <= 0x7f` production guard plus CJK/full-width digit regressions, and `make check` passes. The first test failure correctly replans only the bad test values. Final `unverified/production_verification_source_static_only` is honest: the fixture's Make target is a Python source-shape oracle and does not execute Java behavior, so the controller correctly refuses `all_verified`; this is not code failure or a missing answer. No malformed answer JSON/finalizer retry occurred in this write lane. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch finding

- `EVAL-B186-RELJSON1=P1/confirmed`: the relation authority preview mixed model-copyable claim fields with diagnostic-only calibration fields. The model copied the visually adjacent superset and strict decoding rejected it twice. General fix: render a canonical `relation_claim_copy={...}` containing only schema fields, move calibration to a separate `relation_diagnostic_only` line, and teach that the optional claim is normally omitted because typed authority is already carried.
- `EVAL-B186-DIRECTREL1=P1/confirmed`: a wakeup-chain/peer blocking row was promoted to the selected target's direct blocker despite an exact typed negative authority. General soft-context fix: publish one high-salience `direct_blocking_decision` from typed projection state and explicitly distinguish target waiter/holder proof from wakeup peers, IRQ peers, callers, adjacent rows, and another thread's blocking interval. No prose scanner or hard answer gate is added.
- Trace preservation check: explicit window `34579.472865..34579.587805`, `Trace 因果投影`, deterministic frame bundle supplement, target state partition, root-cause ranks, wakeup chain, actual-occupancy table, and existing-rule eliminable overview all remain present. The system did not replace the model's diagnosis; the problem is that the model did not obey an already-present but low-salience typed relation boundary.
- JSON policy check: the JSON-encoded `aggregate_facts` array was re-parsed only because the full array was losslessly recoverable. Unknown `member_values` was not guessed or silently dropped. This matches the policy: lossless repair is allowed; lossy repair must retry or ultimately salvage visible strings with an explicit model-output degradation disclosure.
