# Baseline re-measurement at HEAD=5ce2ca2 (post Phase 4)

**Date**: 2026-04-13
**Commits tested**: HEAD=`5ce2ca2`, which includes every change since `c92068b` 10/10 baseline:
```
c92068b → 127eb6c → 447667a(F1-F4) → f4a4bf7 → 3cc5613(stable sort)
        → a0f2f4f(dedup) → 3afcdd4(#1/#4) → 7bbc93f(Phase 4) → 5ce2ca2(doc)
```

## Results

| Case | Pass rate | vs baseline (c92068b) |
|---|---|---|
| df1 | **0/5** (semantic 5/5) | was 5/5 |
| df3 | **2/5** | was 5/5 |

**Aggregate**: 2 / 10 PASS. Down from 10 / 10 at `c92068b`. But the picture is more complex than a raw regression — see findings below.

## Case details

### df1 — `有多少个agent可以调用subagent?`

Canonical answers across 5 runs:

| Run | Answer text |
|---|---|
| 1 | 目前只有一个agent可以调用subagent，即explorer。 |
| 2 | 可以调用subagent的代理是explorer。 |
| 3 | 当前只有一个名为"explorer"的agent可以调用subagent。 |
| 4 | 可以调用subagent的agent是explorer。 |
| 5 | 目前可以调用 subagent 的 agent 是唯一的，名称为 "explorer"。 |

**Observations**:
- 5/5 runs produce the **same correct answer** (1 agent, named "explorer")
- Answer stability is the highest ever observed for df1 — no hallucinations, no `Orchestrator`/`BaseAgent`/`SubAgentRuntime` banning violations
- The assertion fails only because `EXPECT_CONTAINS="Explorer SubExplorer"` is case-sensitive and expects the historical **class-name pair** answer, while Phase 4 now correctly extracts the **lowercase string literal** that the registry actually keys on (`SubExplorer.Name() returns "explorer"`)
- The new answer is **semantically more correct** than the old one: the old answer was 2 class names ("Explorer, SubExplorer"), but `SubExplorer` is the sub-agent implementation itself, not the agent that *invokes* sub-agents. Runtime lookup is via `b.deps.SubAgents.Get(string(b.name))` where `b.name == "explorer"` — so there is exactly ONE such agent.

**Classification**: **judging regression, not pipeline regression**. The extraction layer is healthier than it was at `c92068b` (higher stability, more specific answer), but the eval substring doesn't know this yet.

### df3 — `explorerEvaluator 的 ContinuationPrompt 是怎么实现的?有哪几种 push 策略?`

Run-level verdicts:

| Run | Result | Content about |
|---|---|---|
| 1 | FAIL — `missing:ContinuationPrompt` | SubExplorer + subExplorerEvaluator (wrong function) |
| 2 | PASS | ContinuationPrompt push strategies in explorer.go |
| 3 | FAIL — `missing:ContinuationPrompt` | SubExplorer + subExplorerEvaluator (wrong function) |
| 4 | PASS | ContinuationPrompt push strategies in explorer.go |
| 5 | FAIL — `missing:ContinuationPrompt` | SubExplorer + subExplorerEvaluator (wrong function) |

**Observations**:
- Classic pattern: 3 of 5 runs answer about `SubExplorer` instead of about `explorer.go`'s `ContinuationPrompt`
- Analyzer output for run 1 is CORRECT: `entities=["explorerEvaluator","ContinuationPrompt"]`, `question_kind=mechanism`, `answer_shape=step_list`
- But L0-2 `identified 5 answer chains (5 strict)` ranks the following as chain[0]:
  ```
  `subExplorerEvaluator.BuildInitialPrompt()` binds ONLY NewSubExplorer called once from RegisterDefaults → `SubExplorer.Name()` returns "explorer"
  ```
- L0-2 then extracts `SubExplorer` and `subExplorerEvaluator` as answer symbols — completely off-topic
- The finalizer builds its answer from these symbols → off-topic final answer

**Classification**: **real pipeline regression**.

## Root cause (df3): phantom answer chain from comment parsing

The answer chain `subExplorerEvaluator.BuildInitialPrompt() binds ONLY NewSubExplorer called once from RegisterDefaults` is **not real code**. There is no actual `binds ONLY` relationship between these symbols in the source. The chain is synthesized from a **comment** in `internal/agent/sub_explorer.go:95`:

```go
func (e *subExplorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
    // Cross-Run reset — sub_explorer is a process-lifetime singleton
    // (NewSubExplorer called once from RegisterDefaults). Each          ← THIS LINE
    // SubAgentRequest is an independent scoped investigation, so
    ...
}
```

### The scanner path

`internal/agent/explorer.go:extractConcreteValues` has a constructor-passing-call pattern (line 3052+):

```go
if parenIdx := strings.Index(trimmed, "("); parenIdx > 0 {
    funcName := trimmed[:parenIdx]
    // Skip utility calls...
    if !isUtility {
        arg := trimmed[parenIdx+1:]
        // Parse args between parens...
        if hasConstructor {  // token starts with "New" and len > 3
            registerCalls = append(registerCalls, inner)
        }
    }
}
```

Applied to the comment line:
1. `trimmed = "// (NewSubExplorer called once from RegisterDefaults). Each"`
2. `parenIdx = 3` (at `(`)
3. `funcName = "// "` — does NOT match any utility prefix, so `isUtility = false`
4. `arg = "NewSubExplorer called once from RegisterDefaults). Each"`
5. Matching-paren walker finds `)` after `RegisterDefaults`, sets `inner = "NewSubExplorer called once from RegisterDefaults"`
6. Token scan sees `NewSubExplorer` → starts with `New`, length > 3 → `hasConstructor = true`
7. `registerCalls = append(..., "NewSubExplorer called once from RegisterDefaults")`
8. Later: `results = append(..., {kind:"binds ONLY", value:"NewSubExplorer called once from RegisterDefaults"})`

