# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T02:50:48Z
- sweep_start_ts: 20260813-195047
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-195048 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 103s | 31 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B759 production-positive: stale model aggregate disappeared and target-binding conclusion is unproven. New exact prompt pollution remains: coverage exposes another thread's 36 fragments/33.322ms and an unrequested target blocked-reason census; final answer assigns both to the target and claims all 70.338ms S-sleep was fscache-driven. It also swaps CPU0/CPU4 limit-row counts. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-195048 | answer_regex,answer_contains,mermaid_edge_count | none | 252s | 37 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=5/0,fin_reject=2,unavail=0,prune=0 | partial | Final answer retains stage precedence and exact typed data-flow recipes; unproved BusContext/Mutable incidence remains visibly disconnected instead of fabricated. Runtime fell from 590s to 252s, but two legitimate relation repairs and five mid-loop steps remain. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. B759 closed the authority collision it targeted. The Finalizer prompt no longer carries the stale
   model-authored aggregate that called 558000kHz a maximum, and the answer correctly keeps policy-limit
   presence separate from unproved target binding. No root board, rank roster, full causal projection, malformed
   JSON recovery, stale-draft recovery, empty answer, or active-stream age downgrade appeared.
2. H4 still fails product correctness because the prompt's compact Trace Observation Coverage ranks
   `tui thread-13629` state churn beside the named target. The model copies that row's `fragments=36`,
   `max_segment=33.322ms`, and `p95=33.272ms` onto `.ugc.aweme.lite-17267`. This is deterministic
   cross-subject context contamination, not a value missing from the trace engine.
3. The finite typed profile requests target scheduler state, count/duration, and frequency residency; it does
   not request `recorded_reason`. Nevertheless `blocked_reason_census` remains in the final prompt. Its 50
   kernel-callsite records sum to only 16.358ms and carry no state-interval join, yet the answer upgrades them
   into the cause of all 70.338ms S-sleep. Existing generic caution text did not overcome the unnecessary
   high-salience row.
4. Frequency witnesses are correct in typed authority (CPU0=16 rows, CPU4=28 rows), but the answer swaps the
   counts. Presenting multiple CPUs in one semicolon chain needlessly raises copy-binding load; one witness per
   line is a generalized context-shape improvement, not an answer-text gate.
5. The answer correctly states the target also ran on CPU4 earlier, then reasons as if it ran only on CPU12.
   Exact slice/limit overlap remains unavailable, so `target binding unproven` is still the only supported
   condition-to-target verdict. Its subtraction of top-two running buckets from total running is also not an
   exhaustive roster claim because those rows explicitly say `ranked ... bucket (not a complete subject total)`.
6. QF preserves verified relations and refuses to invent missing high-level carrier incidence. Its automatic
   pass therefore remains human partial rather than a reason to weaken relation evidence gates. The substantial
   runtime/read reduction is positive but not yet a generalized closure of navigation churn.
