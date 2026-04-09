# Investigation: §6 Follow-ups, Doc-Comment Bait, and the Decontamination Tradeoff

> **Date:** 2026-04-09 (third investigation in the explorer-knowledge-flow series)
> **Trigger context:** the five lower-priority issues left unfixed at the end of `investigation-grep-readfile-tool-defects.md` §6
> **Outcome:** Three fixes shipped (commits `9e67cc2`, `466240c`); two reverts inside the same session caught by audit; reliability transparency chosen over apparent stability; the over-fitting audit formalized into a five-test rule and saved to memory as a durable behavioral instruction.

This document continues the arc started in `investigation-explorer-knowledge-flow.md` and `investigation-grep-readfile-tool-defects.md`. Where the second doc was about Layer 4 tool defects (smart-case grep, small-file `read_file` passthrough), this one is about everything that happened *after* those tool defects shipped. It is preserved as a worked example of how the over-fitting audit catches its own author at three layers (skill prompt, doc comment, and tool design), and as the record of the decision to accept honest reliability over illusory stability.

## 1. The starting state

After commit `91de97f` shipped Fixes I and J, the trigger query `"这个项目有多少个agent可以创建subagent？"` converged correctly across three runs. `investigation-grep-readfile-tool-defects.md` §6 enumerated five remaining issues that were deliberately left unfixed at that point:

| Code | Issue | Impact | Fix cost |
|---|---|---|---|
| A | Finalizer dilutes precise answers into templated prose | HIGH | MEDIUM |
| B | Continuation budget = 1 too tight for two-step recovery | MEDIUM | LOW |
| C | Explorer's `HasEnoughFacts` floor `≥2 distinct sources` is too strict | LOW | LOW |
| D | Cataloger drift in explorer OutputFormat | LOW-MED | MEDIUM |
| E | `propose_sub_agents` injection by name match is undocumented | LOW (doc debt) | LOW |

The user asked to "fix what can be fixed". The session attacked four of the five (A, B, D, E) and deliberately left C alone. C is hypothetical — no failing case has surfaced — and relaxing the floor risks re-introducing the trivial-floor regression that the prior session shipped Fix A+B specifically to prevent. **Touch only on demonstrated failure** turned out to be the only one of the five decisions that needed no later correction.

## 2. Fixes A+D — finalizer skill routing and explorer few-shot OutputFormat

The default `final-answer-skill` workflow is `summarize all changes / compile patch information / write usage instructions / list action steps / mark tasks complete` — verbs that presuppose a code change actually happened. Forcing an analysis answer through that template was the source of the verbose templated prose with invented "Action Steps" and "Tasks Completion" sections observed in the prior session's verification.

The fix had two pieces:

1. A new sibling skill `analysis-final-answer-skill` registered in `internal/skill/defaults.go`, with a Q-and-A shaped workflow.
2. A skill-name override in `internal/orchestrator/orchestrator.go:dispatchStage`: when the active policy is `analysis` and the stage is `finalize`, use `analysis-final-answer-skill` instead of the configured DefaultSkill. Implementation pipelines keep the original skill unchanged.

For Fix D the explorer's `repo-explore-skill` got the same shape-by-example treatment because the explorer's stage report flows into the finalizer and they need to share a format.

A regression test `TestRun_FinalizeSkillRoutedByPolicy` was added to lock the routing in both directions — analysis policy must land on `analysis-final-answer-skill`, implementation policy must keep the configured DefaultSkill. The test uses mock agents that inspect the `*skill.Config` parameter their Execute receives, capturing the routed name without depending on actual LLM behavior.

The first version of these skills is where the trouble started. See §4.

## 3. Fix B — continuation budget 1 → 2 with differentiated prompts

`explorer.go:ContinuationPrompt` was returning `false` once `continuationCount >= 1`. The fix raises the budget to 2 with two textually different prompts:

- **First push**: "do it now" — catches the common case where the LLM thought aloud about the next step but did not act.
- **Second push**: "if your last action did not get you closer, try a *different* approach — and if you still do not have a real answer, say so honestly in one sentence" — catches the case where the recovery action from the first push was itself wrong, and gives an explicit exit for the "honest weakness > dishonest confidence" pattern from the prior session.

The third soft-stop is accepted unconditionally; at that point the loop has had two corrective opportunities and another push would just be churn.

This fix is the cleanest one in this session — completely generic, no codebase-specific text, no examples, no hypotheticals. It is the only fix in the second half of this session that survived its first attempt without revision.

## 4. Fix E — the doc-comment over-fit, twice

This is the section worth reading in full. It is the most instructive failure of the session.

### 4.1 First attempt — the hypothetical example

The original 3-line comment at `agent.go:294` was:

