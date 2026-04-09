# Investigation: grep Case Sensitivity and read_file Small-File Slicing

> **Date:** 2026-04-09 (follow-up session)
> **Trigger query (same as prior session):** "这个项目有多少个agent可以创建subagent？"
> **Outcome:** Two Layer 4 (tool capability) defects fixed in commit `91de97f`. Five lower-priority issues identified and left open; see §6.

This document is a follow-up to `investigation-explorer-knowledge-flow.md`. The prior session shipped Path A (skill reframe), Fix G (continuation hook), and Fix H (read_file line banner) and recorded a "successful" trace where the trigger query returned a precise answer with file:line citations. The companion transcript was committed as the gold-standard worked example.

This session re-ran the same trigger query and found that the prior "success" was sampling jitter on top of two structurally fragile tools. The upper-layer fixes (Path A / G / H) were necessary but not sufficient: a Layer 4 defect at the bottom of the stack could deflect the LLM into a search path from which no amount of agent-loop or skill rescue could recover. Once the tool defects were fixed, the upper-layer machinery suddenly worked deterministically across three consecutive runs.

## 1. The regression

After commit `8e321ff`, re-running the trigger query produced:

> "The project includes several agents involved in creating subagents using the `SubAgentRegistry` and `SubAgentRuntime` systems. ... `RegisterDefaultSubAgents()`, `SubAgentValidator`, and `SubAgentReducer` are key functions ..."

Compared to the documented expected answer in `investigation-explorer-knowledge-flow.md` §"Final outcome":

> "There is currently one implemented subagent named `SubExplorer`, which is registered via `RegisterDefaultSubAgents` at lines 63-66 in `subagent.go` ..."

| Field | Doc expected | Re-run actual |
|---|---|---|
| Count | "**one** implemented subagent" | "several agents" (mushy) |
| Name | `SubExplorer` | not named |
| Citation | `subagent.go:63-66` | filename only, no line |
| Tone | precise, with file:line | catalog of components |

`SubAgentRegistry` and `SubAgentRuntime` are *host mechanisms*, not subagents. The finalizer was listing the plumbing instead of answering the question. This is a real regression in correctness, not just style.

## 2. First-pass observation: facts are flowing, answers are wrong

The startup log read `registered 11 tools, 1 sub-agents` — at runtime there is exactly one SubAgent (`SubExplorer`). The truth is one. The pipeline emitted "several". Where in the chain does the precision get lost?

