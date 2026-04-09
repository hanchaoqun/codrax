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

## 7. What is in the tree right now

**Committed (`2f99b8b`):**
- A: `HasEnoughFacts` requires ≥2 distinct tool sources
- B: `explore→finalize` requires `HasEnoughFacts`
- C: `StageReport` channel end-to-end
- D-1: `MaxInlineBytes` 4 KB → 32 KB
- D-2: `read_file` head-only line-aware preview
- E: `RetryHint` channel end-to-end
- F (mechanism): cumulative hint principle, principle-only text

**Not committed:**
- Diagnostic logging in `BaseAgent.Execute` (`[diag ...]` lines and `truncForLog` helper) — left in place for any follow-up runs; should be removed before the next commit.

**Not yet implemented:**
- Path A or Path B from §5.
