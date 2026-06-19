# Codrax Read Mode Architecture Inventory Convergence Gap Plan

## Scope

This document tracks a read-mode system gap exposed by architecture-inventory questions such as:

> 读取代码分析，一共几个 agent，都是什么作用，之间的关系是什么？

The symptom is not a single prompt failure. It is a convergence boundary issue across analyzer classification, typed relation coverage, exploration completion, evidence projection, and final answer handoff.

## Observed Signals

- The analyzer may classify one architecture inventory question as a combined count, enumeration, per-member role explanation, and relation lookup.
- Exploration can split into several investigation lanes and accumulate more than one hundred evidence records.
- The model repeatedly tries to call `emit_investigation_complete`, but pre-complete returns a soft downgrade rendered as "验证还不够稳，正在补一轮".
- UI can appear stuck at "整理上下文中" because each new model request and finalizer prompt assembly must carry a very large evidence/history surface.
- A nearby historical log showed relation handoff repeatedly rejecting the same one-member answer because support relation rows such as tool/runtime/subagent edges were treated as missing principal members.

## Root Gaps

| ID | Priority | Gap | Systemic Cause | Target State |
| --- | --- | --- | --- | --- |
| RAI-C1 | P0 | Typed relation coverage can mix principal answer members with support/mechanism relation rows. | `BuildTypedRelationQuery` used a broad compatibility lane that could add `implements` coverage to non-implement relation shapes, so support rows like `ProposeSubAgents implements Agent` could become hard relation gaps. | Hard relation coverage only checks relation families selected by typed request shape. Support, registry, runtime, tool, and identity rows remain advisory unless they are the requested qualifying-member relation. |
| RAI-C2 | P0 | Repeated pre-complete downgrade lacks a low-delta convergence boundary for inventory questions. | The gate can return the same required handoff after the model adjusts the member set, because the expected relation candidate set is misaligned with the requested principal role. | Repeated downgrade reasons must either produce a new typed repair target or converge to the accepted typed boundary; no loop should keep broadening from identical support-only gaps. |
| RAI-C3 | P0 | Architecture inventory lacks a compact typed inventory projection. | Evidence is carried as many raw `EvidenceItem` rows, while count/list/role/relationship obligations need a smaller entity-centric coverage view. | Introduce a read-mode inventory coverage projection: principal entities, role summaries, relationship edges, support refs, exclusions, unknowns, and coverage counters. Downstream stages consume Top-N typed views, not raw evidence floods. |
| RAI-C4 | P1 | Context preparation cost scales with raw evidence volume. | Prompt sections and finalizer context can traverse evidence, aggregate facts, relation rows, and read history repeatedly after exploration has already closed. | Context builders should consume cached preflight/projection views and compact repeated evidence into entity/edge ledgers before rendering. |
| RAI-C5 | P1 | Generic entity fallback can leak into hard consumers. | Analyzer warns about generic terms like `Agent`, but when provenance lanes are sparse, hard relation consumers can still use broad `Entities` fallback. | Hard relation gates prefer mentioned/exact/primary typed lanes and ignore generic fallback unless an exact carrier proves it is the requested source. |
| RAI-C6 | P1 | Programmatic concrete-value evidence can flood handoff even when markdown preview is capped. | `buildConcreteValuesSection` capped the visible markdown table, but still exported uncapped `allRelevantForEvidence` and chain evidence into `structuredEvidence`, increasing finalizer/context assembly cost. | Keep concrete-value scanning local, but export a typed, ranked, bounded evidence view; architecture inventory receives a tighter compact ledger, scalar/count/literal lanes keep a wider budget. |
| RAI-C7 | P0 | Source-operation-site routing used downstream `RawRequest` phrase checks. | `RequiresSourceOperationSiteMemberSetHandoff` used localized set-boundary and operation-site word lists as a hard router, which violates the typed-intent boundary. | Removed the word lists. The helper now requires typed set-boundary signals plus typed operation-site surface signals from analyzer IR/profile fields. |

## Design Principles

- Hard gates consume typed signals only: request model fields, candidate relation enums, candidate precision, evidence roles, support refs, and aggregate facts.
- No hard routing depends on raw user keywords, prompt prose, model rationale, visible thinking, or natural-language summaries.
- Support evidence can explain principal members but must not become a principal-member obligation.
- Routine read-mode UX should keep running automatically; users should only see compact status reasons, not repeated internal gate text.
- Existing read scheduler L1 behavior remains untouched. Fixes land in typed selectors, pre-complete gates, projection/cache layers, and tests.

## Delivery Plan

| Batch | Status | Task | Acceptance |
| --- | --- | --- | --- |
| Batch A | delivered | Document this gap and fix typed relation selector pollution. | `AxisCall` / `AxisRegister` relation shapes do not add `implements` compatibility coverage; true `AxisImplement` coverage still catches omitted grounded implementers. |
| Batch B | planned | Add relation principal/support role guard to pre-complete diagnostics. | Repeated support-only relation rows produce advisory handoff, not hard missing-member loops. |
| Batch C | delivered | Add architecture inventory shape trait and compact concrete-value handoff export. | One inventory question carries a bounded, ranked programmatic evidence view instead of every concrete-value candidate. |
| Batch D | planned | Add full architecture inventory coverage projection. | Finalizer/extractor prompts consume entity/edge views and keep raw evidence available by ref, reducing "整理上下文中" stalls beyond concrete-value evidence. |
| Batch E | delivered | Quarantine generic entity fallback for hard relation gates. | Generic analyzer entities cannot seed hard relation coverage unless also present in mentioned/exact/primary lanes; prompt hints still retain soft fallback. |
| Batch G | delivered | Remove source-operation-site `RawRequest` keyword routing. | Operation-site member-set handoff is triggered only by typed set-boundary and operation-site surface fields. |
| Batch F | planned | Add eval coverage. | Architecture inventory, scalar role lookup, relation member-set, and implementer enumeration cases pass without repeated identical downgrades or evidence explosion. |

## Test Matrix

- `AxisCall + relational lookup + category enumeration`: query selects only `called-by` for hard/prompt relation coverage, not `implements`.
- `AxisRegister + relational lookup`: query selects only `registers`, not `implements`.
- `AxisImplement + grounded implementers`: omitted grounded implementer still downgrades completion.
- Support-only relation rows: accepted as advisory context and do not force principal member-set expansion.
- Architecture inventory smoke: evidence count remains bounded by projection; final answer preserves count, roles, and relation edges.
- Concrete-value export budget: architecture inventory questions compact programmatic evidence to the architecture inventory budget; scalar/count/literal questions keep a wider literal-proof budget.
- Prompt hygiene: no new keyword matching of user/model prose; no prompt-only route becomes a hard gate.

## Progress Ledger

- 2026-06-20 Batch A delivered: typed relation selector compatibility is now prompt/coverage-safe for precise non-implement relation axes. Added selector and pre-complete regression tests covering call-axis relation questions polluted by implementer support rows.
- 2026-06-20 Batch C delivered: added `IsArchitectureInventoryShape` and compacted concrete-values handoff evidence by typed request shape. This reduces context assembly pressure while preserving local concrete-value scanning and wider scalar/count literal budgets.
- 2026-06-20 Batch E delivered: coverage-gate relation source selection now uses mentioned/exact/primary provenance lanes only. Broad analyzer `Entities` remain available for prompt/search guidance but cannot seed a hard relation member-set downgrade.
- 2026-06-20 Batch G delivered: removed downstream `RawRequest` localized keyword routing from source-operation-site member-set handoff. Tests now construct the shape through typed IR fields instead of prose fixtures.
