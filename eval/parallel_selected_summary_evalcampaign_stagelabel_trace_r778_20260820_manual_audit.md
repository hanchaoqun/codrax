# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T15:27:14Z
- sweep_start_ts: 20260820-082713
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-082714 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 177s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 2.000–2.020s query window and Trace causal projection survived. The model identifies the proven on-chain threadpool-400 iowait seat as #1 at 11.000ms, keeps three mutually exclusive 1.000ms runnable seats under scheduling supply, and keeps the 16.000 IO activity index in background with no window/chain/effective wall-clock value. Adjacent sleep remains context. No old IO-pressure-score wording and no active-stream time degradation. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-082714 | answer_regex,answer_contains | none | 1136s | 72 | read=55,repo_map=3,list=0,trace=0,source_lens=1 | midloop=23,inv=9/0,fin_reject=15,unavail=0,prune=5 | partial | B1245 is visibly effective: all three stage-precedence arrows use the canonical business labels, never generic `随后进入`. The prose and table retain Orchestrator, runReadSchedulerLoop, dispatchStage, applyStageOutput, BusContext and each stage carrier. The final diagram, however, reached acceptance only on round 16 after 15 relation rejects and shrank the technical sequence to three surviving calls plus the stage spine. Initial and later model patches repeatedly introduced unsupported call/assignment/data-flow edges or retargeted endpoints; no identical must-emit/must-reject contract was found. The system gap is repair convergence: `preserve_unlisted_edges=true` is guidance only while `replace_blocks` permits an unrestricted whole-diagram rewrite, so each local repair can mint a new failure set and grow context from about 63k to 138k tokens. This is B1246/RELDELTA1, not a 4ms/4m fallback and not a reason for the system to author the graph. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