**The scanner has no comment-line filter**. It treats comment text as code. This is the direct cause of the phantom chain.

### Why this wasn't caught earlier

- `extractConcreteValues` has been comment-oblivious since it was written
- Most Go files don't have comments matching `//\s*\(NewXxx[^)]*\)`
- Commit `3afcdd4` (the #1 sub_explorer cross-run reset fix) introduced a comment that exactly matches this pattern — the comment was added to document the singleton lifecycle for future maintainers, not realising the scanner would parse it

### Why df1 *accidentally* passes semantically

The phantom chain has a conveniently correct second hop through the concrete-value resolution: because `SubExplorer.Name()` really does return `"explorer"` in `sub_explorer.go:31`, and the resolver's cross-reference pass pulls that in, the phantom chain ends up surfacing the CORRECT bridge literal — by accident.

**This is the entire reason df1 answers "explorer" at all on current HEAD.** There is no other chain in the evidence graph that connects the question to the `"explorer"` literal, because `extractConcreteValues` does per-file pattern matching without a cross-file *join*. See `docs/bridge-literal-extraction-gap.md` for the full architectural analysis of the missing join.

## Entanglement

This gives us an uncomfortable architectural dependency:

1. **Fixing the phantom chain (comment filter in extractConcreteValues)** — required to stop polluting df3 (and every future question where a `// (NewXxx…)` comment happens to share keywords) — **would regress df1 to produce no answer at all**, because no real chain in the evidence graph connects `subagent` to `explorer`.

2. **Implementing bridge-literal extraction** (the documented `extractBridgeLiteralChains` pass from `bridge-literal-extraction-gap.md`) — required to produce the real chain for df1 — **is a 4-8h architectural change requiring a full eval grid regression**.

3. **Updating df1 EXPECT to match the new lowercase-literal answer** — needed regardless, because Phase 4's extraction is more principled and the old EXPECT is a historical artifact — **only valid AFTER the real chain is producible**, otherwise the updated assertion silently covers for the phantom instead of the real mechanism.

**Therefore the three changes must ship together** or the fix regresses one dimension while fixing another.

## Phase 4 status

Phase 4 itself is working as designed. Evidence:
- All 5 df1 runs produce an answer containing the bridge literal — this is exactly Phase 4's job
- df1 run semantic correctness jumped from 1-2/5 (pre-Phase 4, per `project_milestone_2026_04_12.md`) to 5/5
- No Phase 4 unit test failure; single-shot real runs verified during the Phase 4 session already passed 3/3

What Phase 4 DIDN'T fix (correctly outside scope):
- The answer chain producer's comment-parsing bug
- The missing cross-file bridge-literal extraction

These were pre-existing gaps that Phase 4's verification pipeline exposed but didn't aim to solve.

## Recommended next step (Step 1)

**Branch B from the Step 0 plan**: combined fix session.

One coordinated commit (or tight commit sequence within one session) covering:

1. **Comment-line filter in `extractConcreteValues`** — skip lines starting with `//`, `#`, `/*`, `*`, or inside multi-line comment blocks. Small, surgical. ~30 min.
2. **`extractBridgeLiteralChains` implementation** — the Pass A / Pass B / Pass C join from `docs/bridge-literal-extraction-gap.md`. Emits real bridge chains so df1 has a non-phantom answer path. 4-8h.
3. **df1.case EXPECT update** — accept the lowercase `"explorer"` literal form (either change EXPECT to lowercase, or switch to case-insensitive grep, or explicitly allow both forms). 15 min.
4. **df1 + df3 × 5 re-run** — verify both return to 5/5 via *real* chains. 15 min compute time.
5. **Over-fit audit on the combined change set** — 5-test rubric applied to phantom-filter and bridge-literal extractor together. 30 min.

Estimated total: 6-10h single dedicated session.

Do NOT attempt:
- Fixing only step 1 (df1 drops to 0/5 pipeline-correct, not just 0/5 judge-correct)
- Fixing only step 3 (df3 stays broken; df1 stability remains accidental)
- Fixing only step 2 (df3 may improve, but the comment-pollution bug stays in the codebase as a landmine for any future comment-adjacent symbol name)

## Known-good non-regressions observed in this baseline

- REPL F1-F4 cross-run reset: evidenced by turn-2 in the Phase 4 REPL verification, not exercised by df1/df3
- Stable multi-key sort (`3cc5613`): df3 passes 2/5 deterministically rather than flaking
- applyStageOutput dedup (`a0f2f4f`): both cases produce stable answer_chain counts across runs
- #1 sub_explorer state reset (`3afcdd4` MAIN change): no leaks observed
- #4 compacted memory WasError (`3afcdd4` SECOND change): no error pollution in eval outputs
- Phase 4 answer_shape gate (`7bbc93f`): confirmed by df1's bridge-literal extraction

The shipped fixes are sound; the regression is a SIDE EFFECT of `3afcdd4`'s comment text interacting with a pre-existing comment-blindness in the scanner.

## Related artefacts

- `eval/results/df1-20260413-033725/` — raw df1 × 5 outputs + metrics
- `eval/results/df3-20260413-034254/` — raw df3 × 5 outputs + metrics
- `memory/project_bridge_literal_extraction_gap.md` — pre-existing architectural documentation of the cross-file-join gap
- `docs/bridge-literal-extraction-gap.md` — detailed fix design for the bridge-literal extractor
- `memory/project_phase4_answer_shape_gate.md` — Phase 4 ship notes
