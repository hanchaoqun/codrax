# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T22:54:52Z
- sweep_start_ts: 20260805-155451
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260805-155452 | log_regex,answer_regex | none | 43s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=1,repair=1,prior_errors=1 | pass | Real replay closes the r57 failure: one custom transform consumes both required materials and directly emits `{"ids":["u1","u3"]}`; there are no rule, decision, entity, contribution, reconcile, or assemble ranks, no terminal contest, and the published bytes are valid JSON only. The one repair is the planner's first omission of `instructions.md` from action consumption; the deterministic required-material check gives a precise same-direction correction, so this remains recoverable model carrier variance rather than a contradictory contract. Runtime falls from 355s/11 rounds to 43s/1 round. |
| 2 | sr_cpp_sink_impls | FAIL | eval/results/sr_cpp_sink_impls-20260805-155452 | typed_inventory_rowset,answer_regex,answer_contains | none | 118s | 20 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Production answer is complete and grounded: it lists ConsoleSink, FileSink, and RotatingSink, cites all three definitions, and preserves the indirect `Sink -> FileSink -> RotatingSink` chain. Runner failure is an eval boundary error: `EXPECT_INVENTORY_ROWS` requires member and filename in the same principal row although the original question did not request a file column. Separately, the soft cardinality checker falsely reads member-local `two levels`/`one file` as aggregate counts after a complete three-row table; fix by disabling member-only count binding once the structured roster is complete while retaining explicit aggregate-label count checks. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
