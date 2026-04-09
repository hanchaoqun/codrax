# Investigation: Duplicate Output Across Tasks on a Single-Hop Analysis Question

> **Date:** 2026-04-09
> **Trigger query:** `这个项目中的pipeline有几种状态，变迁逻辑是怎样的？`
> **Outcome:** N=1 observation recorded. No fix shipped — waiting for N=2 of the same pattern before touching `task-analysis-skill` or context-builder scope rules, per the over-fitting audit's "N=1 → wait" rule.

## 1. What was observed

A run of the trigger query against the current `main` (commit `37b1d4a`) completed successfully and reached the correct answer, but the pipeline produced **two tasks whose final outputs were largely the same content**:

- Task 1 — `Identify pipeline states`: returned the 8-stage list plus a brief description of priority-weighted transitions, citing `config/orchestrator.yaml:16-190`, `internal/types/enums.go:7-14`, `config/orchestrator.yaml:62-124`.
- Task 2 — `Analyze state transition logic`: returned the 8-stage list **again**, plus a per-stage transition table with priority numbers, citing `config/orchestrator.yaml:16-190`, `internal/types/enums.go:7-14`, `internal/orchestrator/orchestrator.go:447-448`.

Task 2 is a strict superset of Task 1. Both completed via the short `explore → finalize` path under the `analysis` policy; both went through `analysis-final-answer-skill` (the routing fix from `9e67cc2` is firing correctly).

The pipeline trace:

```
task=1 step=0 stage=explore  → repo-explore-skill
task 1 transition: explore -> finalize
task=1 step=1 stage=finalize → analysis-final-answer-skill
task 1 reached terminal stage finalize
task=2 step=0 stage=explore  → repo-explore-skill
task 2 transition: explore -> finalize
task=2 step=1 stage=finalize → analysis-final-answer-skill
task 2 reached terminal stage finalize
```

Substance was correct in both tasks; the defect is **redundancy**, not wrongness.

## 2. Why the duplication happens — three layers

### Layer 1: the analyzer over-decomposed a single-hop question

`task-analysis-skill` split `这个项目中的pipeline有几种状态，变迁逻辑是怎样的？` into:

1. Identify pipeline states
2. Analyze state transition logic

Task 1 is a strict subset of Task 2 — you cannot describe transitions without first naming the states they connect. The user's question is one indivisible "explain the state machine" ask, and a single explore + finalize pass would have answered it. The analyzer mechanically applied a "list-then-analyze" decomposition that fits some questions but not this one.

This is the **root cause**. Layers 2 and 3 below are the propagation channels that turn the over-decomposition into duplicated *output*; without Layer 1, neither would matter.

### Layer 2: per-task prompt scope is weak

In `internal/context/builder.go:158-170`, the user-role section of every agent prompt is built as:

```
## User Request          ← ac.Objective — full original question, identical across tasks
## Current Task          ← [task_id] one-line title, the only task-scoped field
## Prior Stage Findings  ← ac.PriorReports — see Layer 3
## Known Facts           ← ac.RelevantFacts — accumulated across the whole BusContext
```

`Objective` is always the original full user request. `CurrentTask` is a one-line title with no instruction telling the LLM "answer only this slice, do not exceed your scope." Faced with "User Request: explain the whole state machine" and "Current Task: identify pipeline states", the LLM reasonably defaults to the larger of the two and answers the full question.

This is **not obviously a bug**. Keeping `Objective` globally visible is intentional — agents need the original ask to ground their work, and most multi-task implementation flows depend on it. Adding a "stay in your lane" hard constraint to the prompt is exactly the kind of over-fit doc-comment / prompt bait that the prior session's Fix E reverts caught: it would help this case while making honest multi-task implementation flows timid and over-narrow.

### Layer 3: `PriorReports` is append-only across tasks

`builder.go:53` does `ac.PriorReports = bus.StageReports`. `BusContext.StageReports` is never reset between tasks, so:

- Task 2's explorer sees Task 1's explore *and* finalize stage reports.
- Task 2's finalizer sees its own explore report **plus** Task 1's explore and finalize reports.

`analysis-final-answer-skill`'s few-shot examples teach the finalizer to organize the contents of its `Prior Stage Findings` into an Answer/Evidence block. When that input already contains a complete answer (because Task 1 is a subset of Task 2), the output naturally restates the complete answer.

This explains the observed asymmetry: **Task 2's output is more detailed than Task 1's**, not the other way around. Task 1's finalizer had only one stage report to work from; Task 2's finalizer had three. More material → more detail → the per-stage transition table appears in Task 2 only.

