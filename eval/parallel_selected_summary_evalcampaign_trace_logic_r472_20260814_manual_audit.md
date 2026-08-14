# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T07:20:28Z
- sweep_start_ts: 20260814-002026
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260814-002028 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 136s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B770 production-positive: the exact finite target-state authority reached Finalizer and the first accepted draft preserved Running/Runnable/Sleep/D/IO separately. Frequency binding remained explicitly unproven. |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260814-002028 | answer_regex,answer_contains,mermaid_edge_count | none | 248s | 40 | read=12,repo_map=4,list=0,trace=0,source_lens=0 | midloop=8,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | One rejection instead of six and no relation was synthesized, but Analyzer emitted an explicitly empty participant slate; the accepted patch therefore erased requested Mutable/BusContext from the diagram while prose retained the claims. This is an upstream typed-contract gap, not B771 endpoint-location failure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

### 1. H4: B770 closes the finite-state authority materialization gap

- The Finalizer prompt now contains `Trace Target-State Scope Authority` for the exact artifact,
  target and selected window: Running=157.248ms, Runnable=5.604ms, Sleep=70.338ms,
  D-state=0.000ms, IO-wait=0.000ms, Total=233.190ms, complete partition and zero
  unaccounted time.
- The model's first draft copied all five state rows separately and the eight-CPU running
  roster sums exactly to 157.248ms. No subtraction reconstructed Sleep and no D/IO state was
  renamed after a blocked-reason family. Finalization had zero rejects.
- CPU0/CPU4 policy ceilings remain CPU-owned observations. The answer says target binding is
  unproven and does not claim CPU12 was limited. One sentence still calls 2075MHz a
  thread-level observed frequency before immediately limiting its authority; this is display
  imprecision, not a wrong constrained/unconstrained verdict and should not trigger a
  case-specific rewrite.
- This bounded fact/effect question correctly has no root-cause board or Trace causal
  projection. The ledger fallback materialized values only; it did not create a causal chain,
  rank or conclusion.

### 2. QF: fewer retries, but the typed participant roster disappeared upstream

- Finalizer validation dropped from six rejects in r471 to one in r472 and runtime fell from
  488s to 248s. The first draft invented unsupported assignment/call/containment arrows and
  was correctly rejected; the patch retained only three proved stage-precedence edges and one
  proved call. No system-generated bridge or semantic edge appeared.
- B771's label-aware collision tuple was not exercised in this replay. The Analyzer had
  emitted `diagram_hint.participants=[]` even though its independently validated required
  diagram dimension copied the exact clause naming analyzer/explorer/extractor/finalizer and
  Mutable/BusContext. With no typed participant obligations, the patch could lawfully remove
  both carriers. The final prose still claims shared state flow, while the final diagram omits
  both requested carriers.
- Register `B772-EMPTYDIAGRAMPARTICIPANTCROSSFIELD1/P1`. The generalized fix must not parse
  the raw request or author participants. For a non-Trace required diagram, when the
  schema-validated required diagram dimension's verbatim source quote co-lists at least two
  analyzer-declared entities while the participant slate is explicitly empty, fail the
  AnalysisIR as internally inconsistent. The Analyzer must add typed participant rows or
  narrow the source quote; it owns roles and relation scope.
- The threshold stays at two so one named enclosing system/scope cannot become an incident
  participant through a hard guess. Trace is excluded so explicit-window projection and
  automatic causal supplementation remain unchanged.

### 3. Recovery and active-stream checks

- Neither case used malformed-JSON answer recovery, old-draft answer degradation, or an empty
  answer. QF needed no Mermaid source syntax repair in the accepted draft.
- Both active streams ran for 136s/248s and completed normally. There was no age-only 4ms
  degradation; active bytes remain authoritative progress and do not permit fallback.
