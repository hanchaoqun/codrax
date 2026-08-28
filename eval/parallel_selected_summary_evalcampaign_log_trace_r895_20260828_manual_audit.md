# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T17:39:51Z
- sweep_start_ts: 20260828-103951
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | log_path_question_multi_runtime_files | FAIL | eval/results/log_path_question_multi_runtime_files-20260828-103951 | answer_regex,answer_contains | none | 130s | 26 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | The model's first final draft named both failures and the key frames. After a typed dimension retry, the accepted patch moved the frame data into `section.items[].cells`; the dimension checker counted those cells, but the renderer only emitted Label/Text, so every frame disappeared from the user answer and the legacy regex correctly failed. The prompt also exposed a separate authority gap: runtime aggregate rows with a valid log coordinate were promoted wholesale to independently proven even though their member notes still contained model-authored caller/nil mechanisms, while the producer-owned `read_file` byte observation had no direct ledger row. These are generic schema/render and provenance-granularity defects, not a reason to scan or rewrite model prose. |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-103951 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 191s | 42 | read=0,repo_map=0,list=0,trace=11,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | The explicit 34579.472865..34579.587805 frame window, five typed trace dimensions, four-hop ThreadPoolForeg→NetworkService→CookieMonsterCl→target dependency, on-chain priority/scheduler, D/IO, supply, deterministic VerifyClass clue, actual-time versus rule-eliminable ledgers, business identities, adjacent/background separation, representative windows, and full Trace causal projection are all present. The two r894 authority overclaims (target repeatedly woken but unable to schedule; ~26ms called close to full utilization) are absent. The model supplies the ranking and repair directions; the system does not author or replace the conclusion. One softer upstream-sleep interpretation remains suitable for continued observation, not a text gate. No fixed 4ms/4m/stream-age downgrade occurred. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
