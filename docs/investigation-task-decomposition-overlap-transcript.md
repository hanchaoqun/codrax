# Investigation Transcript — Task-Decomposition Overlap and MissingPlan Leak

> **Date:** 2026-04-09
> **Companion document:** `investigation-task-decomposition-overlap.md`
> **Trigger query:** "这个项目中的pipeline有几种状态，变迁逻辑是怎样的？"

A faithful (though condensed) transcript of the live session that produced the two N=1 observations recorded in the companion doc. User turns are verbatim; assistant turns are paraphrased to keep the file readable, with key technical content and tool actions preserved.

---

## Turn 1 — read recent commits, run the trigger query

**User:**
> 读一下我们最近的6次提交，然后尝试运行输入"这个项目中的pipeline有几种状态，变迁逻辑是怎样的？"

**Assistant:** Ran `git log -6 --format="%h %s%n%b%n---"` and read all six commit messages back to `8e321ff`. Then ran:

```
go run . -config config/orchestrator.yaml -repo . -branch main \
  -request "这个项目中的pipeline有几种状态，变迁逻辑是怎样的？" -max-steps 50
```

Trace `trace-1775777029128134356` completed successfully. Pipeline produced **two tasks** under the analysis policy, both routed `explore → finalize`, both finalized via `analysis-final-answer-skill` (the routing fix from `9e67cc2` firing correctly):

```
task=1 step=0 stage=explore  → repo-explore-skill
task 1 transition: explore -> finalize
task=1 step=1 stage=finalize → analysis-final-answer-skill
task=2 step=0 stage=explore  → repo-explore-skill
task 2 transition: explore -> finalize
task=2 step=1 stage=finalize → analysis-final-answer-skill
```

Both tasks reached the correct substantive answer: 8 stages (`analyze → explore → plan → design_review → implement → code_review → verify → finalize`), priority-weighted transitions filtered by task policy, with citations to `config/orchestrator.yaml:16-190`, `internal/types/enums.go:7-14`, and `internal/orchestrator/orchestrator.go:447-448`. Task 2 additionally produced a per-stage transition table with priority numbers.

Flagged in the response: the two task outputs are largely the same content; if the user wants to act on the per-stage numbers I recommended verifying against the actual yaml first per the CLAUDE.md "verify before recommending" rule.

---

## Turn 2 — why are the two task outputs the same?

**User:**
> 两个task输出的内容一样，是什么原因

**Assistant:** Read `internal/context/builder.go` end-to-end to see exactly what each task's prompt looks like. Returned a three-layer breakdown:

**Layer 1 — analyzer over-decomposed.** `task-analysis-skill` split a single-hop question into `Identify pipeline states` and `Analyze state transition logic`, where Task 1 is a strict subset of Task 2. The question is one indivisible "explain the state machine" ask. **This is the root cause.**

**Layer 2 — per-task prompt scope is weak** (`builder.go:158-170`). Every task sees the full original `Objective` plus a one-line `CurrentTask` title. There is no "stay in your lane" instruction. The LLM defaults to answering the larger of the two prompts. *Not obviously a bug:* implementation flows depend on every agent seeing the original ask; adding a hard "stay in lane" constraint is exactly the over-fit prompt-bait the prior session's Fix E reverts caught.