The cross-task carry-forward is also **not obviously a bug**. The comment at `builder.go:50-53` makes the rationale explicit: agents should be able to read what earlier stages concluded instead of re-deriving it from raw tool dumps. Cutting it would re-introduce the "re-derive from scratch" failure mode that earlier sessions specifically fought.

## 3. The over-fitting audit on each candidate fix

Three fixes are imaginable. None passes the audit yet at N=1.

### Candidate A — make `task-analysis-skill` prefer single-task decomposition under the `analysis` policy

**What it would do:** add a workflow rule in the analyzer skill: "if the active policy is `analysis` and the question fits in one explore-finalize pass, emit exactly one task."

- **Reverse test:** would it harm anything? Multi-hop analysis questions ("compare A and B and tell me which is faster") legitimately need multiple tasks. The rule needs a real predicate for "fits in one pass", which the analyzer doesn't have.
- **Class test:** does the rule generalize? Possibly — but only if "fits in one pass" can be made concrete. "Single-hop" is the LLM's own judgment, which is exactly the noisy signal the analyzer is supposed to compensate for.
- **N test:** **N=1.** This is the first observed case of an over-decomposed analysis question producing duplicate output. The rule "wait for N=2 before generalizing" applies. **Defer.**

### Candidate B — strip `Objective` from the agent prompt for non-leading tasks

**What it would do:** show the original user request only on Task 1; later tasks see only `Current Task`.

- **Reverse test:** **fails.** Implementation flows depend on every agent having access to the original user ask — that is how `verify` knows what success looks like, how `code_review` knows whether the patch matches intent, etc. Stripping `Objective` would break the load-bearing case to fix a non-load-bearing one.
- **Verdict:** **Reject.** Wrong layer.

### Candidate C — filter `PriorReports` by `task_id` before passing to the finalizer

**What it would do:** in `builder.go`, filter `bus.StageReports` to only those whose `task_id == ac.CurrentTaskID` (would require adding `task_id` to `StageReport` first).

- **Reverse test:** would it harm anything? Yes — within a single multi-stage task, downstream stages benefit from upstream stage reports. The filter would have to be "same task OR earlier stage in current task", which is more complex than the current rule.
- **Class test:** does it generalize? It would help any case where Task N is a superset of Task N-1, which is the failure pattern here. But the right fix is "don't create Task N if it's a superset of Task N-1" (Candidate A), not "pretend Task N-1 didn't exist".
- **N test:** **N=1.** **Defer.**
- **Bait check:** this fix would *hide* the over-decomposition from the finalizer instead of fixing it. The duplicate output would go away but Task 1 and Task 2 would still both run, both consume LLM budget, and both produce outputs that the user sees. Cosmetic fix at the wrong layer.

## 4. Decision

**No fix shipped.** Record this as N=1 of an "analyzer over-decomposes a single-hop analysis question into a superset chain" failure pattern. Triggers for revisiting:

- **N=2 of the same pattern** — another analysis-policy run where Task K is a strict subset of Task K+1 and both are dispatched. At N=2, Candidate A becomes worth attempting, with the predicate for "fits in one pass" informed by what the two cases have in common.
- **A user complaint about pipeline cost or duplication** — would raise the priority but should still go through the audit.
- **A case where the duplication produces *contradictory* outputs** instead of just redundant ones. That would be a different failure pattern (cross-task drift) and would deserve its own investigation.

What is **not** a trigger for revisiting:

- Aesthetic dissatisfaction with the duplicate output on this single query. The system reached the right answer; the cost was extra LLM turns and a slightly redundant final report. That is a tolerable failure mode at N=1.

## 5. Reflections

**The audit stayed the hand at the right place.** Three fixes were tempting; all three are wrong at N=1. Candidate B is wrong at any N. Candidate C is wrong at any N. Candidate A is potentially right but needs a second data point to know what predicate to use. The instinct to "fix the symptom we just saw" is exactly what the over-fitting audit exists to slow down.

**The layer-by-layer breakdown is itself the artifact worth keeping.** Even without a fix, the analysis above pinpoints which propagation channels would matter if this pattern recurs. Future-me reading this doc on the N=2 trigger will not have to re-derive that `Objective` is load-bearing or that `PriorReports` filtering is a cosmetic fix at the wrong layer.

**Honest reliability transparency, again.** The prior session's lesson — "a system that is honestly 33% reliable on a hard query is better than a system that appears 100% reliable because the answer is leaked into the prompt" — has a sibling here: a system that produces honestly redundant output on an over-decomposed question is better than a system that hides the redundancy with a cosmetic filter while the underlying over-decomposition continues to burn LLM turns. Fix the cause if and when N=2 arrives; do not paper over the symptom.

