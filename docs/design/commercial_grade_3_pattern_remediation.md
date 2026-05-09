# Commercial-Grade 3-Pattern Remediation

**Status**: WIP — single-session delivery
**Baseline**: `6dd16d7` (post u9a/u9b broaden)
**Started**: 2026-05-09 02:30 (after sweep bnnk2i1a5: 49/58 PASS)

## Goal

Eliminate the 3 systemic patterns surfaced by tonight's eval sweep without leaving any residual gaps. Each phase ships its own commit with prompt-audit gate before push.

## Three patterns identified

| Pattern | Symptom case(s) | Root cause class |
|---|---|---|
| **3 — Context-narrowing white-list leak** | mr_pin_isolation | `buildToolBusContext` drops typed signals (MultiGraph/TypedDenials/PendingSubRepos/Ctx) when narrowing AgentContext → BusContext for tools; new typed fields added in any future Phase risk silent drop. |
| **1 — Hard-gate carve-out incompleteness** | s7a (R2.2 over-fires on count question), prior u4a/u4b/u8a (L0-B over-fire on relational lookup) | analyzer/coherence hard gates condition on typed signals but don't declare typed-predicate carve-outs explicitly; new typed predicates require manual gate-by-gate carve-out audit. |
| **2 — ERM breadth-only (no depth gate)** | s7b (count off-by-2 via visual count), s8a (call_chain misses normalizer/compiler/hdp/binder), latent: u9b enumeration cardinality | ERM is breadth heuristic; no per-family completeness dimension (count_tool_required / path_depth_closure / declared_count_match / entity_parity / layer_depth). |

---

## Phase 3 — BusContextProjection (SMALLEST, FIRST)

### Architecture

**Insight**: replace white-list field-by-field copying with a struct-copy-then-explicit-drop pattern. New typed fields auto-included; only fields explicitly dropped (Mutable, TaskState ephemeral) get filtered out.

### Files touched

| File | Change |
|---|---|
| `internal/types/bus_context_projection.go` | NEW: `BusContextProjection` type + `Build()` with default drop-set |
| `internal/agent/agent.go:1806-1832` `buildToolBusContext` | migrate to projection |
| `internal/agent/context/builder.go:1081-1110` `BuildSubAgentContext` | migrate to projection |
| `internal/types/bus_context_projection_test.go` | NEW: build-time test enforcing all typed signals propagate |

### Default projection (DROP-SET only — black-list)

