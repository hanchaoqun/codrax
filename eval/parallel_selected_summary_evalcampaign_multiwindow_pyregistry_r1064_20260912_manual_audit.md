# r1064 Manual Audit — original outputs preserved

- date: 2026-09-12T11:07:36Z
- sweep_start_ts: 20260912-040735
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

Audited after both original runs finished. The machine verdicts below are unchanged; human correctness separately covers provenance, reasoning, diagram meaning, and final publication. No third run or oracle change was used.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e1_dual_window_normalized | PASS | eval/results/real_trace_e1_dual_window_normalized-20260912-040736 | log_regex,trace_attachment,answer_regex | perf_triage+trace_query | 109s | 36 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Correct A/B numbers; model chose full_artifact instead of requested members; unsupported competition inference; B1618-P2b appendix provenance loss |
| 2 | sr_py_registry_dispatch | FAIL | eval/results/sr_py_registry_dispatch-20260912-040736 | answer_regex,answer_contains | none | 424s | 33 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=14,unavail=0,prune=0 | fail | Core lookup/registration prose retained, but system B1647c normalization/lease conflict blocked legitimate orphan cleanup; degraded original diagram loses executor distinction |

## Frozen build and execution

- Source `4fed22871e80`, pushed before live; binary built `2026-09-12T11:07:08Z` without dirty marker. B1626 final frozen count1 suite: 86 tested packages pass; pre/postcommit make pass.
- Exactly two cases, once each, concurrently, original case/oracle/CAP5/1200s. SDKROOT26.5 inherited. Runner start `11:07:36Z`, last finish `11:14:40Z`; aggregate runner exits 0 but records machine **1 PASS / 1 FAIL**, not 2/2 success.
- Original logs, Markdown, HTML, root report, and eval metrics remain unchanged. Subsequent fixes must not be described as changes to these historical outputs.

## Trace: values correct, scope routing and interpretation not closed

Original artifact: `.codrax/output/20260912-040923.566-75309.md`; log: `eval/results/real_trace_e1_dual_window_normalized-20260912-040736/run-1.logs/codrax-20260912-040737-000-75309.log`.

1. Independently recomputed from the original trace: A `34579.472865..34579.475857`, 2.992ms, Running0/Runnable0.014/Sleep2.978ms; B `34579.475857..34579.505857`, 30ms, Running3.414/Runnable0.780/Sleep25.806ms. Running ratios 0% and11.38%; state sums close, D/IO markers0. The boundary switch-in belongs to B, not A. MD19–33 retains correct values and denominators; no zero-base growth factor error.
2. Log331 teaches ordered `time_windows` and forbids an envelope; model `emit_analysis` at535 instead selects `full_artifact` and quotes “这份 trace”. Six model queries nevertheless use exact A/B. This is not the system overwriting a valid multiwindow profile. B1626's new member path was **not exercised naturally**; public integration tests still stand, but this live cannot close its production adoption.
3. Log2030 runs actual whole-trace supplementation under the accepted profile. Its own JSON source (`.codrax/blob/20260912-040737-000-75309/trace-query-result-ef5514fd.json`, bounds9434+) is `34579.450627..34579.595184`, 144.557ms; it is not a32.992ms enclosing A/B query. The question remains bounded state comparison, no causal/root contract forced. Default131-byte schema2 sidecar exists with empty `root_causes` and `trace_root_cause_contract_not_active`, appropriate here. No Mermaid: parse/render N/A.
4. MD35 calls A “纯睡眠” despite0.014ms Runnable, and uses raw Runnable increase across unequal windows to infer substantially greater competition/workload. Neither claim follows from the supplied counters. The finalizer already has proper state and IO-caliber facts (including log2752); MD23's unqualified absence of IO wait is wider than marker evidence. These are model-quality observations, not grounds for prose scanning, answer rewriting, or same-case chase-green.
5. **Existing B1618-P2b now has a production witness:** MD52–53 append two whole-trace five-state lines without measurement endpoints/capture. `proseWallClockAccountsFromLedger` drops those typed axes into `proseWallClockAccount`, then `buildProseFactEvidence` stores one account perTID; `proseFactThreadLine`/`proseFactPartitionFact` show only values/duration. The input retains the correct coordinates; the appendix loses them. Preserve all account sources/windows and disclose each own measured scope; never guess A/B from prose or call model-selected scope the measurement's provenance. Merely adding coordinates cannot close the separate sameTID/multicapture overwrite arm.
6. The unique original model summary at log2859 is byte-preserved in the final Markdown. 0 finalizer rejects/patches; no hard-degraded stream or forced causal projection.

