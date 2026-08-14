# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T03:26:40Z
- sweep_start_ts: 20260813-202639
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-202641 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 125s | 34 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B760 production-positive on visible answer: no cross-thread 36-fragment values, no fscache causal upgrade, no CPU witness count swap, exact four-state account and unproven target binding retained. Auto FAIL is now oracle-only: it bans the explicitly qualified sum of two visible non-exhaustive top buckets and requires a narrow phrase-order regex. A secondary prompt bypass still exposed unrequested wait census and off-target cpuset rows, though the model did not consume them. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-202641 | answer_regex,answer_contains,mermaid_edge_count | none | 272s | 35 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | One legitimate diagram repair removed fabricated Run/BusContext/Mutable arrows. Final graph retains the three precedence edges and one exact call edge, with BusContext/Mutable visibly disconnected and unproven. Prose still states a broader shared-state flow than the accepted relation graph proves, so the requested carrier flow remains incomplete. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. B760's principal production goal is positive. H4 no longer copies another thread's state-churn values, no
   longer mentions the unrequested fscache census, and preserves CPU0=16 / CPU4=28 in the typed witness feed.
   The visible answer gives running=157.248ms, runnable=5.604ms, sleep=70.338ms, D/io=0 and concludes that
   policy ceilings existed but binding to the target remains unproved.
2. The automatic failure is no longer a product-correctness proxy. The answer explicitly labels 96.081ms and
   35.960ms as two visible ranked buckets, keeps 157.248ms as the authoritative complete running total, and says
   the remainder is outside the visible top buckets. The case nevertheless bans their arithmetic subtotal
   `132.041`; its second regex also misses a semantically equivalent unproved-binding sentence because of
   phrase distance/order. Do not alter the answer architecture merely to fit either prose oracle.
3. A genuine context bypass remains despite the correct answer. `Runtime Trace Kernel Wait Call-Site & Wakeup
   Evidence` is compiled before the Finalizer Observation Ledger projection and still exposes the target's
   blocked-reason census plus unrelated threads. Its finalizer scoping predicate treats
   `target_wait_occurrences` or `count_or_duration` alone as permission, contradicting the existing typed
   `recorded_reason AND count_or_duration` contract.
4. The global frequency allow-list also admits `cpu_constraint`, which is a target-bound next_info/cpuset fact,
   not a global frequency-policy row. The coverage top five therefore become unrelated cpuset-constrained
   threads. This did not change this answer, but it proves prompt projection must cover all dynamic faces and
   preserve predicate caliber.
5. B760b fixes both roots structurally: bounded Finalizer wait/wakeup evidence keeps only typed subject/object
   relations incident to the user target (plus evidence boundaries); exploration and causal diagnosis keep the
   full multi-subject ledger. Unbound blocked-reason inventory now uses exactly
   `RequestsBlockedReasonCensus()`. `cpu_constraint` is removed from global frequency facts and remains available
   only when its subject is the requested target.
6. Analyzer again emitted `bounded_fact_set` and omitted the required `requested_answer_dimensions` object even
   though “频率有没有受到限制” is a finite target-effect verdict. The function schema declares that object
   required, but execution accepts nil for compatibility. The final result stayed correct through independent
   typed binding authority; the schema/executor admission mismatch is recorded as B761 for cross-case treatment,
   not repaired by scanning this request.
7. QF needed one valid correction rather than repeated contradictory retries. The initial graph invented several
   call/assignment arrows; the patch retained only typed precedence/call edges and disconnected BusContext and
   Mutable. The prose still overstates that those carriers pass all stage data, so human correctness remains
   partial and the system must gather real incident operations rather than manufacture the missing bridges.
8. Neither case used malformed-JSON recovery, stale-draft fallback, empty-answer recovery, or an active-byte age
   downgrade. Active output past 4ms remains a live attempt, not permission to return a degraded answer.
