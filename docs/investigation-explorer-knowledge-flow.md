# Investigation: Explorer Knowledge Flow

> **Date:** 2026-04-09
> **Trigger query:** "这个项目有多少个agent可以创建subagent？"
> **Outcome:** 5 structural fixes shipped (commit `2f99b8b`); deeper architectural mismatch identified but not yet fixed.

This document records a debugging session that started with a single failing query and surfaced multiple layers of structural issues in the pipeline. It is preserved as a worked example of how to localize "deep" causes in this codebase, and as a reference for the architectural questions still open at the end.

## 1. The trigger

The user asked the running pipeline how many agents in this project can create subagents. The correct answer is **1** (only the `explorer` main agent, because `SubExplorer.Name()` returns `"explorer"` and `agent.go:219-236` auto-injects `propose_sub_agents` only for main agents that match a registered SubAgent name).

The first run produced "I couldn't find it, please review manually". The pipeline visited only `analyze → explore → finalize` and exited cleanly.

## 2. The investigation

Each round below diagnosed one cause, fixed it, and ran the same query again. Each round revealed a layer that the previous one had been hiding.

### Round 1 — explorer's "done" criterion was trivial

`explorer.go:ParseOutput` set `HasEnoughFacts = (len(facts) > 0)`. A single grep returning anything was enough to declare exploration complete. Combined with `explore→finalize` (priority 40) being unconditionally valid, even a thin exploration short-circuited into finalize.

**Fixes A, B (committed):**
- `HasEnoughFacts` requires ≥2 distinct tool sources (`internal/agent/explorer.go`).
- `explore→finalize` transition requires `HasEnoughFacts` true (`internal/orchestrator/orchestrator.go:isTransitionValidBySignals`). Without this, the lower-priority `explore→explore` self-loop never fires.

### Round 2 — explorer's narrative died at the end of its loop

After Fix A+B the explorer ran more tools but the answer was still wrong. Reading `explorer.go:ParseOutput` revealed the schema bug:

- The explorer's final assistant message (`lastContent`) was written into `StageOutput.Data`, which is dropped by `applyStageOutput`. Grep confirmed `output.Data` has no consumer outside `sub_explorer.go`.
- The only thing that survived was `RepoFact{Key: r.ToolName, Value: r.Summary, Source: r.ToolName}` — keyed by tool name, not by file or fact identity.
- Worse: `RepoFact.Source` was filled with the tool name, then `extractRelevantFiles` in `internal/context/builder.go` used `f.Source` as a *file path*, so the finalizer's "Relevant Files" list became `["grep", "read_file"]` — meaningless.
- The finalizer reconstructed its understanding from raw tool dumps because the LLM's synthesis from explore was simply gone.

**Fix C (committed): cross-stage knowledge channel.**

- `types.StageReport{Stage, Agent, Findings}` and `BusContext.StageReports` (`internal/types/context.go`).
- `StageOutput.StageReport` field, auto-populated by `BaseAgent.Execute` from the last assistant message after `ParseOutput` returns (`internal/agent/agent.go`).
- `applyStageOutput` appends to `BusContext.StageReports`.
- `BuildAgentContext` copies `bus.StageReports` to `ac.PriorReports`; `BuildPromptContext` renders them into a "Prior Stage Findings" user section.

This works for all 8 agents, not just the explorer. The implementer's plan, the verifier's failures, the code reviewer's findings — all were being thrown away by the same schema gap.

### Round 3 — `read_file` silently ate file middles

The answer was still wrong. Suspecting that the LLM never saw the relevant code, I checked `internal/tool/blob.go`:

```go
const MaxInlineBytes = 4096
const previewHeadBytes = 3072
const previewTailBytes = 512
```

Any file over ~88 lines was returned as "first 3 KB + last 0.5 KB", with the middle silently truncated. `internal/agent/agent.go` is 10 058 bytes / 330 lines. The auto-inject logic at line 219 starts at byte 6848 — **exactly in the truncated middle**. If the explorer had read it, it would have seen a head, a "[truncated 6474 bytes]" notice, and a tail — never the section that holds the answer.