## Python: actual system retry contradiction, not JSON failure

Original artifact: `.codrax/output/20260912-041438.404-75311.md`; log: `eval/results/sr_py_registry_dispatch-20260912-040736/run-1.logs/codrax-20260912-040737-000-75311.log`.

1. Relevant source is supplied: runner/registry log833–899, plugins963–989, base1037–1081. Correct chain: import-time decorator stores the JsonPlugin **class**; `resolve` looks up and constructs a fresh instance; `run_pipeline` submits the synchronous bound `handle` through the executor and awaits its result. JsonPlugin inherits the cooperative TimestampMixin→ValidationMixin→BasePlugin handle chain. No JSON parser is implemented, and registry lookup is not a REGISTRY-to-resolve call.
2. The first graph contains unproved relations. Initial rejection and exact edge-removal instructions are legitimate. Model patch3584 removes the listed relations;3587–3588 stages those changes and asks only for orphanREG disposition. At3617 the model submits only `retain_as_context(REG,"REGISTRY")`;3619 the system fills identities on untouched RP→RES;3620–3621 then reject this as both unlisted removal and addition. `remove_if_isolated` repeats the same deterministic failure at3941–3945. Later stale-ref/whole-block/absent-action errors are model mistakes, but cannot erase the earlier valid model action blocked by the system.
3. **B1647c/P1 confirmed:** unique typed topology maps recipe n3→n4 to model RP→RES in the normalizer, whereas the lease stabilizer uses only recipe node IDs. Two components use inconsistent identity representations. The safe fix is private receipts of this invocation's actual hidden-field changes, matched to complete unchanged baseline anchor occurrences; preserve normal lease/evidence validation, visible edges, claims, and model prose. Do not solve by disabling the gate, inventing aliases, or deleting the graph.
4. Final status is degraded recovery of the original structured draft:14 rejects/13 patch calls, machine FAIL `degraded_answer_checks_skipped:1`. Core prose accurately explains lookup/registration/executor, but recovered diagram still draws direct run_pipeline→JsonPlugin handling with no executor participant. It also omits the requested registration phase from the diagram. Human overallFAIL; this is not a new Mermaid syntax error. The first draft remains visible with explicit downgrade disclosure; temporary retry reasoning is not published.
5. Repo-bundled Mermaid parsed and rendered the unmodified recovered graph successfully, one sequence diagram, SVG23322bytes; receipt `.codrax/tmp/20260912-r1064-python-mermaid-bundled.json`. Initial audit command failed because `node` was absent from PATH; rerun used the bundled executable, not modified diagram input. Parser/render success does not certify relation correctness.
6. Separate navigation P2: relation_map log529 assigns plugins.py17 `@register("json")` to CsvPlugin.content_type, whose body ends14. `relationMapEnclosingSymbol` chooses nearest preceding start without EndLine containment; the typed relation sibling already checks EndLine. This incorrect advisory edge did not itself enter the final graph; preserve that limitation and add a source-boundary public matrix before fixing. Existing B1664 calls-vs-registration summary wording debt remains separate.
7. JSON compatibility recovered encoded array fields in later patches; that did not cause the valid orphan-only call's failure. Repair teaching must remain current-generation and branch-specific, but adding more prompt prose cannot resolve this exact code-side contradiction. Approved600/300/600second request defaults remain visible in logs; failure is validation exhaustion, not a4ms/4minute active-stream timeout.

## Next batches

1. B1647c: public staged-orphan RED, actual normalization receipts, direct/business-node/multiplicity/partial-field positive and negative controls; independent review and frozen regression before push.
2. B1618-P2b: sameTID multiple query/capture accounts and neutral measured-scope appendix, with ownership and no-prose-inference controls.
3. Navigation EndLine containment and existing B1664 registration wording; then rotate native write/plan and causal Trace cases. B1561 diagnostic coverage and B1575 per-contract execution protocols stay independently open. No third live run in r1064.

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