**Layer 3 — `PriorReports` is append-only across tasks** (`builder.go:50-53`). Task 2's finalizer sees Task 1's explore + finalize stage reports plus its own explore. Since Task 1 is a subset of Task 2, the input already contains a complete answer, so the output naturally restates it. **This explains why Task 2 is the more detailed one** (3 stage reports vs Task 1's 1). *Also not a bug:* the comment in builder.go explicitly says agents should read upstream reports rather than re-derive from raw tool dumps.

Audited three candidate fixes:

- **A** — analyzer prefers single-task under analysis policy: potentially right but needs N=2 to know what predicate to use for "fits in one pass". **Defer.**
- **B** — strip `Objective` from non-leading tasks: fails reverse test (implementation flows need it on every stage). **Reject at any N.**
- **C** — filter `PriorReports` by `task_id` in finalizer: cosmetic fix at the wrong layer — would hide the over-decomposition without preventing the duplicate task dispatch. **Reject.**

Recommendation: no fix, record as N=1, wait for N=2 of the same superset-chain pattern before acting.

---

## Turn 3 — record to a doc

**User:**
> OK，先记录问题到文档

**Assistant:** Listed `docs/`, found the existing investigation doc series. Read the first 40 lines of `investigation-followup-fixes-and-overfitting.md` to match the doc style. Wrote `docs/investigation-task-decomposition-overlap.md` with five sections:

1. What was observed (trace ID, pipeline log lines, the asymmetry that Task 2 was more detailed than Task 1).
2. Why the duplication happens — the three-layer breakdown.
3. Audit on three candidate fixes — verdicts: A defer, B reject, C reject.
4. Decision — no fix, N=1, explicit triggers for revisiting written out.
5. Reflections — the audit stayed the hand at the right place; the layer breakdown is the artifact to preserve even without a fix; honest reliability transparency applies to redundancy too.

Triggers for revisiting deliberately exclude "aesthetic dissatisfaction with this single run" — system reached the right answer, the cost was extra LLM turns, tolerable at N=1.

---

## Turn 4 — commit

**User:**
> 更新并提交

**Assistant:** Checked status, staged the new doc, committed as `48ca41f` with a HEREDOC commit message summarizing the three layers, the three rejected/deferred fixes, and the no-code-change decision. Noted memory index does not need updating — the no-fix decision was driven by the existing `feedback_no_overfitted_solutions.md` rule.

---

## Turn 5 — did routing try to enter plan?

**User:**
> 再看下路由有次是不是试图进plan

**Assistant:** Re-read the orchestrator log lines and noticed the tell:

```
task=1 step=1 stage=finalize missing=plan
```

`missing=plan` showing up at finalize under an analysis policy that does not allow plan stage at all. Investigated through `internal/agent/explorer.go:117-122` (`DetermineMissingPiece` returns `MissingPlan` unconditionally on `HasEnoughFacts`) and `internal/orchestrator/orchestrator.go:447-599` (`decideNextStage` → `filterByPolicy` → `filterBySignals`).

Reconstructed the actual sequence:

1. Explore finished with `HasEnoughFacts == true`.
2. `DetermineMissingPiece` returned `MissingPlan` — **policy-blind**.
3. `decideNextStage`:
   - `GetTransitions(explore)` → `[explore→plan(100), explore→finalize(40), explore→explore(30)]`.
   - `filterByPolicy` removed `explore→plan` because plan ∉ analysis policy's `AllowedStages`. **Plan was eliminated at the policy gate, never reached the signal gate.**
   - Selected `explore→finalize` at priority 40.

Answer to the user's question: **routing did not actually try to enter plan** in any actionable sense — plan was the highest-priority candidate but was filtered out before signals were even evaluated. However, `missing=plan` is **dead state**: written by the explorer, carried forward in `TaskState.Missing`, but unable to influence routing because plan stage is unreachable under this policy.

Then noted it is **not purely cosmetic**: `context/builder.go:221-226` renders `MissingPiece` into the agent prompt unconditionally, so the analysis-policy finalizer's prompt contains a `## Missing Piece\nCurrently missing: plan` section — referencing a stage that is structurally unreachable from where the finalizer stands. This run produced correct answers, so the leak did not cause visible damage, but it is real prompt noise.

Audited three more candidates:
- **Make `DetermineMissingPiece` policy-aware:** punches the policy abstraction through to the agent layer for one return value. N=1, defer.
- **Filter `MissingPiece` in `BuildPromptContext` against active policy:** narrower, but `AgentContext` does not currently carry policy. N=1, defer.
- **Suppress `Missing Piece` at finalize stage specifically:** stage-name-specific carve-out, exactly what the audit warns against. **Reject.**

Recommendation: append as a companion observation to the same doc, since it is the same trace and shares the "harmless now, may matter later" character.

---

## Turn 6 — append the second observation

**User:**
> 追加

**Assistant:** Edited `investigation-task-decomposition-overlap.md` to add §6 "Companion observation — `MissingPlan` leaks under `analysis` policy", containing: the log line that triggered the question, the reconstruction showing plan was eliminated at the policy gate not the signal gate, the explanation of why it is not purely cosmetic (prompt rendering), the audit on the three candidate fixes, and the N=1 verdict with revisit triggers.

The closing note connects §6 back to §1-§5: both findings stem from the same underlying shape — **per-stage agent decisions made without policy context, trusted to be filtered downstream**. If either recurs at N=2, the right fix may address both.

Committed as `5873745`.

---

## Turn 7 — save the transcript

**User:**
> 好，我们本次的对话原稿也保存并提交

**Assistant:** *(this file)*

---

## Reflections on this session

**The trigger query was a side-channel finding, not the question.** The user asked the system to answer a real architectural question. The system answered it correctly. But the *shape* of how it answered — two tasks where one would do, a `missing=plan` line that should not exist under analysis policy — surfaced two structural observations that reading the code in isolation would have missed. The same pattern as the prior session: production traces are the highest-value diagnostic signal at Layers 1-3 because they show the system actually running the path, not just the path it could take.

**Two findings, one trace, still N=1 on each.** The temptation to count them as two separate data points and start fixing was real. The companion structure resists this: §6 explicitly says "this is the same trace, the same run, a second observation — not an independent N=2". Both are deferred. The over-fitting audit's `N=1 → wait, N=2 → generalize` rule cuts cleanly here; ignoring it because two findings showed up together would be exactly the rationalization the rule exists to block.

**The two findings are linked, and that is more interesting than either alone.** Both are "per-stage agent decisions made without policy context, trusted to be filtered downstream". `task-analysis-skill` decomposes without checking what the policy will allow; `explorer.DetermineMissingPiece` writes a missing piece without checking what the policy will allow. The orchestrator catches both at filter time, so the system works — but the agent layer is generating decisions that are dead on arrival under analysis policy. If a third instance of this class shows up, the right fix is probably structural (give agents read access to active policy) rather than per-site patches.

**Investigation docs without code changes are still load-bearing.** Future-me on the N=2 trigger does not have to re-derive the three layers, the three rejected candidate fixes for the duplication, or the three rejected candidate fixes for the missing-piece leak. The docs are the substrate that makes "wait for N=2" actually pay off — without them, N=2 would re-start from zero analysis instead of building on top of the §1-§5 / §6 audit.

**No memory update was needed.** The behavioral rule that drove every decision in this session — `feedback_no_overfitted_solutions.md`'s N=1 → wait — already exists. Adding a new memory for "task-decomposition can produce duplicate output" or "MissingPlan can leak under analysis policy" would be exactly the kind of one-time-fact memory that the memory guidance warns against. The investigation doc is the right home for those facts.