```go
// Auto-inject propose_sub_agents if a sub-agent with the same name as this
// agent is registered. The schema's sub_agent enum is scoped to [self name],
// so the agent can only propose sub-tasks for its own kind.
```

This is concise, accurate, and complete. It should have been left alone. Instead, the first attempt rewrote it into a 22-line doc comment that included a hypothetical example for how to add a new sub-agent capability:

> Today this means: only the main agent named "explorer" can propose sub_agents, because SubExplorer.Name() returns "explorer" and no other SubAgent is registered. **To grant a new main agent (say, "implementer") the same capability, add a SubAgent whose Name() returns "implementer" to RegisterDefaultSubAgents** — no other change is needed.

Verification run 1 produced:

> **Answer:** There are two agents in this project that can propose subagents: the "explorer" and "implementer" agents.
>
> **Evidence:**
> - internal/agent/agent.go:311 — The comment indicates adding a SubAgent whose Name returns "implementer" allows it to propose subagents.

**The LLM read the hypothetical `(say, "implementer")` example as a current registration**, not as a hypothetical. From the LLM's perspective the doc comment said "implementer can propose subagents", so its answer was "two agents: explorer and implementer".

This is not a bug in the LLM. It is a writing failure: hypothetical examples in code comments are bait. They look like documentation but they introduce additional symbol names that the LLM can mis-attribute.

### 4.2 Second attempt — the absence statement

The doc comment was rewritten to remove the speculative example. Instead it described what is intentionally absent:

> The check is implicit and string-based on purpose: there is no separate allow-list config and **no `CanProposeSubAgents()` method on the agent type**.

Verification run 1 produced:

> **Answer:** The project does not have any agents with a `CanProposeSubAgents` method indicating capability to create subagents.
>
> **Evidence:**
> - internal/agent/agent.go:300 — Comment notes that there is no `CanProposeSubAgents()` method on any agent type.

**The LLM read the absence statement "no `CanProposeSubAgents()` method" as a positive answer to a different question.** The comment was trying to explain *why* the design is intentional; the LLM extracted the substring `CanProposeSubAgents` as a relevant search term, found that it does not exist, and reported that as the answer.

This is also not a bug in the LLM. It is the same writing failure at one level of abstraction higher: **doc comments that explain "why X is intentionally absent" introduce X as a token the LLM treats as load-bearing**, and the absence becomes a positive fact.

### 4.3 The revert

After the second failure the fix was reverted to the original 3-line comment. Audit verdict: not every piece of code benefits from elaborated documentation. The original comment was already accurate; both rewrites were strictly worse for the LLM reader. The lesson:

> **Doc comments that explain "why X is NOT done" or that include hypothetical examples are LLM-misreading bait.** Use only positive statements about what *is*, never about what *isn't* or what *might be*.

The two failed versions of Fix E never reached commit; they exist only in the verification logs. The audit caught them inside the same session by re-running the trigger query after each version.

## 5. The few-shot example contamination

### 5.1 The contamination

The first version of `analysis-final-answer-skill` and `repo-explore-skill` (commit `9e67cc2`) included a single concrete few-shot example to bind the LLM away from cataloger sections. The example was:

```
**Answer:** There is one SubAgent implementation in the project, `SubExplorer`,
registered as the default subagent.

**Evidence:**
- internal/agent/subagent.go:63-66 — RegisterDefaultSubAgents adds NewSubExplorer(deps) to the registry
- internal/agent/sub_explorer.go:14 — SubExplorer type definition
```

**This is the trigger query's answer, verbatim, with file:line citations from the actual codebase.** Verification runs that produced "one agent, SubExplorer" with the right file:line citation were *substantially template-copying the answer the LLM saw in its own system prompt*, not independently investigating the codebase.

The user surfaced this in two questions: "本次修改没有过拟合问题吗？" and "如果用户希望展开详细解释流程会怎样". Both questions exposed the same flaw from different angles:

- The first question forced the audit. The example was the trigger query's answer; the prohibitions list was a catalog of the trigger query's specific failure modes. Both were instance-fixes dressed as class-fixes.
- The second question exposed that the OutputFormat pinned the answer length at "one or two sentences", which compressed any genuine multi-paragraph explanation into a degenerate two-sentence summary plus a file list. The format was fitted to the trigger query and would actively damage explanation-class queries.

### 5.2 The decontamination

The fix was to replace the single trigger-query example with three few-shot examples covering three answer scales (lookup, count, multi-paragraph explanation), all using content from a hypothetical unrelated codebase: a Go version lookup, an HTTP handler count, and a cache write-through architecture explanation. None of the example content overlaps with any symbol, file, or concept from this repository.

Above the examples sits a single principle in plain English: *"Match the answer's depth to the question's depth — one sentence for a count/name/yes-no question, multiple paragraphs for an explanation/walkthrough."*

