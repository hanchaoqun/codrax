# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T14:46:43Z
- sweep_start_ts: 20260814-074642
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_perf_quality_raw_fallback | PASS | eval/results/trace_query_perf_quality_raw_fallback-20260814-074643 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 126s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B801 closed: no uncovered-tail runnable/trace_gap, CPU=5 and TGID=20 stayed distinct, and no cycles-to-time conversion. New B805: one sample was promoted to a hotspot and `1 sample / 8ms` was dimensionally misreported as 12.5% profiler coverage despite the typed density caveat. |
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260814-074643 | log_attachment,answer_contains | log_triage | 130s | 25 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B802 projection worked: unsupported relationship aggregate was absent and final context carried cross_error_relation=unproven. Analyzer observation_summary and explorer completion reason still asserted caller/callee propagation; answer schema had no typed runtime-relation assertion, so finalizer repeated an unproved causal chain. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings and disposition

1. `B801-TRACETAIL1` is production-closed in this replay. The only scheduler value is the physically measured 3.000..3.008 Running interval. The requested 3.010 endpoint does not mint runnable latency, a trace gap, or a root-cause seat. Explicit-window Trace projection and automatic supplementation remain available; this bounded sampling-granularity question correctly did not need a causal projection block.
2. `B803-TRACECPUROLE1` did not recur: the answer uses bracket CPU 5 and never treats parenthesized TGID 20 as CPU/migration. Keep the typed header-role guidance and matrix pins; do not add prose scanning.
3. `B805-PERFSAMPLECALIBER1` (P1, confirmed): `sample_count=1`, `running=8ms`, and `weight=9000 cycles` are separate dimensions. No sampling period/frequency/duty receipt exists, so neither statistical hotspot rank nor temporal profiler coverage percentage is provable. The best generalized remedy is typed sample-statistical-caliber context (observed sample vs comparative hotspot; coverage_fraction unavailable) plus soft finalizer guidance. The answer text remains model-owned.
4. `B802-LOGPEERCTX1` is only partial. The new authority projection removed the unsupported behavior/error-granularity aggregate from both SurfacePlan and ObservationLedger, while raw mutable audit history remained intact. The remaining contradiction is not that projection: analyzer `observation_summary` and explorer completion `reason` are model-authored planning summaries that still assert propagation, while the same finalizer context carries two principal peer occurrences and `log:cross_error_relation=unproven`.
5. `B806-RUNTIMERELATIONASSERT1` (P0, confirmed): the final answer schema can label a block `external_observation` without declaring which typed relation authority supports its caller/callee/propagation conclusion. A generalized fix needs a structural runtime-relation assertion/status bound to the precise LogBundle relation carrier; peer-only bundles can require `unproven`, explicit recursive CauseRelation can admit `explicit_artifact_marker`. The validator must inspect typed fields only, never scan user/model/final prose, and must ask the model to revise rather than replace its answer.
6. No malformed JSON, empty answer, stale-draft fallback, Mermaid failure, system-authored answer rewrite, or active-stream fixed-age degradation occurred. Machine PASS therefore remains distinct from human correctness.
