# Follow-ups from the REPL-equivalence audit (2026-04-12)

Parked from the session that shipped `5908354` → `447667a`. None of these are regressions; they are scoped next-steps that surfaced while fixing the 10 main issues and were explicitly deferred. See `docs/investigation-answer-extraction-and-early-stop.md` for the full session narrative and `memory/project_repl_equivalence_audit.md` (local memory store) for the F1–F4 diagnostic trail.

## Priority legend

- 🔴 **High** — has reproducer or log evidence, same fix pattern as a landed commit
- 🟡 **Medium** — has evidence, impact smaller than the high-priority items
- 🟢 **Low** — architectural improvement, not a bug
- 📋 **Meta** — process / methodology, not code

---

## 🔴 High priority

### 1. `sub_explorer` singleton state leak (same pattern as F2)

**Where:** `internal/agent/sub_explorer.go:82-91` (`subExplorerEvaluator` struct) + `BuildInitialPrompt` at line 93.

**Symptom:** The sub-explorer evaluator has the same process-lifetime singleton shape as the main explorer. Fields `investigationNotes`, `structuredEvidence`, `flowFindings`, `idleStreak`, `lastToolCount`, `objective`, `scope` survive across `Run()` calls. `BuildInitialPrompt` only resets `structuredEvidence` and `flowFindings` — the rest leak.

**Why it bites:** Sub-explorer runs only when the main explorer calls `propose_sub_agents`. Across REPL turns, a second invocation would inherit the first turn's scoped investigation notes and treat the new objective as a continuation.

**Not in critical path:** the main REPL flow works without `propose_sub_agents` being triggered. But the bug exists and should be fixed for completeness.

**Fix:** Port F2's pattern — compare `ctx.Objective` (the sub-task objective) to cached `e.objective`; when different, reset every cross-Run field. Analogous reset block to the one in `explorer.go` lines ~48-75.

**Tests:** One `TestSubExplorerBuildInitialPrompt_CrossRunResetOnObjectiveChange` asserting state wipe on objective change; one `_SameObjectiveKeepsNotes` as the intra-Run companion.

**Estimate:** 30–60 min including test.

---

### 2. Audit the other agent singletons

**Where:** All of `internal/agent/analyzer.go`, `planner.go`, `implementer.go`, `design_reviewer.go`, `code_reviewer.go`, `verifier.go`, `finalizer.go`.

**Symptom:** `cmd/root.go:initApp` calls `agent.RegisterDefaults` once at process start, which calls each `NewXxxAgent` constructor once. Every agent evaluator is therefore a process-lifetime singleton, same as the explorer.

**Why the impact is lower than F2:** the main explorer has a `retry` branch (`if len(e.investigationNotes) > 0`) that specifically consumes accumulated state; the other agents don't have retry-branch logic gated on accumulated fields, so the leak is silent rather than semantically wrong. But this is based on a quick grep — not a full audit.

**What to check for each agent:**
- Struct fields on the evaluator (not just method locals)
- `BuildInitialPrompt` reset coverage (`= nil` / `= 0` assignments at the top)
- Anything read via `e.X` in `ShouldStop` / `ContinuationPrompt` / `ParseOutput` / `DetermineMissingPiece` that isn't written in `BuildInitialPrompt`

**Fix:** If any fields leak, apply the F2 pattern per-agent.

**Estimate:** 1–2 hours audit + any per-agent fixes.

---

## 🟡 Medium priority

### 3. `extractRankingEntities` accepts lowercase generic words

**Where:** Whatever file defines `extractRankingEntities` (helper, called from `explorer.go:205` and elsewhere).

