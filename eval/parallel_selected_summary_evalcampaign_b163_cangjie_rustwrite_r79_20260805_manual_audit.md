# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T04:38:17Z
- sweep_start_ts: 20260805-213812
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | hilog_cangjie_panic | PASS | eval/results/hilog_cangjie_panic-20260805-213817 | log_attachment,answer_contains | log_triage | 145s | 22 | read=4,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=3/2,fin_reject=0,unavail=0,prune=0 | fail | Log triage and analyzer emitted valid native JSON on their first calls. Runtime facts and the source-version caveat are correct, but the finalizer put `claim_form` inside three `items[]` objects without item text. The compatibility alias normalizer laundered the enums into visible rows, yielding `1. call_edge` three times while the runner still passed. |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | eval/results/github_issue_chrono_duration_min_symptom-20260805-213817 | write_apply,answer_regex | none | 532s | 20 | read=15,repo_map=3,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Applied Rust patch rejects every negative millisecond value because `%` is negative and `nanos < 0` returns `None`, contradicting the required `-i64::MAX` acceptance. The Go probe only prints simulated success and does not execute Rust; `make check` is a Python source-static oracle. The proof ledger correctly leaves three behavior contracts uncovered and finalizes unverified, but the terminal verify-only batch cannot route the exact gap into a safe next proof/repair generation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit findings

- `EVAL-B163-ADOCENUMTEXT1` (P1, confirmed): answer-document item alias recovery treated the schema-reserved enum field `claim_form` as arbitrary visible prose. Generic fix: front-load the item-vs-claim carrier decision, move valid misplaced claim metadata to block-level `claim_uses[]`, and drop only metadata-only shells when complete block prose already preserves the answer. Never synthesize item prose from an enum.
- `EVAL-B163-RUSTPROOF1` (P1, confirmed): a cross-language standalone probe can simulate the desired behavior without executing changed production code. Current path-level capability/proof ledger correctly prevents a verified verdict, so this is not a false-green terminal bug. Remaining work is to keep a proof-incomplete follow-up resumable without treating missing proof as production-code failure.
- Neither case uses Trace. Explicit-window Trace causal projection, automatic supplementation, root ranking, wakeup chains, window eliminable amount, and model conclusion ownership were not touched.
