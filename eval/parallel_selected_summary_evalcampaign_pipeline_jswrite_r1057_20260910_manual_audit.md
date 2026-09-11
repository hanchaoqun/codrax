# Selected Eval Manual Audit — r1057

- date: 2026-09-11T02:00:08Z
- sweep_start_ts: 20260910-190007
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

Completed human review below preserves the runner's original verdicts. Typed metrics and declared oracles do not decide whether a PASS solves the actual user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dayjs_duration_nan_symptom | FAIL | eval/results/github_issue_dayjs_duration_nan_symptom-20260910-190008 | write_apply,answer_regex | none | 120s | 28 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass (code + honest unverified delivery); program proof unavailable | native Node audit passes original 4 assertions and 26 boundary inputs; npm truly absent, no retroactive proof |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260910-190008 | answer_regex,answer_contains | none | 625s | 59 | read=31,repo_map=3,list=0,trace=0,source_lens=0 | midloop=18,inv=9/5,fin_reject=10,unavail=0,prune=5 | fail | valid diagram/table and B1655 live cleanup; residual role/carrier inaccuracies, opaque label and disconnected/duplicate nodes |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Frozen setup and engineering receipts

- Exact two, one attempt each; 02:00:08Z–02:10:33Z on 2026-09-11. Runner's local stamp is 20260910-190007. No third lane, scenario re-run, question/oracle/budget change, or live source modification.
- Binary revision `b33952a0e975`, built `2026-09-11T01:54:40Z`; snapshot `.codrax/tmp/codrax-selected-20260910-190007`. Four preceding fix commits were pushed separately: `1ba027f7d`, `10503dd33`, `75e864f2a`, `b33952a0e`.
- Frozen full suite exit 0, 86 tested packages (some unchanged packages cached): `.codrax/tmp/20260910-r1057-final-full.log`; tool319.606s, agent73.553s, types45.552s, tracequery106.985s, tracediag14.734s, hitraceconv151.626s, orchestrator27.206s. Build succeeded. Later B1656 diagnostic-only work is not included in this live binary or this full-suite receipt.
- First post-B1656 frozen full suite exit1: `.codrax/tmp/20260910-r1057-post-b1656-full.log`, 85 packages passed; tool324.752s failed only the existing `TestRunTestsTimeoutExitDisclosesInfraDowngradedLockfileAndUntrackedOutput` (2.17s, audit clean rather than expected lockfile/untracked effects). The same test has an earlier intermittent-failure witness in the colleague audit's2026-09-04 G6 record. The original failure lacks actual file/command detail, so startup delay vs lost audit is not proven. Keep this red receipt separate from B1656 targeted passes; no timeout or assertion relaxation.
- That original timeout test passed an unchanged isolated count3 (7.546s). Source review establishes that the outer shell started, not that the fake cargo completed its two writes before the2s deadline. Audit collection failures are explicitly unavailable. A test-only failure diagnostic now captures actual file bytes/errors and executed commands; no product behavior, original condition,2s deadline or8s sleep changed. B1656 and the initial audit were pushed as `369855605`; a same-command full-suite recheck follows separately.
- Final frozen same-command `go test ./...` recheck exited0,86 tested packages (some unchanged packages cached): `.codrax/tmp/20260910-r1057-post-b1656-full-recheck.log`; tool322.495s, agent71.664s, orchestrator22.635s, tracequery110.258s, types45.607s. Source/tests did not change during execution. The earlier red remains recorded; the intermittent timeout fixture's causal reliability issue is still open, not declared fixed by a later pass. No additional live run was performed.
- PATH explicitly included the existing Node24.19.0 bin directory; that directory contains **node only, not npm**. This is not a full Node+npm environment. No packages were installed for this round.

## Write audit: useful patch, correctly unverified original run

Actual diff changes only `src/duration.js:15` to `value === undefined ? 0 : Number(value)`. It repairs absent optional capture groups without truncating fractional seconds or changing the grammar. The original regression file is byte-identical (`cmp` exit0); it was not weakened to get green.

Actual official execution (combined log2101–2115, report.executed_commands):