The head+tail strategy makes sense for `exec_command` (startup banner + trailing error are both useful). It is actively harmful for `read_file` because source code's middles are content, and "head + tail" looks like a complete snippet to a model.

**Fix D (committed):**
- `MaxInlineBytes` raised to **32 KB** (`internal/tool/blob.go`). Most source files in this repo fit whole; `find . | wc -l` and friends still get bounded.
- New `StoreBlobHeadOnly` function that returns the head plus a **line-aware** notice ("showing lines 1-N of M; paginate with offset") instead of head+tail. `read_file` switched to this mode in `internal/tool/builtin.go`.
- `previewHeadBytes` / `previewTailBytes` rescaled to 24 KB / 4 KB to match the new threshold for tools that still want head+tail (`exec_command`).

### Round 4 — self-loops were open-loop

After Fix D the answer was still wrong (and now actively wrong: "There are no agents that can create subagents"). I added a one-line diagnostic to `executeTool` and saw the explorer's actual tool calls: `repo_map`, `grep`, `grep`, `list_files`. **Zero `read_file` calls.** The explorer was being asked about source semantics but never opened a source file.

I tightened the floor (Fix A2) to require at least one `read_file` or `exec_command` call. This blocked the navigation-only outcome — and revealed the next layer.

The trace now showed `explore → explore (5x)`. The explorer was looping but **using the same prompt every time**. It tried different grep regexes instead of upgrading to `read_file`. The orchestrator's self-loop was open-loop: the previous attempt's diagnosis was thrown away on each retry.

**Fix E (committed): retry feedback channel.**

- `TaskState.RetryHint` (`internal/types/task.go`).
- `StageOutput.RetryHint` field; the agent's evaluator fills it when the agent diagnoses its own incompleteness (`internal/agent/agent.go`).
- `applyStageOutput` writes it to `TaskState.RetryHint`; the per-task loop **clears it on any forward transition** so a hint never leaks across stage boundaries (`internal/orchestrator/orchestrator.go`).
- `AgentContext.RetryHint` mirror; `BuildPromptContext` renders it as a top-priority "Retry Directive (READ FIRST)" user section.

Generalizable to all 8 agents — any stage that self-loops can now tell its next dispatch what changed.

### Round 5 — thrashing on multi-criterion floor

After Fix E the explorer still oscillated. Trace showed:

```
attempt 1: 3 navigation → wentDeep=false → hint "use read_file"
attempt 2: 2 read_file  → sources=1     → hint "use different tool"
attempt 3: navigation   → wentDeep=false → hint "use read_file"
attempt 4: read_file    → sources=1     → hint "use different tool"
attempt 5: oscillation guard
```

Each retry fixed the most recently flagged criterion and broke the other. The hint was per-failure, not per-target.

**Fix F (committed, then partially rolled back):** Always state both criteria in a single hint, so the LLM knows it must satisfy both at once.

This made attempt 2 satisfy both (3 navigation + 2 read_file) and the explorer terminated cleanly without tripping the oscillation guard.

### Audit — removing over-fitting

I asked myself which of the fixes were generalizable structural improvements and which were prompt patches dressed up as structure. Audit findings:

| Fix | Verdict |
|---|---|
| A (≥2 sources) | Generalizable. Weak floor that doesn't assume question type. **Keep.** |
| A2 (must use `read_file`/`exec_command`) | Opinionated. Wrong for purely structural questions ("how many .go files", "is internal/agent a directory"). **Remove.** |
| `isDepthTool` excluding grep | Defensible but a value judgment; `grep -A 20` returns substantial content. **Remove with A2.** |
| B (transition gate) | Pure plumbing. **Keep.** |
| C (StageReport) | Pure infrastructure. **Keep.** |
| D-1 (32 KB threshold) | Magic number tuned to Go repos but a reasonable default. **Keep.** |
| D-2 (head-only `read_file`) | Principle-stating, not question-specific. **Keep.** |
| E (RetryHint plumbing) | Pure infrastructure. **Keep.** |
| F (cumulative hint principle) | Generalizable principle: multi-criteria checks should always state the full target. **Keep mechanism.** |
| F's actual prompt text | **Over-fitted.** Original wording: "pick 2-3 source files most likely to contain the answer based on the matches you already have, then call read_file on each. Do NOT revert to grep/list_files/repo_map without read_file." This assumes the question is about source code and prescribes `read_file` as the canonical tool — exactly the "if X-type question then Y-type action" pattern I had earlier criticized. **Rewrite as principle-only.** |

