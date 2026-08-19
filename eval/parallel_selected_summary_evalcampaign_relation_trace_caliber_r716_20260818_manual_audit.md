# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T04:43:02Z
- sweep_start_ts: 20260818-214300
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260818-214302 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 296s | 43 | read=6,repo_map=0,list=1,trace=12,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | B1136 production-positive: blocked_reason census no longer becomes the source of Sleep. State totals and bounded CPU-policy conclusion are correct. New B1138/P1: the answer expands a complete zero `target_window_wait_occurrences` roster into “no wait/blocking”, although the same answer reports 70.338ms ordinary Sleep. This roster covers only D/io_wait and S rows carrying iowait=1; zero is not absence of all waits. B1139/P2: raw evidence enums/keys still leak into Chinese prose. B1140/P1: Analyzer needs three attempts to choose bounded_effect_verdict, then exploration repeats 12 trace queries/6 payload reads. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260818-214302 | answer_regex,answer_contains,mermaid_edge_count | none | 370s | 39 | read=14,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=7/0,fin_reject=2,unavail=0,prune=0 | pass | B1135 production-positive: Explorer iterations 43→28, reads 28→14, finalizer rejects 4→2 versus r715. The typed missing set shrinks once from `[Mutable BusContext]` to `[Mutable]`, so one reset is genuine proof progress. Final keeps the exact BusContext argument-flow edge and an honest unproven boundary for Mutable; the prior duplicate visible Mutable ghost is absent. Remaining relation-search/schema churn is an efficiency debt, not permission to weaken the evidence gate or synthesize an edge. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### qf_logic_view_read_pipeline

- The B1135 convergence key is production-positive. Parser navigation changed repeatedly, but only the typed missing participant set controls reset; the one reset in this run corresponds to a real proof-debt shrink.
- The final graph preserves the four-stage order, the evidence-backed `BusContext` argument-flow relation, and a visible disconnected `Mutable` whose incidence is explicitly unproven. It does not invent a bridge and does not let a system supplement replace the model-authored graph.
- Churn remains high (28 Explorer iterations, 14 source reads, two Finalizer rejects), but is materially lower than r715. Treat exact-schema/candidate presentation as later efficiency work; do not fit the validator to this graph.

### real_trace_h4_supply_thermal_witness

- B1136 is production-positive. The authoring context publishes `relation=unjoined_distinct_observation_domains`, no record-to-state mapping, and unproven state-source attribution. The final no longer attributes the 70.338ms Sleep bucket to a blocked_reason caller or census.
- The finite answer correctly reports Running 157.248ms, Runnable 5.604ms, Sleep 70.338ms, D/IO 0ms, and keeps the target effect of CPU policy limits unproven without target-running-slice binding.
- B1138/P1 is a deterministic semantic gap. `target_window_wait_occurrences=0` is an exact zero only for the narrow engine roster: D/io_wait plus S intervals with `iowait=1`. It excludes ordinary S and cannot establish “no waiting” or “no blocking”. The final makes exactly that invalid expansion while reporting 70.338ms Sleep on the same page.
- B1139/P2 remains visible: `coverage=complete`, `target_effect_unproven_no_slice_binding`, `authority=direct_in_window_policy_limit`, and `target_window_wait_occurrences` appear as customer prose. Fix through local typed-field descriptions and soft authoring guidance, never by scanning or rewriting the final answer.
- B1140/P1: the Analyzer first emits two incompatible causal-diagnosis shapes before selecting the valid finite bounded-effect shape. The schema is fail-loud as intended, but its decision teaching imposes unnecessary JSON mind and contributes to the 12 query / 6 payload-read exploration. Add a general typed decision table; do not route from request keywords.

### Invariants

- No full Trace causal projection is expected for this finite observed-facts plus one bounded target-effect query. This is not a missing-projection failure.
- Explicit root-cause/causal-diagnosis requests still retain deterministic supplement and typed on-chain-only root causes. Adjacent/background evidence remains support-only.
- No active-byte-stream age degradation, system-authored answer replacement, or model/final prose hard scan occurred in either run.