A diagnostic dump was added to the orchestrator's `applyStageOutput` to print every stage's `StageReport` (the LLM's own narrative passed forward to downstream stages) and every new fact:

```go
log.Printf("[diag] stage_report stage=%s agent=%s len=%d\n----- stage_report begin -----\n%s\n----- stage_report end -----",
    o.busCtx.PipelineStage, o.busCtx.ActiveAgent, len(output.StageReport), output.StageReport)
```

The dump showed that the **explorer** was already wrong:

> "No specific agents in this context create subagents directly, but these structures facilitate subagent creation and execution."

The finalizer was not introducing the error — it was faithfully re-formatting an already-mushy explorer report into a more verbose, more template-laden version of the same mush. This narrowed the investigation from "is the finalizer hallucinating?" to "why is the explorer's evidence not landing on the answer?"

## 3. Per-iteration ReAct trace

The orchestrator-level dump only shows the *final* per-stage narrative. To see what the LLM was actually doing turn by turn, a CODRAX_DIAG_TRACE-gated dump was wired into `BaseAgent.Execute` (this is the same shape as the diagnostic logging removed in commit `f4a9ae1`, re-introduced behind an env var so it does not pollute production stderr):

```go
if diagTrace {
    log.Printf("[diag/%s] iter=%d msgs=%d resp.content_len=%d resp.tool_calls=%d", ...)
    if resp.Content != "" {
        log.Printf("[diag/%s] iter=%d assistant_content=%q", b.name, i, diagTrunc(resp.Content, 600))
    }
    for j, tc := range resp.ToolCalls {
        log.Printf("[diag/%s] iter=%d tool_call[%d] name=%s params=%s", ...)
    }
}
```

Plus dumps for the soft-stop path (whether continuation was injected, whether it was rejected) and tool result heads. Re-running the trigger query with `CODRAX_DIAG_TRACE=1`, the explorer trace was:

```
iter=0 tool_call repo_map {"max_depth":2,"path":"./"}
iter=1 tool_call list_files {"path":"./internal/agent","recursive":true}
iter=2 tool_call grep {"include":"*.go","path":"./internal/agent","pattern":"creates subagent"}
       → no matches found
iter=3 tool_call grep {"include":"*.go","path":"./internal/agent","pattern":"CreateSubAgent"}
       → no matches found
iter=4 tool_call grep {"include":"*.go","path":"./internal/agent","pattern":"subagent"}
       → 4 hits, all noise:
         subagent.go:46:    fmt.Errorf("subagent not found: %s", name)
         subagent_runtime.go:136: log.Printf("[subagent-runtime] start[%d]...
         subagent_runtime.go:138: log.Printf("[subagent-runtime] done[%d]...
         subagent_runtime.go:156: fmt.Errorf("subagent errors: %v", msgs)
iter=5 tool_call grep {"include":"*.go","path":"./internal/agent","pattern":"struct"}
       → SubAgentRegistry / SubAgentValidator / SubAgentRuntime / SubAgentReducer (and others)
iter=6 assistant_content "To determine how many agents can create subagents,
       I'll look into the SubAgentRegistry and SubAgentRuntime structures further.
       These seem relevant to subagent management. Let's read these structures..."
       soft_stop_path entered (continuationsUsed=0)
       continuation INJECTED ✓
iter=7 tool_call read_file {"limit":20,"offset":0,"path":"./internal/agent/subagent.go"}
       tool_call read_file {"limit":20,"offset":0,"path":"./internal/agent/subagent_runtime.go"}
       → "[./internal/agent/subagent.go: showing lines 1-20 of 66 total]
          package agent
          import (...)
          type SubAgent interface { ... Run(...) }"
iter=8 assistant_content "The SubAgent interface and SubAgentRuntime structures
       are key components ... Let me further investigate the SubAgentRegistry to
       see which agents can register or create subagents."
       soft_stop_path entered (continuationsUsed=1)
       soft_stop ACCEPTED (no continuation) ← loop quits
```

This trace is decisive. Two Layer 4 defects compound:

### Defect 1 — `grep` is case-sensitive with no `-i` exposure

iter4 grep'd `"subagent"` (lowercase) and got 4 noise hits. The CamelCase identifiers `SubAgent`, `SubAgentRegistry`, `RegisterDefaultSubAgents` — i.e. **all the load-bearing names** — were silently filtered out because they contain uppercase characters.

The "successful" run documented in `investigation-explorer-knowledge-flow.md` had grep'd `"SubAgent"` (CamelCase). That run got the 6.4 KB hit-list mentioned in the doc; this run got 4 noise lines. **The difference is one keystroke of casing in the LLM's chosen pattern, and there is no way for the LLM to recover if the wrong casing is sampled.**

`internal/tool/builtin.go:GrepTool` ran `grep -rn <pattern>` with no `-i` flag and exposed no `ignore_case` parameter. The schema and description said nothing about case sensitivity. From the LLM's perspective there was no contract to follow — and the result of guessing wrong was an empty haystack labeled "no matches found", which actively misleads the LLM into searching elsewhere.

### Defect 2 — `read_file` honors `offset/limit` even on tiny files

After Defect 1 deflected the investigation, Fix G's continuation hook fired and pushed the LLM to do a `read_file`. But the LLM passed `offset=0, limit=20` on `subagent.go`, a **66-line file**. The answer (`RegisterDefaultSubAgents` at line 63) was 46 lines past the cutoff.

Fix H's banner did fire correctly: `[./internal/agent/subagent.go: showing lines 1-20 of 66 total]`. The LLM read it. The LLM did *not* paginate. Instead it produced another thinking-aloud summary, the second soft-stop tripped, `continuationsUsed` was already 1, the second continuation was rejected, and the loop quit.

The `offset/limit` parameters on `read_file` exist for paging files too large to inline (the threshold `MaxInlineBytes = 32 KB` is the existing IO subsystem boundary). Honoring them on a sub-1KB file is strictly worse than ignoring them: the LLM passes `limit=20` because "20" is a popular default in its training distribution, not because 20 is meaningful for the specific file. The cost of returning the whole 66-line file is zero; the cost of slicing it is occasionally hiding the answer just past the cutoff.

### Why these are the *real* root causes (and Fix G is not)

It is tempting to say the bug is in the continuation budget (`continuationsUsed >= 1` is too tight). But the continuation hook **fired** in every failing run; it just couldn't rescue an LLM that was reading from the wrong tool output. Raising the budget would only mask the underlying problem: the LLM would still be searching the wrong haystack and reading the wrong lines, just for more turns.

Path A (skill reframe to investigation) actually worked. Fix G actually fired. Fix H actually printed the banner. All three upper-layer fixes were doing their job. The defects were **below** them, in tools that silently misled the LLM about what was true. Fix the tools and the upper-layer machinery worked deterministically across three consecutive runs.

Layered root causes only become visible after the layer above is no longer hiding them. The prior session called this out as a session-level lesson. This session is a literal repeat of it: Path A/G/H made the previously-invisible Layer 4 defects observable.

## 4. The fixes

### Fix I — Smart-case in `grep` (`internal/tool/builtin.go`)

Match ripgrep's `-S` rule: if the pattern contains no uppercase character, add `-i` automatically. Plus an explicit `ignore_case` parameter so the LLM can pin exact-case when it really needs to. Description updated to flag the contract.

```go
caseInsensitive := false
if p.IgnoreCase != nil {
    caseInsensitive = *p.IgnoreCase
} else {
    caseInsensitive = !strings.ContainsAny(p.Pattern, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
}
args := []string{"-rn"}
if caseInsensitive {
    args = append(args, "-i")
}
```

This matches the convention of every modern code search tool (ripgrep, VS Code, JetBrains): a lowercase query is conceptually case-blind, an uppercase query is exact. It matches LLM intuition because LLM training data is full of this convention. No special-casing of "subagent" or this codebase — the implementation is one `ContainsAny` line that generalizes across any project, any language, any naming style.

### Fix J — Small-file passthrough in `read_file` (`internal/tool/builtin.go`)

If the file fits in `MaxInlineBytes`, ignore `offset/limit` and return the whole file with a banner announcing the override. Reuses the existing 32 KB threshold from the blob/inline IO subsystem; introduces no new constant and no per-file special case.

```go
if (p.Offset > 0 || p.Limit > 0) && len(data) > MaxInlineBytes {
    // existing slicing logic
} else if p.Offset > 0 || p.Limit > 0 {
    overrode = true
}
// banner reflects which path was taken
```

The semantics: `offset/limit` are paging knobs for files too large to inline. On a small file there is nothing to page. The override is announced in the banner so any caller that genuinely needed a partial read can see it happened.

## 5. Verification — three consecutive runs

The trigger query was re-run three times after the fixes shipped. All three runs converged on the correct substance, vs. the baseline "several agents":

| Run | Path through the code | Answer correctness |
|---|---|---|
| 1 | LLM passed `ignore_case:true` explicitly (read the new tool description and adopted it); `read_file subagent.go limit=50` returned the whole file via passthrough | Correct substance: names `RegisterDefaultSubAgents`, cites `subagent.go:63` |
| 2 | Continuation injected after lowercase `subagent` grep; `read_file offset=63 limit=40` targeted at the right line | Correct alternate interpretation: identifies the `propose_sub_agents` injection mechanism in `agent.go:297-298` |
| 3 | LLM grep'd CamelCase `SubAgent`; `read_file offset=0 limit=20` would have failed under the old code, but small-file passthrough returned the whole 66-line file | **Best case: explicit count "1 (SubExplorer)" with both name and citation** |

Notable: Run 1 demonstrates that the LLM actually reads tool descriptions and adopts new parameters when the description tells it to. Run 3 demonstrates that the small-file passthrough rescues the LLM from a wrong `limit` parameter without the LLM needing to know it was wrong. Run 2 demonstrates the alternate-but-correct answer interpretation, which matters because the trigger query is genuinely ambiguous in Chinese ("能创建 subagent 的 agent" can mean either "implementations of SubAgent" or "main agents that can dispatch subagents"). Both interpretations resolve to "1" for this codebase.

The remaining variation across runs is in **finalizer formatting verbosity**, not in explorer correctness. See §6.A.

## 6. What was deliberately left unfixed

The fixes shipped are the smallest set that resolves the trigger query deterministically. Several adjacent issues were identified during the investigation but deliberately not fixed in this session, either because they are downstream of the trigger (not load-bearing for this query), because they are skill-prompt issues that need a separate review pass, or because they are documentation debt rather than code defects. They are recorded here so the next session does not have to re-discover them.

### A. Finalizer dilutes precise answers into templated prose (HIGH impact, MEDIUM cost)

`internal/skill/defaults.go:final-answer-skill` requires an SDLC-style `Summary / Changes / Conclusion / Action Steps / Instructions` structure. This is correct for implementation tasks (where the user wants a changelog) and wrong for analysis tasks (where the user asked a question and wants an answer). On Run 3 the explorer's stage_report contained a clean `Counted agents capable of creating subagents: 1 (SubExplorer)`, but the finalizer wrapped it into a five-section template with invented "Action Steps" and "Tasks Completion" sections that the explorer never produced.

**Fix direction:** route by task policy. If the current pipeline is `analyze → explore → finalize` (analysis policy), use a separate `analysis-answer-skill` whose OutputFormat is "one-sentence direct answer with file:line citations, no template". Keep the existing skill for `implementation` / `high_risk_implementation` policies. This is a Layer 3 (skill) + finalizer-routing change, not a tool change. It is the highest-leverage remaining issue because the explorer already produces correct substance in most runs — the loss is at the formatting layer.

### B. Continuation budget = 1 is too tight for two-step recovery (MEDIUM impact, LOW cost)

`explorer.go:ContinuationPrompt` returns `false` once `continuationCount >= 1`. If the LLM's recovery action is itself slightly wrong (e.g. a `read_file` at the wrong offset), the second soft-stop is hard-stopped with no second push. Fixes I and J make the recovery path no longer load-bearing for *this* query, but the budget will trip again as soon as a different Layer 4 tool produces an occasional wrong-but-not-empty result.

**Fix direction:** raise to 2 with a different prompt on the second push: "if your last action did not answer the question, choose a *different* approach." Combined optionally with a fast-path: "if you genuinely don't know, say 'I don't know' in one sentence and stop — that is better than a confident wrong answer." The second part operationalizes the "honest weakness > dishonest confidence" lesson from the prior session. Not fixed in this round because the trigger query is now stable; revisit on the next failure case.

### C. Explorer's `HasEnoughFacts` floor is `≥2 distinct tool sources` — too strict for single-tool questions (LOW impact, LOW cost)

`explorer.go:82` requires two distinct tool names before declaring exploration complete. This is a sensible anti-trivial-floor for most cases, but trips false-positive on questions that legitimately need one tool: "how many .go files in this project?" → one `exec_command find . -name '*.go' | wc -l` is the correct answer, but the floor will trigger an unnecessary self-loop.

**Fix direction:** swap "distinct tool count" for "an explicit signal from the LLM that the question is answered." This is the same root cause as §6.A — the explorer doesn't distinguish between "I have enough information to answer" and "I have explored the repo enough." Defer alongside the skill-routing fix.

### D. Cataloger drift in explorer output format (LOW-MEDIUM impact, MEDIUM cost)

Path A reframed the explorer's skill goal and workflow as investigation, but did not update the OutputFormat field. The LLM still occasionally outputs `## Key Components / ## Files Involved / ## Usage Instructions` cataloger sections (Run 1's explorer report did exactly this). The semantic field (goal) and the format field (OutputFormat) are pulling against each other.

