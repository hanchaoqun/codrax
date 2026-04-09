# Investigation Transcript — Explorer Knowledge Flow

> **Date:** 2026-04-09
> **Companion document:** `investigation-explorer-knowledge-flow.md`
> **Trigger query:** "这个项目有多少个agent可以创建subagent？"

This is a faithful (though condensed) record of the live debugging session that produced the structural fixes and investigation document. Preserved so future reviewers can see how the deep root causes were uncovered, and which judgment calls were made along the way. User turns are quoted verbatim; assistant turns are paraphrased to keep the file readable, with key technical content preserved.

---

## Turn 1 — first run

**User:**
> 运行一下，输入：这个项目有多少个agent可以创建subagent？

**Assistant:** Ran `go run . -config config/orchestrator.yaml -repo . -branch main -request "这个项目有多少个agent可以创建subagent？" -max-steps 50`. Pipeline went `analyze → explore → finalize` in 5 tool calls. Finalizer's answer was "I couldn't find it, please review manually" — the explorer cataloged a few subagent-related files but couldn't draw a conclusion. Noted that the startup log already prints `1 sub-agents`, which is the actual answer hiding in stderr.

---

## Turn 2 — locate the deep cause

**User:**
> 定位一下深层原因

**Assistant:** Investigated `internal/agent/agent.go:219` (auto-inject of `propose_sub_agents` only for main agents whose name matches a registered SubAgent), `internal/agent/sub_explorer.go:27` (`SubExplorer.Name()` returns `"explorer"`), and `main.go:78` (`RegisterDefaultSubAgents` registers exactly one SubAgent). Concluded the answer is **1**: only the explorer main agent.

Identified two surface causes for why the pipeline failed to reach this:
- **Knowledge fragmentation:** the answer requires correlating three files; single-keyword grep can't surface it.
- **Analysis policy is one-shot:** explore→finalize was unconditional, so when facts were thin the finalizer just surrendered.

Listed three improvements: (1) explorer prompt patch, (2) self-loop transition fix, (3) inject runtime topology into Facts.

---

## Turn 3 — challenge

**User:**
> 第三点为啥要做？

**Assistant:** Withdrew #3 — it violates layering (Layer 1 orchestrator should not directly write into Facts), pollutes the Facts schema, and is the wrong shape: if introspection is needed, it should be a tool the explorer calls, not orchestrator-injected state.

---

## Turn 4 — generalization check

**User:**
> 1、2的修改泛化吗

**Assistant:** Honest assessment:
- **#1 (prompt patch)** is over-fitted. Adding "must read source files for architectural questions" is exactly the kind of "if X-type question then Y-type action" patch that doesn't generalize.
- **#2 (self-loop transition)** is partially generalizable but not by simply adding a transition edge. The existing `explore→explore` edge exists; the issue is that `explore→finalize` always wins on priority. Need to fix the signal-based filter.

Suggested a more general fix: **explorer's `ShouldStop` should require "more than one tool source" before allowing voluntary exit**. This is a structural floor on exploration quality.

---

## Turn 5 — go ahead

**User:**
> 把能泛化的方案都加上试试

**Assistant:** Implemented:
- **Fix A:** `explorer.go:ParseOutput` → `HasEnoughFacts` requires ≥2 distinct tool sources.
- **Fix B:** `orchestrator.go:isTransitionValidBySignals` → `explore→finalize` requires `HasEnoughFacts`. The lower-priority `explore→explore` self-loop now wins when the floor isn't met, with the existing oscillation guard as the upper bound.

Build and tests passed. Re-ran the trigger query — the explorer used 4 tool calls this time but the finalizer still produced an inaccurate answer. The structural changes worked (explorer was forced to do more than one tool call), but the answer quality didn't improve in the way I'd hoped.

Honest report: "Structural fixes raised the floor on exploration quality but did not guarantee correctness on this question. The model did multi-tool exploration but still didn't connect the right files."

---

## Turn 6 — keep going

**User:**
> skill加评估器是泛化方案吗

**Assistant:** Conceded a terminology slip: I had said "skill 的评估器" but the codebase has agent-level evaluators, not skill-level. The question of whether to push evaluators down to the skill layer:

- **Theoretically generalizable** — different skills under the same agent could have different completion criteria, and the skill layer is the right home for "what counts as done".
- **Practically over-engineering right now** — every agent currently has exactly one skill, and there's no second consumer for per-skill evaluators. Speculative abstraction.

Recommendation: keep evaluators at the agent layer for now; revisit when a real consumer appears.

---

## Turn 7 — push deeper

**User:**
> 要不加点日志定位一下深层根因，我觉得需要泛化方案才能解决，不是llm的问题

**Assistant:** Before adding logs, read the data flow path (explorer → finalizer). Found three problems by reading code, not by logging:

1. **`StageOutput.Data` is silently dropped** by `applyStageOutput`. Grep confirmed `output.Data` has no consumer outside `sub_explorer.go`. The explorer's final synthesized narrative (`lastContent`) is captured into `Data` and then thrown away.
2. **`RepoFact.Source` is filled with the tool name**, but `extractRelevantFiles` in `internal/context/builder.go` uses it as a *file path*. The finalizer's "Relevant Files" list becomes `["grep", "read_file"]` — meaningless.
3. **No stage→stage findings channel.** Each stage's reasoning dies at the end of its ReAct loop; the next stage has only raw tool dumps to work from.

Proposed **Fix C: `StageReport` channel** — add a structured findings field that flows from each stage to the next. Generalizable to all 8 agents, not just the explorer.

---

## Turn 8

**User:**
> 修一下

**Assistant:** Implemented Fix C end-to-end:
- `types.StageReport{Stage, Agent, Findings}` and `BusContext.StageReports`.
- `StageOutput.StageReport` field, auto-populated by `BaseAgent.Execute` from the last assistant message after `ParseOutput` returns.
- `applyStageOutput` appends to `BusContext.StageReports`.
- `BuildAgentContext` copies into `ac.PriorReports`; `BuildPromptContext` renders as a "Prior Stage Findings" user section.

Build and tests passed. Re-ran the query — the answer was actually worse than before: "There are no agents that can create subagents" (confidently wrong). Investigated: the explorer didn't read files; my existing `wc` confirmed `agent.go` is 10058 bytes / 330 lines, with the auto-inject logic at line 219 — which would land in the *truncated middle* of the head+tail blob preview (`MaxInlineBytes = 4096`, head 3072 + tail 512).

Found **layer 2 of the structural problem**: `internal/tool/blob.go` was silently eating file middles. Proposed **Fix D**:
- D-1: raise `MaxInlineBytes` from 4 KB → 32 KB so most source files pass through whole.
- D-2: `read_file` uses a head-only preview with line-aware "showing lines X-Y of N" notice instead of the misleading head+tail dump. Source code's middles are content; head+tail makes sense for `exec_command` but not `read_file`.

Implemented and tested. Re-ran — the answer was still wrong. Added a one-line diagnostic to `executeTool` and saw the explorer's actual tool calls: 0 `read_file` calls. The explorer was being asked about source semantics but never opened a source file.

Tightened the floor with **Fix A2**: require at least one `read_file` or `exec_command` call (`isDepthTool`). This blocked navigation-only outcomes — and revealed the next layer.

The trace now showed `explore → explore (5x)` and the oscillation guard tripping. The explorer was looping but using the same prompt every time. Tried different grep regexes instead of upgrading to `read_file`. The orchestrator's self-loop was open-loop: previous attempt's diagnosis was thrown away on each retry.

Implemented **Fix E: `RetryHint` channel**:
- `TaskState.RetryHint` and `StageOutput.RetryHint`.
- `applyStageOutput` writes to `TaskState.RetryHint`; the per-task loop clears it on any forward transition (so a hint never leaks across stage boundaries).
- `AgentContext.RetryHint` mirror; rendered as a top-priority "Retry Directive (READ FIRST)" user section.
- `explorerEvaluator.ParseOutput` fills the hint when the floor isn't met.

Re-ran. Now the explorer was thrashing between two failure modes:
```
attempt 1: 3 navigation → wentDeep=false → hint "use read_file"
attempt 2: 2 read_file  → sources=1     → hint "use different tool"
attempt 3: navigation   → wentDeep=false → hint "use read_file"
attempt 4: read_file    → sources=1     → hint "use different tool"
attempt 5: oscillation guard
```