After the audit:
- A2, `isDepthTool`, and `wentDeep` were removed (`internal/agent/explorer.go`).
- The retry hint text was reduced to:
  ```
  "Previous attempt used fewer than 2 distinct tool types. The next
   attempt must use at least 2 different tools — choose them based
   on what the question actually needs."
  ```

**Cost of de-overfitting:** the floor became weaker. The original query, which Fix A2+F had pushed close to a correct answer, regressed. But the regression was a "I'm not sure" answer, not a confidently wrong one — a better failure mode than the intermediate state. Honest weakness > dishonest confidence.

## 3. Where this stops being a structural problem

After all the de-overfit fixes were in, I added a full ReAct transcript dump to `BaseAgent.Execute` (still in tree, unstaged) and re-ran. The trace showed:

- `iter=0`: `repo_map` (299 bytes) — directory tree
- `iter=1`: three parallel greps including `grep "SubAgent"` returning **12 813 bytes** that contained, in plain text:
  ```
  ./main.go:78:	agent.RegisterDefaultSubAgents(subAgentRegistry, deps)
  ```
  along with all the surrounding registry code in `main.go` and `propose_sub_agents.go`.
- `iter=2`: 1556 chars of assistant content — the LLM's synthesized findings — including:
  > "Functions like `NewSubAgentRegistry` and **`RegisterDefaultSubAgents`** are used to set up and register subagents."
  > "The function `RegisterDefaultSubAgents` is used to register default subagents."

Then it stopped. Without reading `subagent.go:RegisterDefaultSubAgents` (which would have shown `r.Register(NewSubExplorer(deps))` — exactly one registration).

**The LLM had the answer in its hands and chose to stop one tool call short.** It explicitly named the function that contains the answer. It then wrote a summary saying "you may need to look at how the SubAgentRegistry is used throughout the codebase". It was prescribing next steps to the user instead of doing them.

This is not a model capability failure. The LLM did exactly what its skill told it to do.

## 4. The deep architectural cause

The explorer is using `repo-explore-skill`, defined in `internal/skill/defaults.go:31-55`:

```go
Goal: "Build a trusted factual foundation about the codebase."
Workflow: [
    "find entry points",
    "grep for key functions",
    "build module map",
    "analyze call chains",
    "identify relevant files",
    "document findings as RepoFacts",
]
OutputFormat: "JSON with repo_facts, entrypoints, call_chains, relevant_files"
```

Six workflow steps, all about *locating* and *cataloging*. None about *answering*. The output format is structural metadata, not an answer field. **The skill defines the explorer as a cataloger, not an investigator.**

Combined with the second mismatch — `final-answer-skill` has `ToolSuggestions: []string{}` and `finalizerEvaluator.ShouldStop` returns `true` unconditionally on iteration 0 — analysis tasks (`analyze → explore → finalize`) have **no stage that actually investigates the question**:

- `analyze` writes the task list and stops.
- `explore` catalogs and stops.
- `finalize` reformats the catalog with no tools and no iterations.

For implementation tasks (`analyze → explore → plan → implement → verify → finalize`), the cataloger model is fine: `plan` and `implement` will read the cataloged files. For analysis tasks, the cataloger model leaves a hole at the end of the pipeline.

The LLM is doing its job. The pipeline contract is wrong.

## 5. Two paths forward (not yet implemented)

### Path A — reframe `repo-explore-skill` as investigation

Change `internal/skill/defaults.go:31-55`:

- **Goal:** "Investigate the user's question to a defensible, evidence-backed answer."
- **Workflow:** form a hypothesis → read code or run commands to verify → iterate until you have a defensible answer → record both the answer and supporting evidence.
- **OutputFormat:** include an `answer` field, not just `facts`.