1. `make check` exited0; its Python script checks source/test text and does not execute JavaScript.
2. Node syntax preflight was recorded separately; it did not execute the test assertions.
3. `npm test --` exited127 with `sh: npm: command not found`. The missing runtime is **npm**, not the already-available node binary.

The controller received 0/3 required behavior contracts covered and precise reasons (log2227–2248); the final report retained `failure_kind=runner_missing`, and final disposition was `accept_unverified`. No old/source-static result was promoted to a successful runtime assertion. This is an environment limitation, not proof of a false runner-missing classifier or a code failure. Do not silently replace arbitrary npm lifecycle scripts with a guessed direct-node invocation.

Independent **post-run** verification on `run-1.applied-tree`:

- Original native test: `.codrax/tmp/20260910-r1057-dayjs-native-final.log`, exit0, all4 original assertions reached. Baseline original tree failed its first assertion (`...-dayjs-native-baseline.log`), so the remaining3 were not baseline failures.
- `.codrax/tmp/20260910-r1057-dayjs-blackbox.cjs` → `...-dayjs-blackbox.log`, exit0: 16 valid inputs, 10 invalid inputs, 2 exact formatting checks. Covers each independent component, missing/zero/leading-zero/mixed/full inputs, fractional seconds, and unchanged invalid grammar. `P`/`PT` were already accepted by the old regex; no new rejection requirement was invented.
- These external audits establish human confidence in the delivered patch, **not** retroactive closure of the program's own missing npm proof ledger. Machine FAIL is preserved.

## Read audit: actual repaired output, not yet a good answer

Final artifact: `.codrax/output/20260910-191031.310-98693.{md,html}` (MD4617 bytes). The unmodified Mermaid was parsed and rendered in Chrome using the repository's bundled renderer with network routes blocked: `.codrax/tmp/20260910-r1057-pipeline-mermaid-render.json`, one sequence diagram, SVG27438 bytes. No reconstruction from rejected drafts or silent syntax rewrite was used to obtain this PASS.

What worked:

- Sequence diagram and the requested four-stage input/output/carrier table both exist; no final-output disappearance or outer1200s timeout this time.
- The input had clear teaching: log5925 minimal-first four-stage precedence; 5974–5981 three precise stage/agent recipes and explicit non-call/data-flow boundary; 6182 actual `Orchestrator.dispatchStage → ag.Execute`. Supporting call evidence was not absent.
- B1655 **naturally exercised**: after model-selected visible edits and removal of T, log7233–7254 exposed3 O→BC stale metadata rows. At7320 the model selected their3 distinct newly issued failure refs (plus2 stale boundary refs); 7343–7344 accepted the patch. It did not add arrows or delete the model's graph for the model. This closes the r1056 duplicate-stale-metadata execution defect, not the whole diagram campaign.
- Stringified patch arrays were repaired by existing shape coercion, without inferring new edits. Active stream semantic traffic remained live; the32.404s finalizer call at7020–7056 was not age-degraded. This run does not naturally exercise an uninterrupted4-minute response.

Why human FAIL remains:

- The diagram exposes `StageBindingStage_8244dfca36d6b97a` as a visible label, duplicates stage/agent participants, leaves O/BC as bare disconnected declarations, and adds local enum-to-field bindings that do not explain request execution. Those2 binding additions were model-selected, not secretly added by the system.
- Main prose says `emit_analysis` directly produces AnalysisIR and `applyStageOutput` merges it. Actual emitter writes RequestModel; analyzer's deterministic build compiles IR; `runAnalyzePhase` assigns `busCtx.AnalysisIR` directly (orchestrator2496).
- Main prose/table conflate AnswerDocumentV2, FinalAnswer and Citations as `emit_answer_document` products written together into `Mutable.answerDocumentV2`. Actual tool persists the document; the evaluator renders StageOutput.FinalAnswer, later recorded in Mutable.Result. extract also has legitimate skip/reuse branches, so unconditional four-agent invocation is not a complete account.
- The15 model citation pool entries never got item/body references; actual canonicalization pruned them as unused (log7327–7341). Only3 pre-stage citations survive from the inventory supplement. Main-stage claims therefore lack useful visible citations; supplement also contains pre-stages outside the requested analyze→finalizer interval. Do not fix this by attaching guessed citations or system-authored conclusions.
- **System supplement scope defect, not merely model prose**: the original answer had3 blocks (log6674); at6678 the system appended the missing-member supplement, visible at MD48–54. Its source is the accepted7-member `read_mode_stage_sequence` aggregate (`model_emitted/current_source`, log6157), not invented symbols. The3 supplemented members are LogTriage, PerfTriage and MultiRepoFocus. The first2 are conditional stages before the explicitly requested interval; MultiRepoFocus is not in the current read topology at all. Definition/binding evidence establishes existence, not current-path membership. The same context already supplies the narrower main4/conditional2 authority and says declarations cannot widen it (5924–5929). The append-only compiler honors model block ownership but amplifies an unsupported principal-membership claim. This reopens the B183 declaration-vs-active-membership family on the **system supplement** consumption surface (B1658/P1), not a license to rewrite model conclusions or scan prose to suppress symbols.

