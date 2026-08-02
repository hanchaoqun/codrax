# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T00:14:40Z
- sweep_start_ts: 20260801-171439
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260801-171440 | write_apply,write_patch_oracle,answer_contains | none | 102s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Applied-tree diff contains exactly `retrun` → `return` in `main.go`; no unrelated production edit, controller reached verified, and the declared Go verification passed. Analyzer needed four emits to satisfy owner-qualified field-value shape, but this did not broaden the patch. |
| 1 | read_combo_analyze_retry_anchor | PASS | eval/results/read_combo_analyze_retry_anchor-20260801-171440 | answer_regex,answer_contains | none | 153s | 32 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B29-DOC1 materially improved: populated `Error`, nil IR, semantic degraded recovery, fallback builder and no write fallback are now correct. Residual factual errors remain: it falsely nests `runAnalyzePhase` under `runTaskGraph`; calls the additive retry formula a product; presents internal `max_retries_per_stage` carrier tag as the public runtime YAML key (actual override `pipeline_max_retries_per_stage`); and refers to nonexistent `QualityGate.Error` instead of checks/detail. Runner oracles do not cover these relations. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