**Fix direction:** rewrite the explorer's OutputFormat as an explicit few-shot example: `Output the answer in this form: '<one-sentence answer>. Evidence: <file:line>, <file:line>.'` Few-shot format examples bind LLM output much more tightly than abstract format descriptions. Same skill-pass as §6.A.

### E. `propose_sub_agents` injection by name match is implicit and undocumented (LOW impact, doc debt)

`internal/agent/agent.go:297-298` decides which main agents can call `propose_sub_agents` by checking whether `b.deps.SubAgents.Get(string(b.name))` succeeds — i.e. whether a SubAgent is registered under the same name as the parent agent. This is the actual mechanism behind "which agents can create subagents" for interpretation B of the trigger query. It works correctly. But it is **not documented** anywhere: no godoc on `BaseAgent.buildToolSchemas`, no overview in `architecture.md`, no enum or registry that lists the relationship. Run 2 of the verification correctly inferred this mechanism, which is impressive but accidental — a reader of the codebase will find this coupling surprising.

**Fix direction:** add a doc comment at `agent.go:294-310` explaining the name-based scoping convention. Optionally promote the check to a `BaseAgent.CanProposeSubAgents() bool` method so the relationship is named in the type system. Not a behavior change, just legibility.

## 7. Reflections

**Observability earned its keep at exactly Layer 4.** The orchestrator-level stage_report dump (§2) localized the failure to "explorer is already wrong, not the finalizer." The per-iteration ReAct trace (§3) localized it further to "explorer's grep on iter4 returned noise, and read_file on iter7 missed the answer past line 20." Neither dump cost more than ~30 lines of code, and both were stripped immediately after the diagnosis the same way commit `f4a9ae1` stripped its predecessor. The reusable lesson is that diagnostic logging should be cheap, scoped behind an env var, and removed when done — not preserved as production telemetry.

