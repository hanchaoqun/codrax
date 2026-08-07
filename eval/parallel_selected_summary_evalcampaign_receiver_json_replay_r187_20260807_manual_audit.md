# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T21:14:46Z
- sweep_start_ts: 20260807-141443
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260807-141446 | answer_regex,answer_contains | none | 174s | 22 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | S37at production-positive: typed graph has 5 relations, 6 nodes, 1 weak component; final answer preserves `run -> fetchUser -> send -> dispatchOnce -> fetch` and the `@app/core -> tsconfig.base.json -> packages/core/src/index.ts` resolution. Analyzer nevertheless submitted the documented `source + discover + empty sink` shape twice and received the same generic rejection because the source was a pre-scan file path rather than a request-proven code identity; this is a typed normalization/error-contract gap, not prose fluctuation. Finalizer then mislabeled the real conditional invocation `send -> retry.nextDelay` as `guard`; the precise call/guard teaching was present, the validator correctly rejected it, and patch removed only the optional diagram. The remaining POST/TCP overclaim is model adherence fluctuation. S37au did not fire because the accepted payload was valid native JSON. |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260807-141446 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 301s | 44 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | Explicit 114.94ms window remained on all 6 trace queries. Model lead covers both dimensions: actual occupancy (running/runnable/sleep/D/io) and typed eliminable candidates; it ranks CookieMonsterCl/NetworkService/ThreadPoolForeg/supply deficit, carries the 4-node wakeup path and representative windows, and discloses frame evidence absent. System Trace causal projection and supplementation remained present and did not replace model blocks. Finalizer first misplaced `trace_causal_claim_caliber` on a table despite precise placement teaching; retry fixed it, so this is model compliance fluctuation. New generalized presentation gap: 8 model blocks became 20 visible blocks and 141224 bytes; 51 E# rows plus full per-node detail duplicate named-artifact and staged-attachment observations, drowning the model conclusion. Fix must compact typed duplicate provenance/visible audit detail while preserving model blocks, projection, full internal ledger, explicit completeness and automatic supplementation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2 PASS; human: partial + partial.
- `EVAL-B312`: production-positive and closed.
- `EVAL-B313`: implementation-closed; the malformed-array repair was not exercised in this replay.
- `EVAL-B314-TRACEPROJECTIONVISIBLEBUDGET1`: P1 confirmed. This is system presentation amplification, not model-answer ownership replacement.
- `EVAL-B315-CALLCHAINDISCOVERDEMOTION1`: P1 confirmed. A discover source that is not an exact request-proven code identity must deterministically demote to authority-free `discover_path`; the error must not advertise the rejected shape as valid.
- Trace explicit-window causal projection, automatic supplementation, root ranking, wakeup chain, window eliminable amount and dual root-cause dimensions: preserved.