Each retry fixed the most-recently-flagged criterion and broke the other. **Fix F**: state both criteria in a single hint, always.

After Fix F: pipeline ran cleanly in 2 explore rounds, no oscillation guard. Answer was closer but still didn't reach "1" — explorer read `subagent.go` and `subagent_runtime.go` but missed the relevant logic in `agent.go` and `main.go`.

---

## Turn 9 — over-fitting audit

**User:**
> 检查一下这些修改里面有没有过拟合的部分（含提示词）

**Assistant:** Honest audit:

**Over-fitted (recommend rollback):**
1. **Fix F's prompt text** — "pick 2-3 source files most likely to contain the answer based on the matches you already have, then call read_file on each. Do NOT revert to grep/list_files/repo_map without read_file" — this is exactly the "if X-type question then Y-type action" pattern I had earlier criticized. Assumes the question is about source code, prescribes `read_file` as canonical. Should be rewritten as principle-only.
2. **Fix A2 (must use `read_file`/`exec_command`)** — opinionated. Wrong for purely structural questions ("how many .go files in /internal"). Encodes a preference for code-semantics questions as a universal floor.
3. **`isDepthTool` excluding grep** — defensible but a value judgment. `grep -A 20` returns substantial content.

**Slightly over-fitted (acceptable):**
- 32 KB threshold is a magic number tuned to Go repos but a reasonable default.

**Clean structural fixes (keep):**
- A (≥2 sources), B (transition gate), C (StageReport), D-2 (head-only `read_file`), E (RetryHint plumbing), F's mechanism (cumulative principle).

---

## Turn 10

**User:**
> 同意，都修一下吧

**Assistant:** Removed `isDepthTool`, `wentDeep`, and the depth requirement from `explorer.go`. `HasEnoughFacts` simplified to `len(sources) >= 2`. Retry hint rewritten as principle-stating:

```
"Previous attempt used fewer than 2 distinct tool types. The next
 attempt must use at least 2 different tools — choose them based
 on what the question actually needs."
```

Build and tests passed. Warned the user that removing A2 would likely regress the trigger query, since the explorer would no longer be forced to read files.

---

## Turn 11

**User:**
> 跑一遍看看

**Assistant:** Re-ran. As predicted, the answer regressed to "no straightforward method for counting" — but importantly, the failure mode was healthier than the intermediate "There are no agents" version. Honest weakness > dishonest confidence.

Pipeline still benefited from the structural fixes (StageReport channel, blob honesty, etc.) — they just couldn't compensate for the LLM picking the wrong files to read.

---

## Turn 12 — push to commit

**User:**
> 一把提交推送

**Assistant:** Committed Fixes A/B/C/D/E in a single commit (`2f99b8b`) with a descriptive message and pushed.

---

## Turn 13 — keep digging

**User:**
> 要不继续加点日志，定位一下llm没有读到根源文件的深层原因，我觉得系统上还有很大的问题

**Assistant:** Added full ReAct trace dump to `BaseAgent.Execute`: every iteration's initial messages, assistant content + tool call params, tool results. Wrote to stderr. Re-ran the query and captured to `/tmp/codrax-trace.log`.

The trace was damning:

```
iter=0: repo_map (299 bytes) — directory tree
iter=1: 3 parallel greps; grep "SubAgent" returned 12813 bytes
        containing literal main.go:78: agent.RegisterDefaultSubAgents(...)
iter=2: 1556 chars of assistant content — the LLM's synthesized findings —
        explicitly mentioning RegisterDefaultSubAgents by name:
          "Functions like NewSubAgentRegistry and RegisterDefaultSubAgents
           are used to set up and register subagents."
        Then no tool calls, loop ended.
```

**The LLM had the answer in its hands and chose to stop one tool call short.** It explicitly named the function that contains the answer, then wrote a summary saying "you may need to look at how the SubAgentRegistry is used throughout the codebase". It was prescribing next steps to the user instead of doing them.

This was not a model capability failure. The explorer's skill (`internal/skill/defaults.go:31-55`) defines it as a cataloger:

```
Goal: "Build a trusted factual foundation about the codebase."
Workflow: [
    "find entry points",
    "grep for key functions",
    "build module map",
    "analyze call chains",
    "identify relevant files",
    "document findings as RepoFacts",
]
```

