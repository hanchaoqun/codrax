# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T13:03:59Z
- sweep_start_ts: 20260807-060358
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260807-060359 | answer_regex,answer_contains | none | 126s | 30 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Native JSON remained valid, the requested table and Mermaid both shipped, and there was no finalize retry. The two Name literals are correct. However the operational conclusion flattens the source contract: full emit remains valid for a complete rewrite, while patch is preferred only for a small retry edit; the answer repeatedly presents first-vs-retry as an unconditional binary switch. It cites only the Name definitions, not the Description/scheduler evidence for that timing claim. The mixed table carrier also renders the second tool with an empty first column. Runner PASS is therefore a false green for the requested finalizer relationship. File `EVAL-B272-COMPROLECALIBER1` P1; the blank mixed-label row is a presentation observation under the same item. |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-060359 | answer_regex,answer_contains | none | 322s | 29 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=7/1,fin_reject=2,unavail=0,prune=0 | fail | The analyzer emitted exact `buildAnalysisIR -> gate.Run` on its first successful call, so S37t was not exercised but the production outcome is positive: Explorer and final prose/diagram correctly preserve `buildAnalysisIR -> gate.RunWith`, reverse wrapper `gate.Run -> RunWith`, and `no_directed_path`. Two new gaps remain. First, exact endpoint existence does not qualify the grounded bare definition `Run @ gate.go:134` as typed `gate.Run`; the model needed a crossfile-exists workaround and 22 Explore iterations (`EVAL-B270-QUALIFIEDDEFENDPOINT1`). Second, all nine hop citations were shifted by one; the pre-emit checker computed every exact replacement but treated the mismatch as a soft advisory, so the accepted answer visibly cites the next hop (`EVAL-B271-UNIQUECITEREBIND1`). The answer also still omits required `analyzerGraphForNormalize`, so B262 stays open. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
