# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T05:11:58Z
- sweep_start_ts: 20260805-221156
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | hilog_cangjie_panic | PASS | eval/results/hilog_cangjie_panic-20260805-221158 | log_attachment,answer_contains | log_triage | 103s | 19 | read=0,repo_map=1,list=0,trace=0,source_lens=1 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass | Final answer contains the three real artifact-local frames and the observed `index=5,size=3` boundary, while explicitly retaining the external-source/version caveat. No `call_edge` enum rows, no system-authored variable identity, and no Finalizer reject. |
| 2 | github_issue_chrono_duration_min_symptom | PASS | eval/results/github_issue_chrono_duration_min_symptom-20260805-221158 | write_apply,answer_regex | none | 365s | 20 | read=12,repo_map=4,list=2,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Runner false green. The only executed command is `make check`; both Rust paths are typed `declared_project_check/source_static`, yet final proof is `strong/all_batches_verified`. Planner first emitted a valid JSON-array string carrier for `verification_probes`; strict decode rejected it, and the retry deleted all probes. Applied Rust was never compiled/executed and calls the associated function as bare `try_milliseconds(...)`, so correctness is unproven and likely uncompilable. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit findings

- `EVAL-B163-ADOCENUMTEXT1` is production-closed by this replay: recoverable claim metadata no longer becomes visible enum prose, and the model's complete answer remains intact.
- `EVAL-B164-STATICNOPROBE1` (P0, confirmed): target verification authority currently depends on an unavailable-probe witness. If a model omits probes after a carrier retry, the same source-static report can become `strong/all_verified`. Generic fix: every changed production path needs typed `target_execution` or `target_behavior`; source-static/syntax/unknown/uncovered can never independently authorize all-verified, regardless of probe presence.
- `EVAL-B164-PLANJSONARRAY1` (P1, confirmed): `verification_probes` arrived as a string containing a fully valid JSON array. The system can losslessly decode this carrier but currently rejects it; the retry omitted the probes. Add a bounded typed string-carrier repair with shape/schema validation and explicit repair telemetry. Bracket-imbalanced or ownership-ambiguous JSON remains fail-loud.
- Neither product case invokes Trace; explicit-window causal projection, auto-supplementation, root ranking, wakeup chains, window eliminable amount, two root-cause axes, and model conclusion ownership remain untouched.