Six steps of locating and zero of answering. Combined with the second mismatch — `final-answer-skill` has `ToolSuggestions: []string{}` and `finalizerEvaluator.ShouldStop` returns `true` unconditionally — analysis pipelines (`analyze → explore → finalize`) had no stage that actually investigates the question.

Two paths forward proposed:
- **Path A:** reframe `repo-explore-skill` as investigation (single point of change, defensible as contract fix)
- **Path B:** give the analysis-pipeline finalizer real teeth (preserves explorer's current contract, two changes, slightly more invasive)

Recommended Path A.

---

## Turn 14

**User:**
> 你能把你这次的分析结果和过程存储文档吗？

**Assistant:** Wrote `docs/investigation-explorer-knowledge-flow.md` capturing the full debugging timeline, the over-fitting audit, the deep architectural finding, and the two paths forward.

---

## Turn 15

**User:**
> 诊断日志和文档都提交我们再看看怎么选

**Assistant:** Committed the diagnostic logging in `BaseAgent.Execute` together with the investigation document (`2d382c2`) and pushed.

---

## Turn 16

**User:**
> 两条路都加上有什么弊端？

**Assistant:** Strict downsides of doing both Path A and Path B:

1. **Role overlap** — both stages investigate. The pipeline loses its "one stage one role" state-machine property.
2. **Implementation pipeline regression** — `finalize` becomes a multi-iteration tool-using stage even when the previous stages already produced the answer. Wasted iterations on every implementation task.
3. **Mutual masking** — Path B's tool-enabled finalize hides any failure of Path A's reframed explorer. Debugging signal disappears: "is explore really fixed?" becomes unobservable because finalize always saves the day.
4. **Dead workaround** — Path A fixes the root cause; Path B adds capability elsewhere as a workaround for the same root cause. After A succeeds, B is dead weight that's hard to delete because you can't prove it's never useful.
5. **Test surface explodes** — many tests rely on `finalizerEvaluator.ShouldStop = true`. Path A barely touches tests; Path B requires updating finalize-related mocks across orchestrator tests.

Concluded: it's a binary choice. Both together is strictly worse than either alone.

---

## Turn 17

**User:**
> 如果吧现在的explorer拆成两个角色呢？是不是就更保险

**Assistant:** Genuinely-considered third option. New stage `investigate` + new agent `investigator` + new skill, alongside the existing `explore`. analysis pipeline becomes `analyze → explore → investigate → finalize`; implementation pipeline stays unchanged.

**Safer in:**
- Additive change with zero implementation-pipeline risk.
- Two clean contracts (cataloger explorer for survey, investigator for answers) that can be described in one sentence each.
- Most consistent with the codebase's existing "1 stage = 1 agent role" pattern.
- Clear debugging signal — you know exactly which stage failed.

**Less safe in:**
- 200-400 lines of new code vs. Path A's 6.
- Doesn't escape the contract-definition problem — if the new investigator skill is also vague, you have the same bug under a new name.
- Pipeline complexity grows (more transitions, more state machine paths).
- Cataloger may turn out to be vestigial — `plan` already does its own deep reads.

Final verdict: split is strictly better than `A + B` together but not strictly better than Path A alone. Recommended trying Path A first because it's six lines and reversible, with split as the fallback if Path A regresses implementation pipelines.

---

## Turn 18

**User:**
> 好，用A试一次

**Assistant:** Implemented Path A. Edited `internal/skill/defaults.go:31-55`:

```go
Goal: "Investigate the user's question and answer it directly using evidence from the code."
Workflow: [
    "form a hypothesis about where the answer lives",
    "read the code or run the commands needed to verify it",
    "if you surface a name that looks load-bearing (a function, type, symbol, config key), open it before drawing conclusions — a name is a hypothesis to verify, not an answer",
    "iterate until you have a specific answer, not a description of where someone else could find it",
    "record the answer alongside the evidence (file:line) that supports it",
]
OutputFormat: "Direct answer to the user's question, plus the file:line citations that establish it. If part of the question is unanswered, say which part and why — do not substitute 'further analysis required' for an answer."
```

Re-ran. Trace showed iter=5 had 1339 chars of content saying:
> "I'll check for registrations that employ subagent capabilities next. **Let's explore the `RegisterDefaultSubAgents` method for details.**"

Then the loop broke. The skill change had moved the LLM's intent in the right direction, but `BaseAgent.Execute`'s hardcoded `(len(toolCalls) == 0 && content != "")` clause force-stopped it.

**Layer 2 root cause:** the ReAct loop's "no tool calls = done" rule overrides any skill contract whenever the LLM "thinks aloud" mid-investigation.

Implemented **Fix G: `ContinuingEvaluator` interface**.

```go
type ContinuingEvaluator interface {
    ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, history []types.ToolResult) (prompt string, shouldContinue bool)
}
```

Split the stop logic in `BaseAgent.Execute`: hard stops still come from `eval.ShouldStop` (finalizer's `return true` keeps current behavior), soft stops now consult the optional `ContinuingEvaluator`. Explorer implements with a one-shot continuation prompt:

> "Your previous turn produced a summary without calling any tool. If you genuinely have a defensible answer, restate it in one sentence and stop. Otherwise, take the next concrete investigative step now — do not describe what you would do next, do it."

Principle-stating, no tool name, no target. Re-ran — explorer started reading files. But it read `agent.go` offset=0 limit=40 (lines 1-40), then offset=40 limit=40 (lines 40-80), and concluded "I have enough" without realizing `agent.go` is 330 lines and the auto-inject is at line 219. Confidently wrong: "all 8 agents can create subagents".

**Layer 3 root cause:** `read_file` returns slices with no metadata. The LLM had no way to know it was looking at the head of a 330-line file.

Implemented **Fix H: line range banner.**

```go
banner := fmt.Sprintf("[%s: showing lines %d-%d of %d total]\n", p.Path, sliceStart+1, sliceEnd, totalLines)
content = banner + content
```

Always present, regardless of slice or full read, blob or inline. Plain metadata, no advice. Updated `builtin_test.go` for the new format.

Re-ran. **Correct answer.** Trace:
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

Correct, precise, with file:line citations. The LLM used grep's line numbers to do a targeted `read_file` at offset=63, instead of starting at offset 0 and missing the relevant section. Fix H made line numbers in grep output meaningful as targets.

---

## Turn 19

**User:**
> 同意你的建议

**Assistant:** Updated the investigation doc with §7 "Resolution" describing the three layers, the three fixes, and the over-fitting audit. Committed Path A + Fix G + Fix H + doc update as `c2bf044`. Removed temporary diagnostic logging in a separate commit `f4a9ae1`. Pushed both.

---

## Turn 20

**User:**
> 把我们的对话也存一下提交

**Assistant:** Wrote this transcript document.

---

## Reflections after the fact

A few patterns from this session that are worth keeping in mind for future debugging:

1. **Each "I think the structural fix is X" was right *and* incomplete.** Layer 1 (skill contract) was the deepest layer I could see at the time, but Layer 2 (loop hardcoded behavior) and Layer 3 (tool metadata) only became visible after Layer 1 was fixed and observed in action. Onion peeling. Don't assume the first deep cause you find is the deepest.

2. **Diagnostic logging earned its keep.** I delayed adding it because I could read code and reason. That worked for Layers C and D, but Layers G (loop behavior) and H (slice metadata) only became visible from a full ReAct trace. Specifically: seeing the LLM say "I'll check X next" *and then not check X* was the moment Layer 2 became unmistakable. Reading code alone wouldn't have surfaced it.

3. **Over-fitting checks should run during the work, not just at the end.** Fix F's prompt text was textbook "if X-type question then Y-type action" — and I wrote it. Catching it on a separate audit pass worked, but I would have been better off pausing on the first version of F to ask "is this a principle or an instruction?".

4. **Honest weakness > dishonest confidence.** The intermediate "There are no agents" answer was *worse* than the original "I couldn't find it" answer, even though it was more decisive. A model that confidently says wrong things is harder to recover from than a model that admits uncertainty. The de-overfitting audit's regression was worth taking, because it moved the failure mode in the right direction.

5. **The user's instinct that "this is structural, not LLM" was correct in every round.** Each time I was tempted to say "this is now an LLM capability question, we're done", there was actually one more structural layer underneath. The discipline of "look one more level" was theirs, not mine. Worth remembering.