Single point of change. Defensible as a contract fix rather than a prompt patch — "explorer" should mean investigator, regardless of question type. Slightly less efficient on the implementation pipeline (the explorer will go a bit deeper than a planner strictly needs), but not harmful.

### Path B — give the analysis-pipeline finalizer real teeth

Two changes:
- Remove the unconditional `return true` from `finalizerEvaluator.ShouldStop` so the finalizer can iterate when it has more to do.
- Add `read_file`, `grep`, `exec_command` to `final-answer-skill.ToolSuggestions`.

Preserves the explorer's current cataloger contract. Closes the gap precisely where it exists (the end of the analysis pipeline). The semantic story is "the finalizer verifies the explorer's report and fills in any gaps it admits to". Slightly more invasive than Path A; needs care to keep the finalizer from becoming a second explorer on implementation pipelines.

### Recommendation

**Path A.** Reasons:
1. The cataloger framing is wrong even on the implementation pipeline — `plan` having to re-discover code that explorer already touched is wasted work.
2. Single point of change.
3. "Explorer" meaning "investigator" is the more honest reading of the role.

Path B is the more conservative alternative if preserving the current explorer contract is valued.

## 6. Open questions

- **Should skills be polymorphic per task policy?** Earlier in the session I argued this was speculative abstraction with no consumer. The analysis-vs-implementation split here is the first real consumer. If we end up wanting different `explore` behavior per policy, the right home is per-skill evaluators, not per-agent.
- **Should the finalizer ever have tools?** Path B says yes; Path A says no. The decision is partly a question of whether "finalize" means "compile" or "verify".
- **Is `RepoFact.Source = r.ToolName` worth fixing?** It is a latent bug (downstream code uses Source as if it were a file path). Not blocking anything we observed, but the schema lies and someone will trip on it. Worth a small follow-up commit.
- **Should `read_file` of a known location be encouraged via prompt, or should we add an introspection tool** (`list_subagents`) so the explorer can query the registry directly? The latter is a Layer 4 capability addition rather than a fix; in the long run it is the right answer for "questions about runtime topology".

## 7. Resolution — Path A landed, plus two more layers found

We took **Path A** and ran the trigger query. The skill change moved the LLM's intent in the right direction but did not produce a correct answer, because two additional structural layers were hiding underneath:

### Layer 2 — `BaseAgent.Execute` hardcoded "soft stop"

Trace dump showed the explorer's iter=5 message saying:

> "I'll check for registrations that employ subagent capabilities next. Let's explore the `RegisterDefaultSubAgents` method for details."

…and then the loop broke. The cause was this clause in `internal/agent/agent.go`:

```go
if b.eval.ShouldStop(resp, i) || (len(resp.ToolCalls) == 0 && resp.Content != "") {
    break
}
```

The hardcoded `(len(toolCalls) == 0 && content != "")` rule treats any "thinking aloud" turn as completion. The skill says "investigate to an answer", but the loop overrides that contract whenever the LLM produces narration without acting. The skill drives the LLM's intent; the loop short-circuits its execution.

**Fix G — `ContinuingEvaluator` interface (committed).** A new optional interface:

```go
type ContinuingEvaluator interface {
    ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, history []types.ToolResult) (prompt string, shouldContinue bool)
}
```

`BaseAgent.Execute` now splits the stop logic into two paths:

1. **Hard stop** (`eval.ShouldStop` returns true) — finalizer's `return true` keeps its current "stop on iter=0" behavior.
2. **Soft stop** (no tool calls + content) — the loop checks whether the evaluator implements `ContinuingEvaluator`. If so, it asks for a continuation prompt; if returned, the prompt is appended as a user message and the loop runs another LLM turn instead of breaking.

`explorerEvaluator` implements the interface with a budget of one continuation per stage and a principle-stating prompt:

> "Your previous turn produced a summary without calling any tool. If you genuinely have a defensible answer, restate it in one sentence and stop. Otherwise, take the next concrete investigative step now — do not describe what you would do next, do it."

The prompt deliberately does not name a tool or a target. It states the principle ("don't summarize without acting"); the LLM picks the next move.