**Symptom:** Even after F1 (strip REPL prefix), the real-scenario REPL test's ERM entity list still contained `many, invoke, agents` — generic English words pulled from the clean current request by the regex. Not as catastrophic as F1's memory-path entities (they don't break `ermAllSatisfied`), but they bloat the entity set and skew the `hasEnough` relevance-weighted coverage calculation at `explorer.go:949-984`.

**Fix direction:** One of
- Add a stop-word filter (over-fit risk: hardcoded list), or
- Only accept tokens with at least one uppercase letter or a `_` (catches Go/Java Pascal, Python snake, JS camel — rejects pure lowercase prose words)
- Or require the token to match a symbol in `graph.SymbolDefs` (strongest, but couples entity extraction to graph availability)

**Over-fit risk:** very high. Any stop-word list runs the risk of killing legitimate lowercase symbol names (`explorer`, `subagent`, etc.). The `one uppercase OR underscore` rule is better but still excludes lowercase-only symbols that are real answers. Must be tested against the full audit grid (Parts 3/6 tests) AND the 5-run REPL stability test before shipping.

**Estimate:** 1–2 hours, mostly on the over-fit audit + eval run. Don't do this at the beginning of a session.

---

### 4. Compacted memory may describe historical errors verbally

**Where:** `internal/memory/store.go:compactOldest` + the LLM summarizer it calls.

**Symptom:** F4's write-side sanitize (`repl.recordTurn` placeholder) and read-side sanitize (`sanitizeErrorResponse`) catch error-laden turns at the structured level: leading `error:` prefix, specific failure phrases. But `compactOldest` runs an **LLM summarizer** over the raw turn before these fixes reach the disk. The summarizer produces natural-language descriptions like *"the previous session hit a 5-visit oscillation guard while trying to list sub-agents"* — which contain none of the structural phrases `sanitizeErrorResponse` looks for.

**Impact:** `### Relevant compacted memory` section of `BuildContext` still surfaces these LLM descriptions on matching queries. Lower impact than the Recent conversation section because compacted entries are short summaries rather than verbatim dumps, but still leaks historical failure signal into the current analyzer prompt.

**Fix direction:**
- (a) Call `sanitizeErrorResponse` on `turn.Response` *before* passing it to the summarizer, so the summarizer sees the clean placeholder. Simple but drops whatever topical signal was in the failed turn's request.
- (b) Add a `WasError bool` field to `IndexEntry`, set it when the summarizer's input was error-laden, and have `BuildContext` skip entries whose `WasError` is true. Preserves the request-side topic for clustering purposes but keeps the summary out of prior-conversation context.

**Option (b) is cleaner** but requires a schema change in `MEMORY.md` (add a field, handle backward-compat on existing entries).

**Estimate:** 1–2 hours.

---

## 🟢 Low priority (architectural)

### 5. Tighter analyzer-entity vs. regex-entity merge policy

**Where:** `internal/agent/explorer.go:197-214`.

**Symptom:** The current policy is *union* — analyzer's structured `ctx.CurrentTaskEntities` AND regex-extracted `regexEntities` both feed `ermEntities`. Even in single-shot mode, the union on `"how many agents can invoke subagent"` produces ~5 entities including lowercase words. A *prefer-analyzer-unless-empty* policy would give 1 entity (`subagent`) which is cleaner but potentially loses CamelCase symbols the analyzer missed.

**Why union was chosen originally** (per the 40+ lines of comments at the current line 180-196): analyzer alone sometimes produces only 1 entity when the user's phrasing has a single CamelCase-looking token, and ERM's `call_chain` requirement demands 2+ entities to reach "satisfied".

**Proposed change:** Prefer analyzer; fall back to regex only when analyzer produced < 2 entities. Addresses the common case without regressing the ERM `call_chain` requirement.

**Risk:** whatever regression the df1 run caught in commit `c04298f` could come back. Must re-run the full df1 5x eval before landing.

**Estimate:** 1 hour change + 30 min eval re-run.

---

### 6. Compress S1 diagnostic debug logs

**Where:** `internal/agent/explorer.go` — the `[explorer] S1 check iter=X` + 4× `S1 erm: ...` lines per soft-stop.

**Symptom:** At `--log-level debug`, every soft-stop that passes the tool-count / phase / ermRequirements-non-empty gates emits 5 log lines. In a 12-iter REPL run that's ~20 lines per turn of diagnostic noise.

**Fix:** Collapse to one line like `[explorer] S1 check iter=X notes=N+1 erm=[enum:sat,call:sat,reg:sat,reg:sat] → decision`. Keep the information, drop the line count.

**Estimate:** 15 min.

---

## 📋 Meta (non-code)

### 7. Save a `feedback_task_id_triangulation.md` rule

**What went wrong this session:** When the log contained `Codrax: error: task 3e5d94c4 stuck at stage explore after 5 visits`, I dismissed it as a "source-code reference" or "rendering artifact" without proof. The user called it out and made me re-check. The correct response would have been:

1. Grep the current session's **strict-format** orchestrator log (`[orchestrator] task=` / `starting pipeline: trace=`) for task/trace IDs
2. Compare the UUID in the error text to this set
3. If it matches → current-session error; investigate
4. If it doesn't match → historical memory replay; trace the source and report *that* as a separate issue

I skipped steps 1-2 and guessed.

**Write:** A new feedback memory file describing this triangulation recipe. Something like:

> When any log contains a `task-<uuid>` or `trace-<n>` reference, first extract the CURRENT session's task/trace ID set from the strict `[orchestrator]` log prefix. Compare the reference against that set before saying anything about its meaning. Mismatched IDs mean historical memory replay, not current-session state — trace the replay source.

**Estimate:** 10 min.

---

## Suggested next-session order

1. **#1 `sub_explorer` reset** — same pattern as F2, low risk, quick confidence build
2. **#4 compacted memory** — same scope as F4, structural metadata is cleaner than heuristics
3. **#2 other agent audit** — if new bugs surface, fix them with the #1 pattern
4. **#7 methodology memory** — 10 min, doesn't touch code
5. **#3 and #5 entity cleanup** — only after the above. Higher over-fit risk, requires eval runs. Consider reserving a dedicated session for this pair.
6. **#6 log compression** — whenever convenient, independent of the above.

**Do not** start the next session with #3 or #5 — they touch the entity-set semantics that every downstream fix (S1/S2, Phase 2/3, stable sort ranking) depends on, and regressions there would cascade through the whole audit grid.