The prohibitions were softened: the previous version listed five specific banned section names (`Summary / Changes / Conclusion / Action Steps / Instructions`) which is the same absence-as-positive-evidence bait that bit Fix E, and which also fights against legitimate use of those headers when the user genuinely asks for a changelog. The new prohibitions describe behavioral anti-patterns ("do not invent next steps the user did not ask for", "do not substitute 'further investigation needed' for an answer the prior stages already established") without naming specific banned headers.

### 5.3 The reliability cost

Removing the contamination dropped the trigger query reliability from ~100% (3-of-3 in commit `9e67cc2`'s verification) to ~33% (3 runs after `466240c`):

| Run | Behavior |
|---|---|
| 1 | Vague generalization ("any agent that can utilize the ProposeSubAgents tool"), no count, partially correct |
| 2 | **Hallucinated**: "Two agents: the `Planner` agent and any agent that has a `SubAgentRegistry` with matching names registered." Planner has no registered SubAgent and cannot propose subagents. Confidently wrong. |
| 3 | **Honest "no direct evidence"** + Caveat naming the missing piece. |

**This is the system's real reliability ceiling on this multi-hop synthesis query without scaffolding.** The previous "stable" verification was illusion; the LLM was template-copying the answer from its own prompt.

The trade-off accepted in commit `466240c` is **reliability transparency over apparent stability**. Run 3's "no direct evidence" + Caveat output is the right behavior — the new finalizer skill's optional Caveat slot exists precisely to surface "the prior stages did not pin this down" without padding or hallucination. That is the "honest weakness > dishonest confidence" exit from the prior session firing correctly for the first time since it was added.

A separate verification with an explanation-class query (`"解释 orchestrator 是怎么决定下一步走哪个 stage 的，包括 priority weight、policy filter 和 signal 这三者之间的关系"`) confirmed the new format scales correctly to multi-paragraph output with internal numbered structure and file:line citations grounding each load-bearing claim. **Format is now genuinely two-sided: it works for both short factual answers and long architectural explanations**, and that two-sided behavior is achieved without contamination.

## 6. Two more over-fits considered and rejected

### 6.1 Producer/consumer multi-hop hint (rejected: N=1)

Candidate: add a generic line to the explorer's workflow saying "When the question asks about a runtime relationship between two things (e.g. 'which X uses Y'), trace both sides — find the producer and the consumer separately and cross-reference. A single grep usually only sees one side."

This is completely generic. It does not name any symbol, file, or concept from this codebase. It would help the trigger query's multi-hop synthesis pattern. It looks defensible.

It was rejected because **N=1**. One query failing on producer-consumer multi-hop is not yet a pattern. It might be jitter, query-specific difficulty, or a one-off. Adding scaffolding for it now would be the same kind of N=1 over-fitting that Fix E demonstrated twice. The threshold rule: **wait for N=2 of the same failure pattern before generalizing**. If a second producer/consumer query fails in a future session, that establishes a real pattern worth addressing.

### 6.2 `list_subagents` introspection tool (rejected: illusion-creation)

Candidate: add a Layer 4 tool that reads the SubAgentRegistry directly and returns a list of registered sub-agents. The trigger query would then "pass" reliably because the explorer could call one tool and get the answer without needing multi-hop synthesis.

The user identified this as illusion-creation. The reasoning:

| Mechanism | Surface | What actually happens |
|---|---|---|
| Trigger query embedded in few-shot example | "the system can answer this question" | LLM reads the answer from its own prompt |
| `list_subagents` tool added | "the system can answer this question" | LLM calls a hand-built API whose only purpose is answering this query class |

Both make the trigger query pass without improving the LLM's actual reasoning capability. Both look like "system improvements" but are really test rigging at different layers — prompt layer for contamination, tool layer for `list_subagents`. The structural test that distinguishes a legitimate tool from an over-fitted one is:

> **If you deleted the failing query that motivated this tool, would you still want to add it?**

For `read_file`, `grep`, `repo_map`, and the smart-case fix from the prior commit, the answer is clearly yes — they handle classes of problems that any codebase exploration needs. For `list_subagents`, the answer is no. Without the trigger query, no-one would think to add a tool that introspects the SubAgentRegistry specifically. Its only justification is the failing test.

Both candidates passed the surface test ("does this code change look reasonable?") and failed the over-fitting audit. They were rejected before any code was written.

## 7. The over-fitting audit, formalized

After the second Fix E failure and the few-shot contamination discovery, the user gave a durable instruction: **"永远记住'过拟合的方案不要写'"** ("Always remember: don't write over-fitted solutions"). This was saved to the project's memory system as `feedback_no_overfitted_solutions.md`. The full text of the audit is recorded there; the headline rules are:

**Five tests every fix must pass before commit:**

1. **Reverse test** — Could this fix apply to a different problem (different query, different file, different language, different codebase)? If it only makes sense for the specific failing case, it is over-fitted.
2. **Deletion test** — If you deleted the failing query that triggered this fix, would you still want this fix in the codebase? If no, the fix exists *only* to make that one test pass.
3. **Class test** — Does this fix improve a class of problems or one specific instance?
4. **No-bait test** (for prompts/comments) — Does the text contain hypothetical examples, absence statements, or specific symbol names from the failing query that the LLM might mis-read as facts? Use only positive statements about what *is*.
5. **No-contamination test** (for examples) — If you're writing a few-shot example, does it use content from the codebase under test, or from a completely unrelated hypothetical codebase?

**Threshold rules:**

- **N=1 → wait, N=2 → generalize.** When you observe one failure of a pattern, do not add general scaffolding for it. Wait for the second instance to establish a real pattern.
- **Reliability transparency over apparent stability.** A system that is honestly 33% reliable on a hard query (with the right "honest weakness > dishonest confidence" exit firing) is better than a system that appears 100% reliable because the answer is leaked into prompts/tools/comments.

**The strongest signal that you are about to over-fit:** the only argument for the change is "this would make the failing query pass". If you cannot articulate a benefit independent of the specific failing query, you are about to over-fit.

The current `internal/skill/defaults.go` working tree was audited against these five tests before commit `466240c`:

| Test | Result |
|---|---|
| Reverse — would the same skill changes apply to a Python or Rust project? | ✓ Examples are HTTP framework / cache / Go version, applicable to any project |
| Deletion — without the trigger query, would these changes still be wanted? | ✓ They fix the finalizer template-dilution class problem |
| Class — fixing a class or an instance? | ✓ Class (any analysis-policy question) |
| No-bait — hypothetical examples, absence statements, or codebase symbols? | ✓ All-positive descriptions, examples are topic-disjoint |
| No-contamination — examples drawn from this codebase or hypothetical? | ✓ HTTP handlers / cache / Go version, zero overlap with agent / subagent / pipeline |

5/5. The decontamination commit shipped.

## 8. Reflections

**The over-fitting audit caught its own author at three layers in one session.** The doc-comment over-fit (Fix E) happened at the comment layer. The few-shot contamination happened at the prompt layer. The `list_subagents` candidate would have happened at the tool layer. Each was caught by a different mechanism: Fix E by the verification reruns, the few-shot contamination by the user's pointed audit question, and `list_subagents` by the user's "C 是在制造假象吧" challenge. **The audit is not optional and not a one-time check; it has to fire on every fix and every layer because over-fitting moves around.** The previous session's lesson "the audit is now load-bearing" turned out to be an understatement.

**Reliability transparency is a real feature, not just an excuse.** The decontaminated trigger query reliability of ~33% is a worse number than ~100%, but it is a *true* number about what the system can and cannot do. A system that appears 100% reliable because the answer is in the prompt is not a 100% reliable system; it is a 0% reliable system that has been hand-fed the test answer. The honest 33% number is more useful for deciding what to fix next, what to trust, and what to scaffold. The "honest weakness > dishonest confidence" exit is the mechanism that converts this into observable behavior — Run 3's `"no direct evidence" + Caveat` is the system telling the truth about a hard query, and that is the right behavior.

**N=1 is not a pattern.** Two candidate fixes were rejected this session for being N=1 generalizations (the producer/consumer hint, and to a lesser extent any "synthesis-class" scaffolding). The temptation to fix things proactively is real, especially when a generic-looking fix is one line away. The discipline of waiting for N=2 is what separates real generalization from preemptive over-fitting. If the same failure happens again in a future session, the case for a generic fix becomes much stronger.

**Doc comments are LLM input.** This is the most surprising lesson of the session. The LLM reads the entire codebase, including comments, and treats them as evidence. A comment that says "this is intentionally absent" introduces "this" as a token the LLM will treat as load-bearing. A comment that gives a hypothetical example introduces the hypothetical entities as if they were real. **The audit's no-bait test now applies to code comments, not just to prompt text.** For most comments this is not a problem because they describe what the code does. For comments that explain design rationale (why X was chosen, why Y was not), the bait risk is high.

**The Caveat slot earned its keep on its first real test.** It was added in the same commit (`9e67cc2`) that introduced `analysis-final-answer-skill`, and it sat unused through the contaminated verification because the contamination meant no LLM run hit a genuine "I don't know" state. The decontamination produced exactly that state in Run 3, and the LLM correctly used the optional Caveat slot to surface what it could not determine. This is the canonical worked example of why optional output slots matter: they have to exist before they are needed, even though they look like dead weight in the verification runs that don't trigger them.
