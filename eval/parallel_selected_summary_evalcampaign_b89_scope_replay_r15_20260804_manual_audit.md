# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T00:59:58Z
- sweep_start_ts: 20260804-175957
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260804-175958 | answer_regex,answer_contains | none | 256s | 28 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=5/2,fin_reject=6,unavail=0,prune=0 | fail | Source truth is `buildAnalysisIR -> gate.RunWith` plus the separate wrapper edge `gate.Run -> RunWith`; there is no requested-direction path to `gate.Run`. Finalizer repeatedly rejected the qualified presentation `gate.Run -> gate.RunWith` as unproven while simultaneously requiring the same typed edge as bare `Run -> RunWith`, exhausted six retries, and shipped a degraded stale draft that still reverses the wrapper description. Confirmed `EVAL-B89-CALLEDGEQUAL1` P0 contract/identity gap, not model variance. |
| 2 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260804-175958 | answer_regex,answer_contains | none | 667s | 39 | read=2,repo_map=16,list=0,trace=0,source_lens=16 | midloop=20,inv=11/1,fin_reject=0,unavail=0,prune=0 | pass-with-process-gap | Final 3/5/30 roster and every cited source line are correct. Production replay disproves the first B88 scope carrier: the real analyzer kept `internal/analysis/criterion` in a verbatim-validated `SourceScopeProfile.SourceQuotes` lane, not `MentionedEntities`, so the carrier was never minted. Completion then demanded five unrelated repo scopes for 11 attempts and 16 source lenses. `EVAL-B88-SCOPEPROV1-R1` fixed by admitting only the already-validated typed source quote plus matching analyzer-stage lens; no RawRequest/final prose scan. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