| BusContext field | Action |
|---|---|
| **MultiGraph** | KEEP (gate-relevant) |
| **TypedDenials** | KEEP (gate-relevant) |
| **PendingSubRepos** | KEEP (L0 advisory) |
| **Ctx** | KEEP (cancellation) |
| **Memory** | KEEP (REPL access) |
| **EnvFacts** / **EnvRecommendSettings** | KEEP |
| SubRepos / ActiveSubRepo / MultiRepoInactivePreviewCount | KEEP |
| **Mutable** | DROP for tool projection (tool can't mutate Run state) |
| **TaskState** | DROP (ephemeral) |
| All other fields | KEEP |

### Build-time validation

`TestBusContextProjection_AllTypedSignalsPropagated`:
- Reflect over BusContext fields
- Construct AgentContext with non-zero values for every typed-signal field
- Run projection.Build()
- Assert every typed-signal field non-zero in output (except explicit DROP set)
- Adding a new typed field without updating DROP set → test pin still passes (auto-included)
- Adding a new typed field that should be dropped → test pin fails until DROP set updated

### Sub-tasks (5)

- **3.1** Define `BusContextProjection` type + Build() in `internal/types/bus_context_projection.go`
- **3.2** Migrate `buildToolBusContext` → projection
- **3.3** Migrate `BuildSubAgentContext` → projection
- **3.4** Add `TestBusContextProjection_AllTypedSignalsPropagated` build-time test
- **3.5** Run mr_* eval suite + s11b (single-repo regression) + analyzer/agent unit tests

### Prompt audit checkpoints

Phase 3 has zero LLM-facing text changes (pure Go-internal). No prompt audit needed.

### Commit

`refactor(types): BusContextProjection — declarative narrowing with auto-include`

---

## Phase 1 — Hard-Gate Framework (MEDIUM, SECOND)

### Architecture

**Insight**: every analyzer/coherence hard gate must declare its typed-predicate carve-out matrix explicitly. The `HardGate` type has a `CarveOuts []TypedPredicateCarveOut` field that is non-optional (may be empty but must be declared). LLM-facing reject messages and skill-prompt advisory text are GENERATED from CarveOuts so they stay in sync with code.

### Files touched

| File | Change |
|---|---|
| `internal/analysis/gate/hard_gate.go` | NEW: `HardGate` + `TypedPredicateCarveOut` types |
| `internal/agent/analyzer.go:1461` (L0-B) | migrate to HardGate framework |
| `internal/analysis/gate/coherence.go:417-436` (R2.2) | migrate + add `!IsCountQuestion` carve-out |
| `internal/analysis/gate/coherence.go` (R1.2 / R1.4 / R1.5 / R2.1) | migrate with `CarveOuts: []` (explicit empty) |
| `internal/skill/analysis_contract.go` | rewrite carve-out advisory section to read from HardGate registry |
| `internal/analysis/gate/hard_gate_test.go` | NEW: `TestAllHardGates_ExplicitCarveOutDeclaration` |

### HardGate type (sketch)

```go
type HardGate struct {
    Name             string                  // "L0-B" / "R2.2" / "R1.2" / ...
    File             string                  // for source-of-truth audit
    Trigger          func(rm types.RequestModel) bool
    CarveOuts        []TypedPredicateCarveOut
    RejectMessage    string                  // LLM-facing
    LLMAdvisoryHint  string                  // for skill-prompt auto-gen
}

type TypedPredicateCarveOut struct {
    Predicate string  // e.g., "IsRelationalLookup" / "IsCountQuestion"
    Reason    string  // why this predicate carves out the gate (LLM-facing)
}

func (g HardGate) Run(rm types.RequestModel) (rejected bool, msg string) {
    if !g.Trigger(rm) { return false, "" }
    for _, co := range g.CarveOuts {
        if predicateValue(rm, co.Predicate) { return false, "" }
    }
    return true, g.RejectMessage
}
```

### Per-gate carve-out matrix (final)

| Gate | Trigger condition (typed-only) | CarveOuts | Reason |
|---|---|---|---|
| L0-B | `IsCategoryEnumeration && distinct(entities)≤1` | [IsRelationalLookup] | "filter set X by relation to Y" |
| R2.2 | `isLongForm && isScalarSubjectKind && conf≥0.6` | [IsCountQuestion] | "count question is long-form explanation + numeric scalar" |
| R1.2 | `IsCrossComponent && nSub≤1` | [] | direct contradiction, no carve-out |
| R1.4 | `nSub≥2 && !IsCrossComponent && (IsCategoryEnumeration\|Intent=Enumerate)` | [] | structural inconsistency, no carve-out |
| R1.5 | resolver mismatch in SubTopics | [] | structural check, no carve-out |
| R2.1 | `IsScalarAnswer && nSub≥2` | [] | direct contradiction, no carve-out |

### Sub-tasks (8)

- **1.1** Define `HardGate` + `TypedPredicateCarveOut` types
- **1.2** Migrate L0-B (preserve existing behavior — the !IsRelationalLookup carve-out is already in place)
- **1.3** Migrate R2.2 with new `!IsCountQuestion` carve-out
- **1.4** Migrate R1.2/R1.4/R1.5/R2.1 with `CarveOuts: []` explicit-empty
- **1.5** Skill prompt: replace hand-written carve-out advisory with auto-generation from HardGate registry
- **1.6** Build-time test `TestAllHardGates_ExplicitCarveOutDeclaration`
- **1.7** Run analyzer/coherence unit tests + s7a/u4a/u4b/u8a eval
- **1.8** Verify no regression on R1.2/R1.4/R1.5/R2.1 historical PASS cases

### Prompt audit checkpoints

Before commit:
- Skill prompt section auto-generated from CarveOuts — verify text reads naturally + R6 clean
- Reject message LLM-facing — verify R6 clean (no internal pipeline terms)
- TestPromptSnapshot_NoInternalTermsInRenderedOutput must pass

### Commit

`refactor(gate): HardGate framework with declarative typed-predicate carve-outs`

---

## Phase 2 — Two-Tier ERM (LARGEST, THIRD)

### Architecture

**Insight**: split ERM into Tier 1 (breadth heuristic, current) for explorer soft-stop signal AND Tier 2 (depth completeness gate) for finalize-time hard validation. Per-family completeness validators handle dimension-specific checks.

### Files touched

| File | Change |
|---|---|
| `internal/agent/explorer_erm.go` | extend ERMRequirement with Tier1/Tier2 split |
| `internal/agent/erm_completeness.go` | NEW: per-family CompletenessValidator implementations |
| `internal/types/requirement_kind.go` | add ReqCountMeasurement kind |
| `internal/orchestrator/finalize.go` (or extractor entry) | wire Tier 2 validators into pre-finalize gate |
| `internal/skill/explorer_skill.go` (or defaults.go) | per-family ERM expectation prose + count-question tool requirement |
| `internal/agent/erm_completeness_test.go` | NEW: per-family Tier 2 validator fixture tests |
| eval/cases/* | add per-family completeness fixture cases (if needed) |

### CompletionDimension enum + per-family Tier 2 validators

| Dimension | Family | Validator semantics |
|---|---|---|
| **scalar_count** | QFEnumeration when IsCountQuestion=true | answer must derive from exec_command (grep -c / wc -l) tool result, not visual count |
| **cardinality** | QFEnumeration with declared_count > 0 | item count must satisfy `min(declared_count, len(answer)) >= declared_count` |
| **path_depth** | QFCallChain | evidence must include entry function + ≥3 mid-segment + exit function (topology check) |
| **layer_depth** | QFConfigPrecedence | evidence must include all 3 layers (default, config-file, override) |
| **entity_parity** | QFComparison | per-bucket evidence count balance within 50% range |

### Count-question hard route (s7b fix)

When `IsCountQuestion=true`:
- explorer skill prompt mandates exec_command + grep -c / wc -l for the count
- Tier 2 ScalarCountValidator: answer must cite ≥1 EvidenceItem with `Source` containing `(grep -c|wc -l|find ... | wc)` tool result
- Reject finalize if no exec_command-derived evidence cites the count

### Call-chain path-depth (s8a fix)

Tier 2 PathDepthValidator for QFCallChain:
- Ground-truth function-call topology from `Graph.SymbolDefs` + `FileInfo.Relations`
- Validate evidence chain reaches both entry (buildAnalysisIR) and exit (gate.RunWith) AND has ≥3 distinct intermediate function calls
- Reject finalize if path closure fails

### Sub-tasks (12)

- **2.1** Define `CompletionDimension` enum + `CompletenessValidator` interface in `erm_completeness.go`
- **2.2** Add `ReqCountMeasurement` kind in requirement_kind.go
- **2.3** Implement `ScalarCountValidator` (count question hard route)
- **2.4** Implement `CardinalityValidator` (declared_count match)
- **2.5** Implement `PathDepthValidator` (call_chain entry-mid-exit closure)
- **2.6** Implement `LayerDepthValidator` (config_precedence 3-layer)
- **2.7** Implement `EntityParityValidator` (comparison bucket balance)
- **2.8** Wire Tier 2 validators into pre-finalize gate (orchestrator.go)
- **2.9** Skill prompt: count question MUST use exec_command (R6-clean prose)
- **2.10** Skill prompt: per-family ERM expectation (path closure / declared count)
- **2.11** Build-time test + eval s7b/s8a/u9b targeted
- **2.12** Full eval sweep (final verification)

### Prompt audit checkpoints

Before each commit in this phase:
- Per-family ERM expectation prose — verify R6 clean
- Count-question tool-mandate prose — verify R6 + LLM-natural ("use exec_command tool" not "explorer skill enforces")
- Reject messages from Tier 2 validators — verify R6 + actionable
- TestPromptSnapshot_NoInternalTermsInRenderedOutput

### Commit (split into 2-3)

- `feat(erm): two-tier ERM with depth-validator per question family`
- `feat(erm): scalar-count + cardinality + path-depth validators`
- `feat(erm): wire Tier 2 into pre-finalize gate + skill prompt updates`

---

## Cross-Phase invariants

### R6 prompt redline (every commit)

Every LLM-facing string must be R6-audited before push:
- No internal pipeline names (analyze/explore/extract/finalize)
- No internal mechanism names (gate / emit / dispatch / orchestrator)
- "looked up after classification" instead of "explore stage will fetch"
- "the answer must come from a deterministic tool" instead of "Tier 2 validator rejects without exec_command"

### R3 precise-typed-signal hard gate (every commit)

Hard gates must read typed precise signals only:
- Single boolean flags (typed predicates)
- Single integer comparisons (cardinality)
- Schema-validated typed enums (Family / Intent / Kind)
- Verbatim string substring matches against typed slot values

Soft guidance can read noisy signals (similarity scores, ranker outputs).

### R2' 6-spot sync (every new typed signal)

Adding a new typed field requires sync at 6 spots:
1. Go struct definition
2. JSON schema description
3. Skill prompt explainer
4. Retry hint composer
5. JSON decoder error remap
6. Cooccurrence rule / RepairLocus mapping

### Eval verification (every commit)

- Targeted: relevant test cases for the phase
- Final (after all 3 phases): full 58-case sweep ≥ 95% PASS

---

## Final audit checklist (after Phase 2 ships)

| Check | How |
|---|---|
| All BusContext typed fields propagate through narrowing | TestBusContextProjection_AllTypedSignalsPropagated |
| All hard gates declare CarveOuts explicitly | TestAllHardGates_ExplicitCarveOutDeclaration |
| Per-family ERM Tier 2 validators implemented | TestPerFamilyERM_Tier2Coverage |
| No internal pipeline terms in any LLM-facing string | TestPromptSnapshot_NoInternalTermsInRenderedOutput |
| All originally-failing eval cases now PASS | full sweep verdict |
| Single-repo zero-regression preserved | s11b + 50+ non-multi-repo cases |
| `go test ./...` green | full Go test suite |
| `go vet ./...` clean | no new vet warnings |

---

## References

- Tonight's sweep bnnk2i1a5: 49/58 PASS, 8/9 cases identified above
- Audit reports (in conversation history):
  - Pattern 1: 1 hidden gap (R2.2 IsCountQuestion)
  - Pattern 2: 5 per-family completeness gaps
  - Pattern 3: buildToolBusContext + BuildSubAgentContext both leak typed signals

## Status log

- 2026-05-09 02:30 — design doc landed, Phase 3 starting
- 2026-05-09 03:30 — Phase 3 / 1 / 2 advisory shipped (cf87d17 / 936b0f7 / f32b33c)
- 2026-05-09 04:00 — Phase 2.B planning starts (post-finalize answer-aware hard gate)

---

## Phase 2.B — Answer-Aware Hard Gate (post-finalize enforcement)

### Architecture

**Insight from deep audit (2026-05-09)**: codrax already has a production-grade
violation→retry→caveat pipeline. Tier 2 hard-gate enforcement does NOT need
new orchestrator state — it just needs to:

1. Move CompletenessValidator execution from pre-finalize advisory to
   post-finalize, AFTER the answer is composed
2. Validators read AnswerDocumentV2 (typed blocks) + ToolResults +
   EvidenceItems (3-tier input precedence), NOT just evidence pool
3. Failures emit a typed Violation with the existing 5-Locus → FallbackTarget
   machinery handling retry routing
4. Caveat materialization on retry exhaust is automatic via
   `AppendUserCaveatsToAnswer` once a CaveatFamily is registered for each
   new ViolKind

This makes Phase 2.B a *registry update* + *validator refactor* rather than
a state-machine refactor. Concretely:

### Files touched

| File | Change |
|---|---|
| `internal/types/violation.go` | NEW: 5 ViolationKind constants (ViolScalarCountUnsourced / ViolPathDepthInsufficient / ViolLayerDepthIncomplete / ViolCardinalityShort / ViolEntityParityImbalanced) |
| `internal/types/violation.go::AllViolationKinds()` | append 5 new kinds |
| `internal/types/violation_registry.go` | NEW: 5 ViolKindSpec entries — Severity / FallbackLocus / Layer / CaveatFamilyID + 5 CaveatFamily templates (EN + ZH) |
| `internal/orchestrator/retry_state.go::inferViolationLayer` | route 5 new kinds (legacy fallback for the 6th sync site) |
| `internal/orchestrator/fallback_policy.go` | inherit from registry (default policy already imports specs) — confirm no per-kind override needed |
| `internal/agent/erm_completeness.go::ValidatorInput` | add `AnswerDocumentV2 *types.AnswerDocumentV2` field |
| `internal/agent/erm_completeness.go::5 validators` | rewrite as answer-aware: 3-tier input precedence (typed blocks → SurfaceText → fallback) |
| `internal/orchestrator/orchestrator.go:3862` | new call site: `runPostFinalizeCompletenessGate(out, res, mutable)` AFTER ComputeRootCauseClosure, BEFORE retry budget checks |
| `internal/orchestrator/orchestrator.go:3705-3739` (current Phase 2 advisory) | DELETE the pre-finalize advisory wire-up (post-finalize gate replaces it; LLM still sees FixHint via the same Mutable.SetTier2CompletenessHint surface, set inside the new validators) |

### 4 ViolationKind designs (LayerDepth dropped)

**LayerDepthValidator removal rationale (cross-project portability)**:
the original design hard-coded "3 layers (default / config-file / CLI
override)" — this is codrax's specific config-precedence model. Real
projects vary widely:
  - Simple projects: 2 layers (env var + default)
  - Spring Boot: application.properties × N profiles + env + cmdline + ...
  - Kubernetes: ConfigMap + Secret + env + volumeMount + cluster default
  - Remote config (Consul / Apollo / Nacos): adds another layer
  - Static frontends: zero config layers

A hard validator demanding 3 layers would over-fire on any project that
isn't codrax-shaped. The correct typed signal already exists:
`AnswerDocumentV2.MissingRequestedRoles` is LLM-emitted; if the LLM
correctly identified missing layers, the existing `MissingRequestedRoles`
section in the rendered answer already discloses the gap. Adding a
system-side validator on top would either (a) duplicate existing
disclosure or (b) over-fire on legitimate non-codrax projects.

**Decision**: keep system-side enforcement only for the 4 dimensions that
are language-portable AND project-portable (count / cardinality /
path-depth / entity-parity). The config-precedence dimension stays
LLM-trusted via `MissingRequestedRoles`. The skill prompt rule
"CONFIG PRECEDENCE" (P2.9 already shipped) gives the LLM the abstract
guidance ("surface every layer in the precedence chain") without
mandating a count.

| ViolKind | Dimension | Severity | FallbackLocus | Layer | CaveatFamily |
|---|---|---|---|---|---|
| `ViolScalarCountUnsourced` | DimensionScalarCount | hard | LocusExplore | LayerEvidence | tier2_scalar_count |
| `ViolPathDepthInsufficient` | DimensionPathDepth | hard | LocusExplore | LayerEvidence | tier2_path_depth |
| `ViolCardinalityShort` | DimensionCardinality | hard | LocusExtract | LayerExtraction | tier2_cardinality |
| `ViolEntityParityImbalanced` | DimensionEntityParity | hard | LocusExplore | LayerEvidence | tier2_entity_parity |

All use `FixableByAgents = [AgentExplorer]` (or `+ AgentExtractor` for
Cardinality which can be re-emitted from existing evidence).

The bounded-retry semantics fall out of the existing per-kind retry budget
machinery (`state.retryUsedForKind(kind)`) — no new bookkeeping needed.

### Per-validator answer-aware redesign (3-tier input)

Each validator now reads `ValidatorInput.AnswerDocumentV2` PLUS the
existing IR + EvidenceItems + ToolResults. Per-tier precedence makes
false-positives impossible when the answer ALREADY contains the
required structural coverage:

| Validator | Tier 1 (typed) | Tier 2 (prose) | Tier 3 (fallback) |
|---|---|---|---|
| ScalarCount | doc.Blocks where Kind=BlockScalar AND has Citation linking to ToolResult.ToolName="exec_command" | scan block.Text for digits AND verify ≥1 exec_command in ToolResults with integer summary | reject (zero typed evidence) |
| Cardinality | sum of Items[] across BlockBulletList/BlockOrderedList ≥ DeclaredCount | regex `^(\d+\.|[-*])` count across block.Text | len(AnswerSymbols) (existing behavior) |
| PathDepth | distinct AnchorSymbol from EdgeAnchors / ClaimUses with FacetID matching call-chain | distinct identifier-shaped tokens in BlockOrderedList items | distinct AnchorSymbol from EvidenceItems (existing behavior) |
| EntityParity | per-bucket evidence counts via Buckets[].Anchors substring match in block.Text | per-bucket prose word-count proxy | existing behavior |

### Caveat templates (4 new CaveatFamily entries)

Templates are user-facing, R6-clean (no internal pipeline names),
project-portable (no Unix / Go / codrax-specific assumptions), and
abstract enough that the recommendation makes sense across operating
systems and codebase shapes.

```
tier2_scalar_count:
  ZH: 答案中的数字来自人工视读而非确定性的计数工具，建议用一个可靠的计数命令重新核对精确值。
  EN: The number in the answer was derived by visual counting rather than from a deterministic counting tool; consider re-verifying the exact value with a reliable counting command.

tier2_path_depth:
  ZH: 调用链答案缺少关键的入口、出口或足够的中间节点，覆盖可能不完整。
  EN: The call-chain answer is missing the entry, exit, or sufficient intermediate steps; coverage may be incomplete.

tier2_cardinality:
  ZH: 问题明确指定了条目数量，但答案列出的项目少于声明数量，可能遗漏部分条目。
  EN: The question stated an explicit count, but the answer contains fewer items than declared; some items may be missing.

tier2_entity_parity:
  ZH: 对比问题中各方的证据采样不均衡，某一方的支持强度明显弱于另一方。
  EN: The comparison answer's evidence is skewed; one side is well-grounded while the other relies on weaker support.
```

**Cross-project portability audit** (2026-05-09):
- ScalarCount caveat avoids naming specific Unix tools (`grep -c` / `wc -l`)
  so Windows / restricted-shell users get a meaningful message. The skill
  prompt's COUNT QUESTIONS rule already gives concrete examples for the
  LLM during exploration.
- PathDepth / Cardinality / EntityParity caveats use abstract concepts
  (entry/exit/intermediate, declared count, balanced sampling) that map
  to any language ecosystem.
- LayerDepth dimension was REMOVED from this remediation (was codrax-
  specific 3-layer assumption); LLM's `MissingRequestedRoles` typed slot
  already discloses missing layers natively per the user's question
  shape.

### Wire-up at post-finalize

```
finalize stage emit_answer_document
  → runContractCheck() [orchestrator.go:3818]
    → contract.Result with violations[]
  → ComputeRootCauseClosure() [orchestrator.go:3862]
  → [NEW] runPostFinalizeCompletenessGate(out.FinalAnswer, mutable, ir):
      input := ValidatorInput{
          IR:               busCtx.AnalysisIR,
          EvidenceItems:    busCtx.EvidenceItems,
          AnswerSymbols:    busCtx.AnswerSymbols,
          ToolResults:      busCtx.ToolResults,
          AnswerDocumentV2: mutable.AnswerDocumentV2(), // NEW
      }
      family := BuildAnswerSemanticView(ir, nil).Family
      if fail := agent.RunFamilyValidators(family, input); fail != nil {
          v := dimensionToViolation(fail.Dimension, fail.Reason, fail.FixHint)
          res.Violations = append(res.Violations, v)
          mutable.SetTier2CompletenessHint(fail.FixHint)
      }
  → existing retry / caveat machinery handles the rest
    [orchestrator.go:3865-4030]
```

The existing `AppendUserCaveatsToAnswer(out.FinalAnswer, res.Violations, lang)`
at lines 3949 / 3970 / 4003 picks up the new violations automatically once
their CaveatFamily is registered.

### Sub-tasks (12) for Phase 2.B

1. **2B.1** Add 5 ViolKind constants + AllViolationKinds() append
2. **2B.2** Register 5 ViolKindSpec entries (Severity / FallbackLocus / Layer / CaveatFamily)
3. **2B.3** Register 5 CaveatFamily templates (EN + ZH, R6-clean)
4. **2B.4** retry_state.go::inferViolationLayer — route 5 new kinds
5. **2B.5** ValidatorInput add AnswerDocumentV2 field
6. **2B.6** ScalarCountValidator answer-aware rewrite (3-tier)
7. **2B.7** CardinalityValidator answer-aware rewrite
8. **2B.8** PathDepthValidator answer-aware rewrite
9. **2B.9** LayerDepthValidator answer-aware rewrite
10. **2B.10** EntityParityValidator answer-aware rewrite
11. **2B.11** Replace pre-finalize advisory with post-finalize hard gate at orchestrator.go:3862; delete the pre-finalize wire-up
12. **2B.12** Per-validator unit tests + R6 audit + commit + push

### Risk mitigation (false-positive control)

- **3-tier input precedence**: validators ALWAYS check typed evidence first. Pre-existing answers that look right on typed signals never trip the validator.
- **per-kind retry budget**: bounded at 1 — second persistent fail degrades to caveat-only.
- **No skill-prompt regression**: existing P2.9 / P2.10 skill prompt rules stay (count → exec_command, call-chain coverage, config precedence). LLM proactively avoids the gap; validators are the safety net.
- **Eval gate**: full sweep after 2B.12 — verify 95%+ PASS rate maintained, no new regressions on previously-passing cases.

### Prompt audit checkpoints

Before commit:
- 5 CaveatFamily templates EN + ZH — verify R6 clean, user-friendly tone (not LLM-instructional)
- TestPromptSnapshot_NoInternalTermsInRenderedOutput
- TestNoInternalTermsInPrompts (skill side)
- New TestTier2CaveatFamilies_R6Clean asserting templates contain no internal pipeline terms

### Backward compatibility

- Pre-finalize advisory (current Phase 2) DELETED in 2B.11. The same FixHint
  surface (Mutable.SetTier2CompletenessHint) is now set INSIDE the
  post-finalize validators when they fail. The `Answer Coverage Notes`
  section in the finalize prompt becomes irrelevant for the FIRST finalize
  attempt (no advisory) but VERY relevant for the RETRY attempt (LLM sees
  exactly what to fix).

