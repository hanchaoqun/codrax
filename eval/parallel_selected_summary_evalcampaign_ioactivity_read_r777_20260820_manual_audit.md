# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T15:05:23Z
- sweep_start_ts: 20260820-080521
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-080523 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 153s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 2.000–2.020s window and Trace causal projection are intact. On-chain root cause remains threadpool-400 iowait 11.000ms; three runnable 1.000ms seats remain scheduling-supply candidates. The 16.000 IO activity index is background-only, non-wall-clock, with projection/cumulative/effective columns empty. No old IO-pressure/composite-score wording and no active-stream degradation. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-080523 | answer_regex,answer_contains | none | 654s | 46 | read=74,repo_map=5,list=0,trace=0,source_lens=0 | midloop=41,inv=17/0,fin_reject=3,unavail=1,prune=0 | partial | Narrative and four-column stage table are useful, but the final sequence diagram shrank from the model's detailed first draft to only three generic “随后进入” edges. One reject was malformed JSON and another had model-authored endpoint errors; no identical must-emit/must-reject conflict was found. A system context gap remains: canonical stage recipes publish business transition labels while participant candidates publish generic labels for the same typed edges. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