## 6. Companion observation — `MissingPlan` leaks under `analysis` policy

A second N=1 finding from the same trace, recorded here because it surfaced from re-reading the same orchestrator log lines and shares the "harmless now, may matter later" character.

### What the log shows

```
task=1 step=0 stage=explore  missing=facts
task 1 transition: explore -> finalize
task=1 step=1 stage=finalize missing=plan          ← here
```

`missing=plan` appears at the `finalize` step under an `analysis` policy that does not allow the `plan` stage at all. Both tasks in this run produced the same line.

### Reconstruction

1. `explore` finished with `HasEnoughFacts == true`.
2. `internal/agent/explorer.go:117-122 DetermineMissingPiece` returned `MissingPlan` unconditionally on that signal:
   ```go
   if output.SignalUpdates != nil && output.SignalUpdates.HasEnoughFacts {
       return types.MissingPlan
   }
   ```
   This is **policy-blind** — it does not consult `determineActivePolicy()`.
3. `decideNextStage` (orchestrator.go:447) evaluated explore's out-edges:
   - `GetTransitions(explore)` returned `[explore→plan(100), explore→finalize(40), explore→explore(30)]`.
   - `filterByPolicy` (line 505) removed `explore→plan` because `plan` is not in the `analysis` policy's `AllowedStages`. **Plan was eliminated before signal evaluation ever ran.**
   - `filterBySignals` accepted `explore→finalize` (HasEnoughFacts) and `explore→explore` (fallback).
   - Selected `explore→finalize` at priority 40.
4. The transition log shows `explore -> finalize` directly. **Routing did not "try to enter plan"** in any actionable sense — `plan` was the highest-priority candidate from `GetTransitions` but was filtered out at the first policy gate, not the signal gate.

`missing=plan` is therefore **dead state**: written by the explorer, carried forward, but unable to influence routing because the only stage that could consume it is unreachable under this policy.

### Why it is not purely cosmetic

`internal/context/builder.go:221-226` renders `MissingPiece` into the agent prompt unconditionally:

```go
if ac.MissingPiece != types.MissingNone {
    pc.UserSections = append(pc.UserSections, types.PromptSection{
        Title:   "Missing Piece",
        Content: fmt.Sprintf("Currently missing: %s", ac.MissingPiece),
    })
}
```

So under `analysis` policy, the `finalize`-stage prompt to the LLM contains:

```
## Missing Piece
Currently missing: plan
```

The finalizer is the terminal stage. There is no future stage that can satisfy "missing: plan" under analysis policy. The line is at best ignored, at worst nudges the finalizer toward a "plan still pending" caveat that is structurally impossible to act on. This run produced the right answer in both tasks, so the leak did not cause visible damage — but it is observable noise in the prompt, not just in the log.

### Audit on candidate fixes

- **Make `explorer.DetermineMissingPiece` policy-aware** (return `MissingNone` if plan is forbidden): conceptually right but punches through the layer boundary — the explorer agent currently does not need to know about task policies, and adding the dependency for one return value is an over-fit at N=1. **Defer.**
- **Filter `MissingPiece` in `BuildPromptContext` against the active policy** (don't render a Missing Piece pointing to a forbidden stage): narrower, no layer-crossing on the agent side, but requires `AgentContext` to carry policy info it currently doesn't. Also N=1. **Defer.**
- **Suppress the rendered "Missing Piece" line at the `finalize` stage specifically** (terminal stages have nothing left to be missing): smallest change but is exactly the kind of stage-name-specific carve-out the over-fitting audit warns against. **Reject.**

### N=1 verdict

This is **the same trace, the same run, a second observation** — not an independent N=2. Both findings are recorded together and both deferred. Triggers for revisiting the missing-piece leak:

- **A finalizer output that visibly references the dead "missing: plan" state** (e.g. "no plan yet, more work needed" in an analysis answer). That would upgrade this from "noise in the prompt" to "noise in the user-visible output", which is a different impact tier.
- **Any second site where a `MissingPiece` is set but the corresponding stage is unreachable under the active policy.** A second instance of the same class makes the policy-blind `DetermineMissingPiece` design (not just the explorer's specific case) the right thing to fix.

The two findings in this doc are linked: the duplication in §1-§5 and the missing-piece leak here both stem from the same underlying shape — **per-stage agent decisions made without policy context, trusted to be filtered downstream**. If one of them recurs under N=2, the right fix may turn out to address both at once.