**The prior session's "successful" trace was pure jitter.** The doc in `investigation-explorer-knowledge-flow.md` records a precise answer with file:line citation. Re-reading that trace alongside this session's finding, the previous run succeeded *only* because the LLM happened to capitalize `SubAgent` in its grep AND happened to pass the correct `offset` to `read_file` — two independent dice rolls on top of two structurally fragile tools. A worked example that depends on sampling jitter is not a worked example, it is a screenshot of a coincidence. The previous session's confidence in Path A/G/H was warranted in direction but not in magnitude; the upper-layer fixes were necessary but the bottom layer was still leaking.

**Layer 4 defects have outsized impact because they corrupt the LLM's epistemic input.** When a tool returns `"no matches found"` for a pattern that should have matched, the LLM does not know it was misled. It reasons forward from a false negative. Every downstream agent loop, every continuation hook, every skill prompt is then trying to recover from an evidentiary base that the LLM trusts. Tools must not lie by omission. Smart-case grep and small-file passthrough are both instances of the same principle: the tool's surface area should match the user's mental model, even when the user's mental model is implicit.

**The over-fitting audit pattern is now load-bearing.** The prior session rolled back Fix A2/F because they were "explorer must use read_file at least once" — a hard-coded preference disguised as a capability. This session's two fixes were tested against the same audit (see §1 of the assistant-side conversation log): does this fix reference any feature of the trigger query? does it special-case any path or symbol? does it have a reverse test? Both fixes passed all three checks, which is why they survived to commit. The audit is not optional; it is the only thing standing between "fixes the failing case" and "fixes the failing case at the cost of the next ten cases."