## Retry accounting and residual priorities

- 2 explore dispatches, 9 completion calls/5 rejected, 10 finalizer rejections/10 patch calls. A successful tool transport (`ok=true`) does not mean completion accepted: first window's4 rejected/downgraded reports precede actual completion; the second window was scheduled for remaining tasks, not a finalizer retry.
- First analyzer reject named inferred `Orchestrator`; it did not reject every submitted participant. Model's claim that the request lacked `BusContext` was its own misread. The request's carrier-table examples need not all become required diagram participants.
- First diagram made BusContext invoke agents and used return/data-flow operators incorrectly despite available accurate teaching. Subsequent orphan state needed only T; the exact error listed6 other participants, not T. Model repeatedly submitted7 or3 and replayed stale refs before selecting T atiter9. This is not established evidence of contradictory contracts; full typed schema/delta exists although diagnostic logging truncates long hints.
- **B1656/P2**: the staged orphan short summary showed count only although the new lease already had the exact row/actions. The post-live fix now displays bounded same-source rows/actions; it changes no gate or model choice and does not promise to eliminate model errors. Public red1.327s → green1.093s; new/neighbor/actual validator census count3 passed2.033s, race count3 passed11.962s. This fix was not in the r1057 binary.
- **B1657/P2, next audit/fix**: post-edit dependency closure currently handles unpaired replies but not newly stale metadata before orphan selection. O/BC had no visible incident edges left when the final metadata cleanup lease was built, so no optional cleanup candidate was carried forward. Prefer extending the existing typed post-edit dependency sequence/provenance, not automatically deleting nodes, inferring identity from labels, or imposing a new cosmetic retry requirement. Production witness + source confirmation exist; new public regression still needed.
- **B1658/P1, next audit/fix**: use the existing typed request/relationship scope and narrower verified execution membership to distinguish an existence inventory from a current-path member set before it authorizes a system completeness supplement. Preserve broad grounded declarations as model guidance/background; never silently replace the model's set with a system-chosen answer. Cover ordinary source inventories, broader explicitly requested workflows, conditional stages, other modes/repos and Trace exclusions with public tests before implementation. Source+production confirmation only; no new regression or implementation is claimed yet.
- Existing model-facing `from_node_visible_label`/`to_node_visible_label` capabilities already support friendly new-node names; the model omitted them. Do not invent another mandatory JSON field or wording hard gate for this one label.
- Architecture doc drift was independently verified: pre-stages execute synchronously in declaration order; extract may skip; already-accepted pre-stage artifacts are not guaranteed nil on error. Corrected **after** live completion, not blamed as this run's proven cause.
- Keep B1651b Gradle current-execution provenance and remaining Meson/Hvigor/history proof work ahead of new Make proof capability. No repeated read sample is scheduled to chase green; next exact-two batch should rotate a higher-priority Trace/proof dimension.

Trace explicit windows, on-chain root eligibility, causal projection, automatic supplementation and the actual-cost/eliminable-cost axes were unchanged. This two-non-Trace round does not replace Trace production replay. B193/B1646/B1654 did not naturally hit their Trace branches here; their public regressions and engineering receipts remain distinct from live coverage.