After Fix G, the explorer started actually reading files. But it read the wrong slices: `read_file agent.go offset=0 limit=40` returned the head of the file, the LLM saw the imports + type definitions + start of `Execute`, and concluded "I have enough" — without realizing that `agent.go` is 330 lines and the auto-inject logic is at line 219. It then asserted "all 8 agents can create subagents" (confidently wrong).

### Layer 3 — `read_file` returned slices with no metadata

`read_file path=X offset=0 limit=40` returned 40 lines of content with no indication that the file was 330 lines long. The LLM had no way to distinguish "this is a 40-line file" from "this is the head of a 330-line file". My earlier Fix D-2 (line-aware truncation) only fired when the file exceeded `MaxInlineBytes` (32 KB); files smaller than that, sliced via offset/limit, returned bare content.

**Fix H — line range banner (committed).** `internal/tool/builtin.go:ReadFile.Execute` now always prepends a banner:

```
[<path>: showing lines X-Y of N total]
<content>
```

Always present, regardless of slice or full read, blob or inline. Plain text, no advice. Just metadata the LLM cannot ignore.

### Final outcome

After Path A + Fix G + Fix H, the trigger query produced:

```
iter=0: repo_map
iter=1: list_files internal/agent
iter=2: grep "create subagent"           → 0 hits
iter=3: grep "SubAgent"                  → 6.4 KB (function name surfaces)
iter=4: grep "RegisterDefaultSubAgents"  → 216 bytes (file:line citations)
iter=5: read_file subagent.go offset=63 limit=20  ← targeted read at right line
iter=6: thinking-aloud summary → CONTINUE injected
iter=7: short final answer → soft stop accepted
```

Final answer:

> "There is currently one implemented subagent named `SubExplorer`, which is registered via `RegisterDefaultSubAgents` at lines 63-66 in `subagent.go` by adding `NewSubExplorer(deps)` to the registry."

Correct, precise, with file:line citations. The LLM used the line numbers from `grep`'s output (Fix H made these meaningful by training the model to expect line-addressed reads) to do a targeted `read_file` at the right offset, instead of starting at offset 0 and missing the relevant section.

### Three layers, three fixes

| Layer | Symptom | Fix |
|---|---|---|
| 1 — skill contract | LLM cataloged instead of investigating | Path A: reframe `repo-explore-skill` as investigation |
| 2 — agent loop | LLM said "I'll do X next" then was force-stopped | Fix G: `ContinuingEvaluator` interface + soft-stop split |
| 3 — tool metadata | LLM read 40 lines and assumed file complete | Fix H: `read_file` line range banner |

Each fix is generalizable beyond the trigger query:
- Path A's contract change benefits any analysis-pipeline question.
- Fix G's continuation hook benefits any agent whose LLM "thinks aloud" mid-investigation.
- Fix H's banner benefits any tool that returns sliced content.

### Audit of new code for over-fitting

| Item | Verdict |
|---|---|
| Path A skill text | "if you surface a load-bearing name, open it before drawing conclusions" — borderline (reactive to observed failure) but defensible as a principle ("a name is a hypothesis, not an answer"). Kept. |
| Path A prohibition | "do not stop at 'the answer would require checking X'" — quotes a phrase the LLM literally produced. Borderline. Kept because the principle (don't punt to the user) is general; the wording just makes it concrete. |
| Fix G continuation prompt | Principle-stating; no tool name, no target; provides an explicit escape hatch ("if you have a defensible answer, restate it and stop"). Clean. |
| Fix G budget = 1 | Magic number bounded by `MaxIterations`. Acceptable. |
| Fix H banner format | Pure metadata, no prescription. Clean. |

## 8. Open questions (carried over from §6)

- **Should `RepoFact.Source = r.ToolName` be fixed?** Still a latent bug. Worth a small follow-up.
- **Should we add an introspection tool** (`list_subagents`, `list_agents`) for "questions about runtime topology"? Long-term right answer for this class of question; would short-circuit the multi-grep dance.
- **Diagnostic logging in `BaseAgent.Execute`** has now been removed (the next commit). It was useful for the Layer 2 / Layer 3 discoveries; a future investigation could re-add a similar dump on demand via a `-debug` flag instead of an unconditional stderr stream.
