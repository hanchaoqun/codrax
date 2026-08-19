# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T02:21:13Z
- sweep_start_ts: 20260818-192111
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260818-192113 | log_regex,write_apply,answer_regex,answer_contains | none | 903s | 26 | read=12,repo_map=2,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B1121 preserved the intentional regression oracle and B1126 guidance reached the planner, but the planner omitted `end_line` for a multiline `old_text` twice. The safe compiler kept the one-line default and rejected the mismatch; the 900s global write wall expired before an accepted plan, so no apply or verify occurred. |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260818-192113 | answer_regex,answer_contains,mermaid_edge_count | none | 1000s | 91 | read=15,repo_map=4,list=0,trace=0,source_lens=1 | midloop=23,inv=6/0,fin_reject=20,unavail=0,prune=1 | fail | B1127 stopped the exhausted-tool loop and preserved accumulated evidence. Finalization then repeated an exact one-sided identity omission: every anchor copied `to_identity` but omitted `from_identity`; the typed recipe receipt existed, yet recovery treated all one-sided rows as conflicts. Twenty rejects ended in degraded draft recovery. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### Read mode: deterministic one-sided typed-recipe recovery gap

- The finalizer received a schema-native candidate for
  `analyzerEvaluator.BuildInitialInstruction -> ctx.Mutable.SetPrescanRoundLimit`, including both exact identities,
  the `call` relation, and the instruction to use the broader visible `Mutable` participant only as the target node.
- The model-authored visible edge and direction were correct. Its anchor repeatedly carried the exact
  `to_identity=ctx.Mutable.SetPrescanRoundLimit` but omitted `from_identity`.
- `normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes` can restore a pair only when both identities are absent.
  It rejects every one-sided row before consulting the dispatch-scoped typed receipt, even when the populated side
  selects exactly one receipt. The relation validator therefore falls back to the visible `Mutable` identity and
  rejects the same recipe that the participant repair lane told the model to use.
- This is not model-only fluctuation. The model made a schema omission, but the system had a precise, unique typed
  recovery signal and failed to use it. The generalized fix is to complete only the missing side when the supplied
  side exactly matches one and only one receipt with the same relation. Ambiguous or conflicting rows must remain
  fail-closed. The system must not add an edge, change direction, rewrite labels, or select a relation.
- B1125 stayed effective: the earlier unique one-sided Mermaid node-reference drift did not recur. The surviving
  lesion is endpoint identity metadata, not node topology.

### Write mode: safe compiler, incomplete repair instruction, and budget pressure

- The write analyzer streamed actively for about 11m17s and produced roughly 150k assistant-content bytes. It was
  not degraded because of 4ms, a fixed response age, or continued byte activity. This preserves the active-stream
  red line.
- The planner correctly retained the regression oracle but did not follow B1126's boundary-partition guidance in
  its proposed probes. This is a production non-adherence witness, not authority to introduce a test-name or
  newline-count hard gate.
- The first plan was malformed; a later plan used multiline `old_text` starting at line 24 without `end_line`.
  The structured editor correctly defaulted the omitted value to the single start line and refused to widen it.
  Automatic widening would be unsafe and remains forbidden.
- The repair pack should, when the submitted multiline bytes match one unique contiguous range beginning at the
  declared start, report the exact required `end_line` as a typed correction. It must still leave the corrected
  plan to the model and keep ambiguous/stale bytes fail-closed.
- The global 900s write wall expired during plan repair. Long active intermediate output can consume the time
  reserved for plan/apply/verify. This is a separate scheduling/budget-allocation gap; it must not be "fixed" by a
  fixed-age stream kill. A later design must use typed remaining run budget or stage reservations, preserving active
  byte liveness and caller/transport timeout ownership.

## Status after r712

- `B1127-EXPLOREAVAILABLESCHEMABUDGET1=production-positive-r712`
- `B1126-WRITEBOUNDARYPARTITIONGUIDANCE1=prompt-delivered/production-non-adherent-r712`
- `B1125-MERMAIDONESIDEDNODEREF1=production-no-regression-r712`
- `B1130-ONESIDEDTYPEDRECEIPTIDENTITY1=confirmed/P1`
- `B1129-STRUCTUREDEDITEXACTENDLINEREPAIR1=confirmed/P1`
- `B1128-WRITERUNBUDGETRESERVATION1=confirmed/design-required/no-fixed-age-kill`
- `active-stream-4ms-degrade=forbidden/not-observed`
- `Trace explicit-window/causal projection/auto-supplement=unchanged`
- `Trace root=typed-on-chain-only; adjacent/background=support-only`
- `system-answer/conclusion-authorship=none`
