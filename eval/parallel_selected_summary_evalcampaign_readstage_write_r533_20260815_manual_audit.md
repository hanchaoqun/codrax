# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T22:21:12Z
- sweep_start_ts: 20260815-152110
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260815-152112 | write_apply,write_patch_oracle,answer_contains | none | 91s | 24 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One-line Go typo was planned as `kind=patch`, applied only to `main.go`, and verified successfully; no read-stage or diagram regression crossed into write mode. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260815-152112 | answer_regex,answer_contains | none | 267s | 32 | read=7,repo_map=3,list=0,trace=0,source_lens=1 | midloop=9,inv=3/1,fin_reject=2,unavail=0,prune=0 | partial | B858 is production-positive: the final Mermaid contains all four adjacent Analyze→Explore→Extract→Finalize precedence edges and the table keeps the requested columns. The answer is still materially wrong because it inserts write-only `write_analyze` into the read stage table and claims `runAnalyzePhase → runWriteAnalyzePhase`. The finalizer prompt simultaneously carried the exact current-read four-stage authority and a generic first-pass/support relation floor containing `runWriteAnalyzePhase`; the latter competed with the request-scoped spine. Two diagram rejects followed before the model retained only the correct spine. Register B859: when a complete request-scoped typed spine exists, generic supporting relations must not be promoted into the first-pass or required-repair principal carrier; retain them only as bounded support. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Machine verdict: `2 PASS / 2`.
- Human verdict: `1 pass / 1 partial`.
- B858 is closed on production behavior: the missing full read-stage precedence relation is restored without a system-authored diagram.
- New generalized gap: `B859-REQUESTSPINECONTEXTPRECEDENCE1/P0`. A complete request-scoped typed relation spine and generic grounded sibling relations are both true, but the generic first-pass/repair carrier currently presents siblings as a diagram floor before the narrower principal role. In this run that elevated a write-only branch into a read-only explanation. The fix must consume typed `requestSpine` metadata only; it must not scan the request, draft, final prose, Mermaid labels, language names, or case identifiers.
- B857 validation waterfall did not reproduce: the second reject was introduced by the model's first patch (three extra diagram blocks plus a `data_flow` reply arrow), rather than hidden in the original draft. Keep it under observation.
- No empty answer, malformed-JSON salvage, old-draft fallback, Mermaid syntax degradation, or active-stream fixed-age degradation occurred.
