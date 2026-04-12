# Answer Extraction, Early-Stop, and Sort Stability — A Multi-Audit Cascade

**Session**: 2026-04-12 · **Commits**: `5908354` → `3cc5613` (10 commits) · **Author**: @hanchaoqun + Claude Opus 4.6

---

## Table of contents

1. [Why this document exists](#why-this-document-exists)
2. [Commit map](#commit-map)
3. [Part 1 — REPL `context_length_exceeded` (two signatures)](#part-1--repl-context_length_exceeded-two-signatures)
4. [Part 2 — Interactive status area 刷屏](#part-2--interactive-status-area-刷屏)
5. [Part 3 — Wrong answer: `extractAnswerSymbols` audit](#part-3--wrong-answer-extractanswersymbols-audit)
6. [Part 4 — Always 20 iterations: explorer early-stop audit](#part-4--always-20-iterations-explorer-early-stop-audit)
7. [Part 5 — Truth-slice accumulation on self-loop](#part-5--truth-slice-accumulation-on-self-loop)
8. [Part 6 — Determinism: stable multi-key sort](#part-6--determinism-stable-multi-key-sort)
9. [Cross-cutting lessons](#cross-cutting-lessons)

---

## Why this document exists

The session started from a single user-visible complaint — "some questions take 2 minutes, answer is wrong" — and ended up fixing **ten** distinct defects across memory, rendering, extraction, loop control, orchestration, and sort stability. Every fix was discovered by running the real binary, saving the debug log, and tracing data flow end-to-end before touching code.

This document is the map of that journey. Each section follows the same structure:

1. **Symptom** — what the user saw
2. **Failure mechanism** — why it breaks
3. **Evidence** — log lines, file:line anchors, test output
4. **Root cause** — the actual defect
5. **Fix** — code snippet
6. **Test lock** — what prevents regression

The intent is that a future reader facing a similar symptom class can jump to the right section, match signatures, and either reuse the diagnostic recipe or extend the fix.

---

## Commit map

| commit | area | headline |
|---|---|---|
| `5908354` | memory | cap turn-file size and `BuildContext` inline budget |
| `245f96c` | agent | cap ReAct tool-history bytes and fix grep coverage parsing |
| `2028b01` | render | clamp spinner area lines to terminal width to stop 刷屏 |
| `bf09232` | test | deterministic reproducer for reverse-reference enumeration bug |
| `8686dec` | test | extraction audit grid — 8 characterization cells |
| `b4654b3` | erm | Phase 2 — direction-aware answer-symbol extraction |
| `c5e4f6e` | erm | Phase 3 — decorator/maps routing + multi-language identifier |
| `829c266` | explorer | early-stop fix (S1+S2) — semantic stop + cross-ref filter |
| `a0f2f4f` | orchestrator | dedup truth slices in `applyStageOutput` |
| `3cc5613` | erm | stable multi-key sort for `identifyAnswerChains` |

---

## Part 1 — REPL `context_length_exceeded` (two signatures)

### Symptom

In interactive (REPL) mode the pipeline would crash with `{"error":{"code":"context_length_exceeded"}}` from the OpenAI adapter. Two distinct cases:

- **Pattern 1**: fires on the *analyze* stage (the very first agent), only in REPL mode, only on requests whose keywords match something already in `memory/<slug>/MEMORY.md`.
- **Pattern 2**: fires on the *explore* stage (or any long ReAct loop), in both single-shot and REPL modes, on the 10th+ iteration after many `read_file` / `grep` calls.

Knowing *which stage* fires the error disambiguates the two pattern families in seconds.

### Pattern 1: memory's full-ref inlines blow the window

#### Failure mechanism

`internal/memory/store.go:BuildContext` was loading every keyword-matching memory entry's full turn file and inlining it into the prompt — with no top-K cap, no per-entry cap, no total-budget cap. Meanwhile `Append` (write-side) had no size limit on `turn.Response`. A single legacy turn file from an earlier codrax version was multi-megabyte (ANSI-escaped glamour-rendered content) and got inlined verbatim on every subsequent REPL run.

#### Evidence

```
request → memory keyword match → BuildContext reads turns/turn-legacy.md (3.2 MB)
                              ↓
        assembled prompt ~3.3 MB → gpt-4o 128k ctx → 400 context_length_exceeded
```

Rendering asymmetry: the *recent* turn display was already capped via `oneLine` at 200 chars. The *compacted* turn inline had no cap. A single oversized legacy file poisoned every subsequent analyze call.

#### Fix — three independent brakes

```go
// internal/memory/store.go — write side
const maxTurnBodyBytes = 64 * 1024

func sanitizeTurnText(s string) string {
    s = ansiEscapeRE.ReplaceAllString(s, "") // strip CSI escapes
    if len(s) <= maxTurnBodyBytes {
        return s
    }
    cut := maxTurnBodyBytes
    for cut > 0 && (s[cut]&0xC0) == 0x80 { // UTF-8 rune boundary walk
        cut--
    }
    return s[:cut] + "\n…[truncated]"
}
```

```go
// internal/memory/store.go — read side
const (
    maxBuildContextMatches    = 3       // top-K
    maxInlinedTurnBytes       = 8 * 1024 // per-entry cap
    maxBuildContextTotalBytes = 32 * 1024 // total-budget cap
)
```

The three caps are *independent*: write-side sanitize guards the disk; read-side sanitize catches legacy oversized files; `BuildContext` top-3 × 8 KB × 32 KB bracket stops any single request from blowing the model budget no matter how many memories match.

#### Test lock

`internal/memory/store_test.go`:

- `TestSanitizeStripsANSIAndCapsSize`
- `TestReadTurnFileSanitizesLegacyFile`
- `TestBuildContextCapsInlinedFullRef`

### Pattern 2: ReAct tool history accumulates unboundedly

#### Failure mechanism

`internal/agent/agent.go:BaseAgent.Execute` appended every `tool` role message to the `messages` slice forever. `tool.MaxInlineBytes` (~32 KB) caps one call but not the cumulative total. A 15-iteration explorer run with 3-5 reads per iteration reached ~450 KB of tool output in the conversation before the next LLM call 400'd.

#### Contributing sub-bug

`GrepTool` on a single-file path produced lines like `158:content` instead of `file.go:158:content` — ripgrep and GNU grep both drop the filename prefix when only one file is searched. `internal/agent/explorer.go:extractFileCoverage` then parsed the lineno as if it were a path, inflating the "discovered files" set with dozens of bogus entries per grep call. The LLM then tried to "finish reading" those bogus paths, accelerating tool-history growth.

#### Evidence — sum of `TOOLRESULT len=N` lines in the debug log

```
$ grep -oE 'TOOLRESULT .* len=[0-9]+' log.txt | awk -F= '{s+=$NF} END{print s}'
453721   # 450 KB accumulated, well past the model's context window
```

#### Fix — three coordinated changes

1. **`pruneToolHistory`** in `internal/agent/agent.go` — a 150 KB rolling budget that walks messages newest-to-oldest, keeps the hot window intact, and in-place stubs older `tool` role messages (preserving `ToolCallID` so OpenAI's tool-call pairing stays valid).

   ```go
   const maxToolHistoryBytes = 150 * 1024

   func pruneToolHistory(messages []llm.Message) bool {
       total := 0
       cutoff := -1
       for i := len(messages) - 1; i >= 0; i-- {
           if messages[i].Role != "tool" {
               continue
           }
           total += len(messages[i].Content)
           if total > maxToolHistoryBytes {
               cutoff = i
               break
           }
       }
       if cutoff < 0 {
           return false
       }
       // In-place stub older messages. ToolCallID is preserved so
       // OpenAI's tool_call ↔ response pairing stays valid.
       for i := 0; i <= cutoff; i++ {
           if messages[i].Role != "tool" || messages[i].Content == "" {
               continue
           }
           if strings.HasPrefix(messages[i].Content, "[earlier tool result elided") {
               continue // idempotent
           }
           messages[i].Content = fmt.Sprintf(
               "[earlier tool result elided — %d bytes. Re-invoke the tool if you need this content again.]",
               len(messages[i].Content))
       }
       return true
   }
   ```

   Called at the top of every ReAct iteration in `BaseAgent.Execute`.

2. **`GrepTool` -H flag** in `internal/tool/builtin.go` — force the filename prefix on every line:

   ```go
   args := []string{"-n", "-H"}
   // ... ripgrep path
   args := []string{"-rnEIH"}
   // ... GNU grep fallback
   ```

3. **`extractFileCoverage` defence in depth** in `internal/agent/explorer.go` — handle both `path:lineno:content` (match lines) and `path-lineno-content` (context lines) via a new `firstSeparatorBeforeLineno`, plus an `isValidFilePath` guard that rejects whitespace, lineno-only, or dash-group-separator `--` lines.

#### Test lock

`internal/agent/agent_test.go`:

- `TestPruneToolHistoryKeepsRecentAndStubsOlder`
- `TestPruneToolHistoryIdempotent`
- `TestPruneToolHistoryUnderBudgetNoop`

Memory: `feedback_context_length_exceeded_signatures.md` records the two patterns as a diagnostic recipe — check which stage fires the error, and you can localise the fix in minutes.

---

## Part 2 — Interactive status area 刷屏

### Symptom

In REPL mode the live status area printed the task list and spinner once, then *scrolled forever down the screen*, with the task title line appearing hundreds of times.

### Failure mechanism

`internal/render/renderer.go` uses `pterm.Area` to redraw the task list + status line in place. `pterm.Area` tracks how many rows to overwrite by **counting the `\n`s it wrote last frame**, not by asking the terminal how many visual rows the previous content actually occupied. When a status line exceeds the terminal width, the terminal autowraps it onto a second visual row — but `pterm.Area` only issues one cursor-up (`\e[1A`) on the next frame, so each new frame lands *below* the old one, scrolling the screen forever.

### Evidence — same byte stream, two terminal widths

Captured the raw PTY bytes from a real run, then replayed them through a tiny VT emulator at two widths:

| width | total lines reached |
|---|---|
| 140 cols | **8 lines** (clean in-place update) |
| 80 cols | **348 lines** (status line wraps → flooding) |

The difference is pure — same bytes, different terminal interpretation. 340 of the 348 lines are duplicate copies of the same task title.

### Fix

```go
// internal/render/renderer.go — clamp every line handed to pterm.Area
func (r *Renderer) redraw() {
    maxCols := pterm.GetTerminalWidth() - 4
    if maxCols < 20 {
        maxCols = 20 // degenerate fallback
    }
    // ... task list rendering with truncByDisplayWidth(line, maxCols) ...
}
```

```go
// Clamp an ANSI-bearing line to max display columns.
// Walks the string byte-by-byte, passing ANSI CSI escapes
// through without consuming budget, counting runes via
// runewidth.RuneWidth (so CJK = 2 cols).
func truncByDisplayWidth(s string, maxCols int) string {
    var b strings.Builder
    w := 0
    i := 0
    truncated := false
    for i < len(s) {
        if s[i] == 0x1b {
            if loc := reAnsi.FindStringIndex(s[i:]); loc != nil && loc[0] == 0 {
                b.WriteString(s[i : i+loc[1]])
                i += loc[1]
                continue
            }
        }
        r, size := utf8.DecodeRuneInString(s[i:])
        rw := runewidth.RuneWidth(r)
        if w+rw > maxCols {
            truncated = true
            break
        }
        b.WriteRune(r)
        w += rw
        i += size
    }
    if truncated {
        // ... walk back for ellipsis room, append "…\x1b[0m"
    }
    return b.String()
}
```

Also dropped the `· 28 calls, last: read_file` tail from the status line — redundant with the `detail` field and was the ~30 extra columns that pushed lines past the 80-col boundary in the first place.

### Test lock

`internal/render/renderer_test.go` — 5 tests: ASCII overflow, CJK overflow (the real repro fixture), ANSI passthrough, zero-budget degenerate path, no-truncation fast path.

### Lesson generalised (memory `feedback_pterm_area_visual_width.md`)

Any TUI library that redraws a region via cursor-up (pterm.Area, bubbletea, lipgloss in-place) counts **logical `\n`**, not visual rows. Every line you hand to such a library must be pre-clamped to display width. 80 cols is the floor, 132 is NOT safe.

---

## Part 3 — Wrong answer: `extractAnswerSymbols` audit

### Symptom

```
$ ./codrax -r "有多少个agent可以调用subagent"
唯一可以调用的是 SubExplorer。
```

The answer is **wrong**. `SubExplorer` is the *callee* — the sub-agent type being called. The question asks about the *caller* — the agent(s) that `agent.go:601` would auto-inject `propose_sub_agents` into. The correct answer is `explorer` (the only agent whose `b.name` matches a registered sub-agent's `Name()`).

### Initial reproducer

Before touching any fix logic, we built a deterministic characterization test (`bf09232`): construct the exact `EvidenceItem` that the failing run fed into `extractAnswerSymbols`, assert the current (wrong) output, add a skipped target test asserting the correct output.

```go
func TestReverseRefExtraction_CurrentBrokenOutput(t *testing.T) {
    items := []types.EvidenceItem{evidenceFromFailingRun()}
    syms := extractAnswerSymbols(items, "enumeration", nil)
    if syms[0].Name != "SubExplorer" {
        t.Errorf("characterization: current broken should yield SubExplorer")
    }
}
```

This characterization test passed from day one; it exists to *prevent silent regression* in the broken path until the fix lands. The corresponding `_TargetCorrectOutput` test is `t.Skip`'d with the exact un-skip condition.

### From single-case to systematic audit

The user then said: "*I want a complete plan, not a single-point fix. Audit from multiple languages, multiple scenarios, multiple angles.*" So we did a systematic three-dimensional walk:

**Dimensions**:

| axis | values |
|---|---|
| **question direction** | Forward (what does X), Reverse (who calls X), Identity (link literal) |
| **evidence shape** | 6 Predicate types (binds / returns / decorates / maps / resolution_chain / calls) |
| **language idiom** | Go, Java, Python, JS/TS, Ruby, Rust, YAML |

### Four structural failure modes found (F1-F4)

#### F1. Direction-blind dispatch

`answerSymbolFromEvidence` routed on evidence shape only. Every branch picked a fixed role (registered class for `binds`, receiver type for `returns`, callee for `calls`, rightmost hop for `resolution_chain`). The question's direction was never consulted. `questionKind` was *threaded through the signature* but only stored as metadata on the returned `AnswerSymbol.Kind`.

For a reverse-reference question on the same evidence, the answer lives on the **opposite side** — the caller, not the callee. There was no override path.

#### F2. Missing evidence shapes (unrouted)

`extractConcreteValues` in `explorer.go` produces 9+ evidence shapes but `answerSymbolFromEvidence` only handled 6:

| Predicate | produced by | routed? |
|---|---|---|
| `returns` | short-function scan | ✅ |
| `binds ONLY` / `binds` | constructor-passing call | ✅ via `isRegistrationShape` |
| `decorates` | Python/Java decorator scan | ❌ no case matches |
| `maps` | map/dict literal scan | ❌ no case matches |

Decorator and map evidence was produced, rendered into the finalizer's "Ground Truth" section textually, but *never became an AnswerSymbol*. In translation mode the finalizer had nothing to translate and fell back to soft shape constraints — L0-2 guarantees did not apply.

#### F3. Literal-blind terminal selection

For `Subject="SubExplorer.Name"`, `Object="\"explorer\"", Summary="SubExplorer.Name() returns \"explorer\""`, the case 2 branch:

```go
case ev.Kind == types.EvidenceConcrete && ev.Predicate == "returns":
    sub := ev.Subject
    if dot := strings.Index(sub, "."); dot > 0 {
        sub = sub[:dot]
    }
    sym.Name = firstUppercaseIdent(sub)
```

Returns `"SubExplorer"` — the **class name**. For a "what does X return?" question the answer is the literal `"explorer"`, which lives in `Object` and the chain's rightmost hop, never consulted.

#### F4. Monolingual identifier filter

`firstUppercaseIdent` requires a token starting with `[A-Z]`. Go/Java Pascal-case passes. Python `list_users`, Ruby `find_by_name`, JS `getUsers` (camelCase starting lowercase), YAML config keys — **all silently invisible**. For those ecosystems the picker returns `""` and the symbol drops off the list.

### Audit matrix (excerpt)

| direction ↓ / shape → | `binds` | `returns` | `decorates` | `maps` | `resolution_chain` | `calls` |
|---|---|---|---|---|---|---|
| **Forward** | class ✓ | **receiver ✗** | unrouted | unrouted | rightmost ✓ | Object ✓ |
| **Reverse** | **class ✗** | **✗** | unrouted | unrouted | **rightmost ✗** | **✗** |
| **Identity** | **✗** | **✗** | unrouted | unrouted | ± | ± |

**9 cells wrong, 2 unrouted entirely.** The reverse-reference case that opened the audit was one of the 9.

### Fix plan — three phases

Each phase independently shippable behind green tests.

#### Phase 1 — test grid lock (`8686dec`)

Extended the original reverse-ref reproducer into a **9-test grid**: one characterization test per gap asserting current broken output + one `t.Skip`'d target test asserting the post-fix behaviour. This lets reviewers see the whole failure surface in one place and catches any extraction regression while Phase 2/3 is pending.

#### Phase 2 — direction-aware extraction (`b4654b3`)

Introduced **three new concepts**:

```go
// internal/agent/erm.go
type AnswerRole int

const (
    // Legacy default: answer is the rightmost/terminal position,
    // extracted per evidence shape. Correct for forward questions.
    RoleTerminal AnswerRole = iota
    // Leftmost hop's caller. For "who calls X / 谁调用 X".
    RoleOrigin
    // String literal anywhere in the chain, walked right-to-left.
    // For return-value, name-of, and reverse-enumeration questions
    // where the answer IS a literal bridging caller and callee.
    RoleAnchor
)

func classifyAnswerRole(question, _ string) AnswerRole {
    if question == "" {
        return RoleTerminal // backward compat
    }
    lower := strings.ToLower(question)
    // RoleOrigin: "who/which <verb>" in English, 谁 in Chinese
    if strings.HasPrefix(lower, "who ") || strings.HasPrefix(lower, "which ") {
        for _, v := range reverseVerbs {
            if strings.Contains(lower, v) {
                return RoleOrigin
            }
        }
    }
    if strings.Contains(question, "谁") {
        return RoleOrigin
    }
    // RoleAnchor: return-value, name-of, and count + relationship-verb
    if strings.Contains(lower, "what does") && (strings.Contains(lower, "return") || strings.Contains(lower, "yield")) {
        return RoleAnchor
    }
    // ... Chinese 返回/名称 cues ...
    // ... count + verb: how many / 多少 / 数量 / 统计 + 调用 / 使用 ...
    return RoleTerminal
}
```

```go
// Unified hop walker replacing per-case extraction logic.
func pickHop(ev types.EvidenceItem, role AnswerRole, graph *repomap.Graph) string {
    switch role {
    case RoleOrigin:
        for _, hop := range splitHops(ev) {
            if name := extractCallerName(hop); name != "" {
                return name
            }
        }
        return ""
    case RoleAnchor:
        // STRICT: if no literal is found in any hop, return "" and
        // let the caller drop this item. Falling through to
        // pickTerminalLegacy would re-introduce the bug.
        hops := splitHops(ev)
        for i := len(hops) - 1; i >= 0; i-- {
            if lit := extractQuotedLiteral(hops[i]); lit != "" {
                return lit
            }
        }
        return ""
    case RoleTerminal:
        fallthrough
    default:
        return pickTerminalLegacy(ev, graph)
    }
}
```

The legacy per-shape switch moved into `pickTerminalLegacy` verbatim, so `RoleTerminal` (the empty-question default) still produces exactly the same symbol as before — every pre-existing unit test passes unchanged.

**Three subtle Phase 2 gotchas that the real-scenario test surfaced** (each required a second, third, fourth diagnostic loop):

1. **Analyzer rewrites vary per run.** First re-test: the analyzer produced `title="统计可以调用subagent的agent数量"`. My classifier only knew `多少/几个/有几` — it missed `数量`/`统计`. Added those plus `数量/统计/计算/列出/哪几/哪些` and, on the English side, `determine the number of / count the / list all / enumerate / find all`.

2. **The title alone isn't enough.** Second re-test: analyzer produced `title="识别可以调用subagent的agent"` (no count cue at all) but the task *description* said `"Determine how many agents can call subagent..."`. Plumbed `task.Description` through `AgentContext.CurrentTaskDescription` and concatenated title + description as the classifier input.

3. **`RoleAnchor` must be strict.** Third re-test: produced both `explorer` AND `SubExplorer` because the strict subset contained two chains for the same registration — one with the `returns "explorer"` trailing hop and one without. The literal-walker found `"explorer"` on the first, and `pickHop(RoleAnchor)` fell through to `pickTerminalLegacy` on the second, giving `SubExplorer`. Fix: `RoleAnchor` is now strict — if no literal is found in any hop, return `""` and let the caller drop the item. For anchor questions the answer *is* a literal; an evidence item with no literal is noise, not a fallback opportunity.

End-to-end after Phase 2:
```
L0-2 extracted 1 answer symbols
  answer_symbol[0]: explorer
→ "系统中可以调用 subagent 的 agent 只有一个，即 explorer。"
```

#### Phase 3 — missing shapes + multi-language (`c5e4f6e`)

Three additions:

1. **`hasTerminalEvidence` admits `decorates` and `maps`**:
   ```go
   if ev.Kind == types.EvidenceConcrete && ev.Predicate == "decorates" { return true }
   if ev.Kind == types.EvidenceConcrete && ev.Predicate == "maps"      { return true }
   ```

2. **`pickTerminalLegacy` gains two new cases** using a new `rightmostArrowHop` helper:
   ```go
   case ev.Kind == types.EvidenceConcrete && ev.Predicate == "decorates":
       // `@app.route("/api/users") → list_users`
       // `@GetMapping("/api/foo") → getFoo`
       target := rightmostArrowHop(ev.Object)
       return firstIdent(target) // language-agnostic picker

   case ev.Kind == types.EvidenceConcrete && ev.Predicate == "maps":
       // `"/api/users" → NewUserHandler()`
       // `types.AgentExplorer → NewExplorerAgent`
       val := rightmostArrowHop(ev.Object)
       if name := firstUppercaseIdent(val); name != "" {
           return stripNewPrefix(name) // Go constructor form
       }
       return firstIdent(val) // fallback for snake_case/camelCase
   ```

3. **`firstIdent` — language-agnostic identifier picker**:
   ```go
   // Accepts snake_case, lowercase-first camelCase, and PascalCase.
   // Minimum 3 chars. Used ONLY in Phase 3 paths; firstUppercaseIdent
   // stays in Go-specific legacy paths where uppercase-only filtering
   // is the right safety net.
   func firstIdent(seg string) string {
       // ... walks identifier starts via isIdentStart/isIdentChar,
       //     returns first ≥3-char run ...
   }
   ```

   A conservative "strict first, permissive as fallback" policy: the `returns` case keeps calling `firstUppercaseIdent` first and only falls back to `firstIdent` when the Go-strict picker draws a blank — so `SubExplorer.Name` → `SubExplorer` (unchanged) while `list_users.name` → `list_users` (new).

### Test grid status

All 8 audit cells + sanity non-regression now green:

| # | cell | status |
|---|---|---|
| 1 | (Reverse, binds, Go) | Phase 2 ✓ |
| 2 | (Identity, returns, Go) | Phase 2 ✓ |
| 3 | (Reverse, decorates, Python) | Phase 3 ✓ |
| 4 | (Reverse, decorates, Java) | Phase 3 ✓ |
| 5 | (Forward, maps, Go) | Phase 3 ✓ |
| 6 | (Reverse, resolution_chain, Go) | Phase 2 ✓ |
| 7 | (Identity, returns, Python snake_case) | Phase 3 ✓ |
| 8 | (Forward, binds, Go) | sanity non-regression ✓ |

Plus 14 classifier pattern tests (`TestClassifyAnswerRole_ReverseReference{Chinese,English}`) locking the keyword sets against future accidental deletion.

---

## Part 4 — Always 20 iterations: explorer early-stop audit

### Symptom

After Phase 2/3 landed and the answer was correct, the user observed:

> "Some questions take 60-120s. Some hit `stuck at stage explore after 5 visits` errors. The spinner counter goes up to 19 then resets to 0 and starts over."

### Data-driven diagnosis

Saved a debug log: `/tmp/earlystop_run.log` (148 KB, from `./codrax --log-level debug -r "有多少个agent可以调用subagent"`). Reconstructed the full per-iteration timeline:

| iter | tool calls | event | ERM status |
|---|---|---|---|
| 0 | 5 grep | Phase 0 breadth scan | unsat |
| 1 | 0 | Phase 0→1 transition | unsat |
| 2-5 | 11 | read `subagent.go` + 3 files | unsat |
| 6-9 | 4 | more reads | partial |
| 10-12 | 4 | read `types/subagent.go` | partial |
| **13** | **0** | **LLM writes correct answer** | **✅ ALL SATISFIED** |
| 14-18 | 4 | **reading `internal/agent/explorer.go` itself** | satisfied |
| 19 | 0 | soft-stop, hit max_iter=20 | satisfied |

**Wasted: iters 14-19 = 6 iters, ~23s, 4 tool calls, zero change to final answer.** The explorer was reading its own source code because some earlier iteration's LLM notes had mentioned `ContinuationPrompt`, which cross-referenced `explorer.go` into coverage tracking.

### Five root causes (any one blocks early-stop)

1. **`ShouldStop` hardcoded to `return false`.** The explorer never voluntarily stopped. The only exit was `idleStreakInDepth >= 2` inside `ContinuationPrompt` — a dead path because branches above reset the counter to 0 on every fire.

2. **`trackCrossReferences` had no relevance filter.** Every ≥8-char exported symbol the LLM mentioned in its notes pulled its defining file into `preScannedFiles`. Meta-symbols (`ContinuationPrompt`, `ToolSchema`, `BuildToolSchemas`) added `explorer.go`, `llm.go`, `mcp.go` to the coverage target — none of which the user asked about.

   Log evidence:
   ```
   line 1486: cross-ref: note mentions "ToolSchema" → added internal/llm/llm.go
   line 1487: cross-ref: note mentions "ToolSchema" → added internal/mcp/mcp.go
   line 1601: cross-ref: note mentions "ContinuationPrompt" → added internal/agent/finalizer.go
   line 1602: cross-ref: note mentions "ContinuationPrompt" → added internal/agent/sub_explorer.go
   ```

3. **`preScannedUnread` branch runs BEFORE the ERM-satisfied check.** Even with ERM fully green, the "pre-scanned unread" branch still fires because cross-ref keeps adding files.

4. **`partial-read hint` picks "worst-coverage" function without relevance.** When the LLM briefly read `explorer.go`, `detectPartiallyReadSymbols` flagged the 400-line `ContinuationPrompt` function as incomplete and forced the LLM to "finish" reading it — the explorer literally reading its own source code.

5. **`hasEnoughFacts` in `ParseOutput` doesn't honor ERM satisfaction.** The pre-audit logic required quantitative floors (toolDiversity, fileCoverage, evidenceQuality) independently of ERM. When ERM was satisfied but coverage was below the enumeration-mode 80% floor, `hasEnough=false` → `MissingFacts` → orchestrator re-dispatches explore → 5 visits → oscillation guard → error.

### Fix — three coordinated pieces

#### S1 — semantic early-stop in `ShouldStop`

```go
// internal/agent/explorer.go
func (e *explorerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
    if len(resp.ToolCalls) > 0 {
        return false // LLM still working
    }
    if e.phase != 1 {
        return false // still in breadth scan
    }
    if len(e.ermRequirements) == 0 {
        return false // no requirements → legacy path
    }
    // Refresh ERM state inside ShouldStop. Pre-fix, ERM was only
    // updated inside ContinuationPrompt which runs AFTER ShouldStop,
    // so the first soft-stop after a satisfying iteration saw stale
    // state and burned one extra iteration. The refresh is cheap —
    // pure note parsing, no file reads.
    //
    // Also: include the current soft-stop content in the note set
    // before checking. ContinuationPrompt is what normally appends;
    // without doing the same here we'd miss the iteration that just
    // produced the satisfying evidence.
    notesForCheck := e.investigationNotes
    if resp.Content != "" {
        notesForCheck = append(notesForCheck, resp.Content)
    }
    e.ermRequirements = checkRequirementSatisfaction(e.ermRequirements, notesForCheck, e.structuredEvidence)
    if !ermAllSatisfied(e.ermRequirements) {
        return false
    }
    // During the ReAct loop, e.structuredEvidence is nil (it's only
    // populated at ParseOutput time). Parse notes on the fly for
    // terminal-shape evidence — this covers [REGISTRATION] / [DIRECT]
    // tags the LLM has already written.
    noteEvidence := parseEvidenceItems(e.investigationNotes, "explorer.s1check")
    if !hasTerminalEvidence(noteEvidence) {
        return false
    }
    logging.Debug("[explorer] S1 semantic early-stop at iter=%d", iteration)
    return true
}
```

#### S2 — cross-reference entity filter in `trackCrossReferences`

```go
// internal/agent/explorer.go
// Collect ERM entities once, lowercased, for S2 filtering.
var ermEntities []string
for _, req := range e.ermRequirements {
    for _, ent := range req.Entities {
        if ent != "" {
            ermEntities = append(ermEntities, strings.ToLower(ent))
        }
    }
}

for symName, defs := range graph.SymbolDefs {
    // ... existing length / exported / commonality filters ...

    // S2: require entity overlap before pulling the symbol's file
    // into preScannedFiles. Empty ermEntities bypass the filter
    // (legacy behavior for non-entity questions).
    if len(ermEntities) > 0 {
        symLower := strings.ToLower(symName)
        match := false
        for _, ent := range ermEntities {
            if strings.Contains(symLower, ent) {
                match = true
                break
            }
        }
        if !match {
            continue
        }
    }
    // ... add def.File to preScannedFiles ...
}
```

For `subagent/agent` ERM entities, symbols like `NewSubExplorer`, `SubAgentRegistry`, `AgentName` pass (all contain "agent" as substring); meta-symbols like `ContinuationPrompt`, `ToolSchema`, `BuildToolSchemas` don't.

#### S3 alignment — `ParseOutput` respects ERM satisfaction

```go
// internal/agent/explorer.go — inside ParseOutput
if len(e.ermRequirements) > 0 {
    e.ermRequirements = checkRequirementSatisfaction(e.ermRequirements, e.investigationNotes, e.structuredEvidence)
    allSat := ermAllSatisfied(e.ermRequirements)
    if hasEnough && !allSat {
        // Existing: demote on ERM failure
        // ... unchanged ...
    } else if !hasEnough && allSat {
        // NEW: promote on ERM success. When ERM is fully satisfied
        // we know semantically that the required evidence exists;
        // blocking on a quantitative floor (80% file coverage for
        // enumeration mode) would re-enter the stage uselessly and
        // eventually trip the oscillation guard.
        logging.Debug("[explorer] ERM all-satisfied promote: hasEnough=true (floors: toolDiv=%v fileCov=%v evQual=%v)",
            toolDiversity, fileCoverage, evidenceQuality)
        hasEnough = true
    }
}
```

This is the ParseOutput-side mirror of S1: both trust the semantic ERM state over the quantitative heuristic.

### Stability

5-run test on the original audit question:

| run | elapsed | steps | S1 fired | correct |
|---|---|---|---|---|
| 1 | 35s | 2 | ✓ | ✓ |
| 2 | 40s | 2 | ✓ | ✓ |
| 3 | 37s | 2 | ✓ | ✓ |
| 4 | 49s | 2 | ✓ | ✓ |
| 5 | 44s | 2 | ✓ | ✓ |

**Average 41s (down from 65-85s), 5/5 correct, 0/5 oscillations.**

Over-fit check on two unrelated questions:
- `keywordSearch 的 IDF 评分怎么计算` → 34s, 8 iter, correct
- `BuildContext 如何组装 prior conversation` → 40s, 9 iter, correct

Both fired S1 cleanly and finished in fewer iterations than pre-fix, confirming the fixes aren't tied to the reverse-reference enumeration class.

---

## Part 5 — Truth-slice accumulation on self-loop

### User hypothesis (verbatim, paraphrased)

> `applyStageOutput` is directly appending to `BusContext.AnswerChains/AnswerSymbols`, no dedup, no per-round replacement. Multiple explore self-loops pile the old chains on top of the new ones in every subsequent prompt, amplifying noise.

### Verification

#### Code path (pre-fix)

`internal/orchestrator/orchestrator.go:603-606`:

```go
o.busCtx.EvidenceItems = append(o.busCtx.EvidenceItems, output.EvidenceItems...)
o.busCtx.FlowFindings  = append(o.busCtx.FlowFindings,  output.FlowFindings...)
o.busCtx.AnswerChains  = append(o.busCtx.AnswerChains,  output.AnswerChains...)
o.busCtx.AnswerSymbols = append(o.busCtx.AnswerSymbols, output.AnswerSymbols...)
```

Plain append on all four "truth" slices.

#### Why it bites on self-loop

The explorer evaluator is a **singleton** — `investigationNotes`, `searchResult`, `preScannedFiles` survive across dispatches. On a self-loop re-entry:

- Step 0 ends → `output.AnswerChains` = `[c1, c2]` → `busCtx` = `[c1, c2]`
- Step 1 entry → `investigationNotes` still has step 0's content
- Step 1 ends → `output.AnswerChains` = **`[c1, c2, c3]`** (full snapshot, re-ranked)
- plain append → `busCtx` = **`[c1, c2, c1, c2, c3]`** (2 duplicates)
- Step 2 ends → `output.AnswerChains` = `[c1, c2, c3, c4]`
- plain append → `busCtx` = `[c1, c2, c1, c2, c3, c1, c2, c3, c4]` (4 duplicates)

**Quadratic growth** (≈ N(N+1)/2 after N re-entries) with duplicates as the dominant content.

#### Direct measurement

Unit test (`internal/orchestrator/apply_stage_output_dedup_test.go`) calls `applyStageOutput` twice; second output = first output + 1 new item.

| slice | expected | pre-fix | post-fix |
|---|---|---|---|
| AnswerChains | 3 | **5** | **3** ✓ |
| AnswerSymbols | 3 | **5** | **3** ✓ |
| EvidenceItems (by ID) | 3 | **5** | **3** ✓ |

#### Downstream impact

`internal/context/builder.go:219-256` renders `AnswerSymbols` as bullets under "Extracted Answer Symbols" (the finalizer is told "list EXACTLY these symbols, no others"); and `AnswerChains` as bullets under "Ground Truth". Duplicates either force duplicate names in the finalizer output or cause silent collapse that violates the strict-list contract.

`formatEvidenceItems(items, 18)` caps at 18 but **post-sort, no dedup**. Since sort is by `(Source, LineStart, ID)`, step 0 items share keys with step 1 re-emitted copies and occupy the first slots — step 1's genuinely new items fall off the 18-item cap with a `... and N more` footer. The finalizer never sees the new evidence.

### Fix

Four coordinated edits:

1. **Export `MergeEvidenceItems` / `MergeFlowFindings`** — the unexported `mergeEvidenceItems` / `mergeFlowFindings` in `internal/agent/evidence.go` already do proper ID-based dedup; just needed exported wrappers so the orchestrator can call them.

2. **New `MergeAnswerChains` / `MergeAnswerSymbols`** in `internal/types/evidence.go`:

   ```go
   // MergeAnswerChains — dedup by full string, preserve first-seen order.
   func MergeAnswerChains(groups ...[]string) []string {
       seen := make(map[string]struct{})
       var out []string
       for _, g := range groups {
           for _, c := range g {
               if _, ok := seen[c]; ok {
                   continue
               }
               seen[c] = struct{}{}
               out = append(out, c)
           }
       }
       return out
   }

   // MergeAnswerSymbols — dedup key is Name + File + Line composite.
   // Chain / Kind / Rationale are treated as metadata that may vary
   // run-to-run without implying a different symbol.
   func MergeAnswerSymbols(groups ...[]AnswerSymbol) []AnswerSymbol {
       seen := make(map[string]struct{})
       var out []AnswerSymbol
       for _, g := range groups {
           for _, s := range g {
               key := s.Name + "\x1f" + s.File + "\x1f" + itoa(s.Line)
               if _, ok := seen[key]; ok {
                   continue
               }
               seen[key] = struct{}{}
               out = append(out, s)
           }
       }
       return out
   }
   ```

3. **Rewrite `applyStageOutput` to route through the mergers**:

   ```go
   o.busCtx.EvidenceItems = agent.MergeEvidenceItems(o.busCtx.EvidenceItems, output.EvidenceItems)
   o.busCtx.FlowFindings  = agent.MergeFlowFindings(o.busCtx.FlowFindings, output.FlowFindings)
   o.busCtx.AnswerChains  = types.MergeAnswerChains(o.busCtx.AnswerChains, output.AnswerChains)
   o.busCtx.AnswerSymbols = types.MergeAnswerSymbols(o.busCtx.AnswerSymbols, output.AnswerSymbols)
   ```

4. **Leave `ToolResults` / `MCPResponses` / `RepoFacts` on plain append** — history-style slices, per-call granularity is the desired semantic. Guarded by `TestApplyStageOutput_KeepsAppendingToolResultsOnSelfLoop`.

### Orthogonality to S1/S2

Even with S1/S2 making self-loops rare on ERM-entity-rich questions:

- Questions with empty `ermRequirements` fall outside S1 (its first guard)
- Multi-stage pipelines each re-emit their own cumulative view
- Future agents get dedup automatically

The fix is **correct-by-construction regardless of S1/S2**. The two audits are orthogonal: S1/S2 makes loops rare; this fix makes loops harmless when they do happen.

---

## Part 6 — Determinism: stable multi-key sort

### Symptom (latent, not user-reported)

Having fixed correctness and speed, one stability gap remained in `internal/agent/erm.go:identifyAnswerChains`:

```go
sort.Slice(candidates, func(i, j int) bool {
    return candidates[i].score > candidates[j].score
})
```

**Two problems with this single-key, unstable sort**:

1. `sort.Slice` is a **pattern-defeating quicksort** — not stable. For candidates with identical `float64` scores (common when overlap × bonus lands on the same product), output order depended on Go runtime hash seed, map iteration order, and input iteration order. Same input could produce different output across runs.

2. **No principled tie-break layers.** When scores differ slightly, there was no signal for "prefer the L0-1 passing candidate" or "prefer the shorter chain" or "prefer the deterministic concrete_values extractor (0.95 confidence) over the LLM notes parse (0.8)".

### Fix — six-key stable comparator

```go
// internal/agent/erm.go
type scored struct {
    text        string
    score       float64
    src         types.EvidenceItem
    strictOK    bool
    confidence  float64 // mirror of src.Confidence, cached for sort
    chainLength int     // strings.Count(Summary, "→") + 1, min 1
    sourceLine  int     // src.LineStart, or noSourceLineSentinel
    summary     string  // lex tie-break final key
}

// ... during candidate construction ...
chainLen := strings.Count(ev.Summary, "→") + 1
if chainLen < 1 {
    chainLen = 1
}
srcLine := ev.LineStart
if srcLine <= 0 {
    srcLine = noSourceLineSentinel // 1 << 30
}

candidates = append(candidates, scored{
    text:        display,
    score:       float64(overlap) / float64(len(entities)) * bonus,
    src:         ev,
    strictOK:    strictOK,
    confidence:  ev.Confidence,
    chainLength: chainLen,
    sourceLine:  srcLine,
    summary:     ev.Summary,
})
```

```go
// Stable multi-key sort.
sort.SliceStable(candidates, func(i, j int) bool {
    ci, cj := candidates[i], candidates[j]
    // 1. score descending
    if ci.score != cj.score {
        return ci.score > cj.score
    }
    // 2. strictOK=true first (L0-1 passing beats demoted)
    if ci.strictOK != cj.strictOK {
        return ci.strictOK
    }
    // 3. confidence descending
    if ci.confidence != cj.confidence {
        return ci.confidence > cj.confidence
    }
    // 4. chainLength ascending (shorter = more precise)
    if ci.chainLength != cj.chainLength {
        return ci.chainLength < cj.chainLength
    }
    // 5. sourceLine ascending (earlier code wins; unknown sorts last)
    if ci.sourceLine != cj.sourceLine {
        return ci.sourceLine < cj.sourceLine
    }
    // 6. summary lexicographic — deterministic final tie-break
    return ci.summary < cj.summary
})
```

### Key design decisions

- **Stable sort + lex tie-break combo**: `SliceStable` preserves equal-keyed candidates' insertion order; lex is the explicit final tie-break. Together they guarantee the same input produces byte-identical output across runs.

- **`sourceLine` sentinel = `1 << 30`**: a large but bounded int, not `math.MaxInt`, so the comparator stays well-defined on 32-bit platforms.

- **`float64` equality on `score`**: looks dangerous but is safe here because both sides use the exact same arithmetic path (`float64(int)/float64(int)*bonus`), so equal inputs yield bit-identical floats. If they differ by ε we correctly fall through the `>` branch.

- **Demote-not-drop invariant preserved**: L0-1 predicate demotion still applies (`bonus *= 0.2` for terminal fail, `*= 0.1` for origin fail, `strictOK = false`). Demoted items still enter the loose `chains` list for Ground Truth fallback, but `strictOK=false` pushes them below any `strictOK=true` peer at the same score.

### Test lock

4 new tests in `internal/agent/erm_test.go`:

- `TestIdentifyAnswerChains_StableSortEqualScoreByStrictOK`
- `TestIdentifyAnswerChains_StableSortEqualScoreByConfidence`
- `TestIdentifyAnswerChains_StableSortEqualScoreByChainLength`
- `TestIdentifyAnswerChains_StableSortDeterministicAcrossRuns` (6 identical invocations, byte-identical output each time)

All 6 pre-existing `identifyAnswerChains` tests still pass unchanged — the constructor-origin demotion still loses to register linkage, the range-terminal chain still gets demoted below the literal-return chain, etc.

---

## Cross-cutting lessons

Five patterns kept recurring across the audit cascade:

### 1. Save data before theorising

Every fix in this session was anchored in a preserved debug log (`/tmp/repro_run.log`, `/tmp/earlystop_run.log`, `/tmp/codrax_repro.raw`). Log first, diagnose from the file, then code. Re-running for new hypotheses would have produced different LLM output and masked the real bugs under sampling noise. This is the "Debug runs must save to file first" memory rule in action.

### 2. Trace full data flow before stating a root cause

The early-stop audit had 4 corrections between "I know what's wrong" and "I actually know what's wrong":
- *"S1 should fire when ERM is satisfied"* → fired one iter late because ERM was refreshed AFTER ShouldStop
- *"refresh ERM inside ShouldStop"* → still stale because the in-flight soft-stop content wasn't yet in notes
- *"append the current content before checking"* → structured evidence was nil during the loop
- *"parse notes for terminal evidence on the fly"* → real-scenario still failed because ParseOutput's hasEnough still used the old floors
- *"add promote branch to hasEnough"* → finally worked

Each step would have been shorter with a full end-to-end trace up front. This is the "Trace full data flow before concluding root cause" memory rule.

### 3. Per-item caps are not enough

Pattern 1 had per-turn caps (`maxTurnBodyBytes=64KB`) but no total-budget cap, so matching 5 memories × 64KB = 320KB still blew the window. Pattern 2 had per-tool caps (`tool.MaxInlineBytes=32KB`) but no cumulative cap, so 15 iters × 30KB = 450KB. The fix for both: an **independent cumulative budget** at the aggregation point. This is the `feedback_cap_vs_evidence_separation.md` memory rule: any subsystem that accumulates externally-sourced text needs an independent cumulative budget on top of any per-item cap.

### 4. Structural fixes beat keyword lists

The classifier keyword lists (`多少/数量/统计/计算/列出/哪几/哪些`) look fragile, but each entry encodes a *class of phrasings*, not a specific eval case's exact string. Removing any one entry loses a class of questions, not one eval case. The over-fit audit (`feedback_no_overfitted_solutions.md`) passes by construction: reverse test (no eval case is uniquely tied to any rule), no-bait test (removing one rule doesn't flip any specific case from correct to wrong).

### 5. Orthogonal fixes compound

The early-stop fix (S1/S2) and the truth-slice dedup are both required:
- Early-stop alone: reduces self-loop frequency from "always" to "sometimes"
- Dedup alone: makes self-loops harmless but does nothing about latency
- Together: loops rare AND harmless when they happen

Same with the stable sort: latency and correctness are both fixed, but without determinism two identical runs can still produce different finalizer prompts. Three orthogonal properties (latency, correctness, determinism) needed three orthogonal fixes.

---

## Appendix A — Debug log grep recipes

When a future symptom matches one of the classes in this document, use these one-liners to localise the fix:

```bash
# Pattern 1 (analyze stage memory blow-up):
grep "analyze" log | grep "context_length_exceeded"
ls -lh memory/<slug>/turns/  # look for files > 100KB

# Pattern 2 (explorer tool history blow-up):
grep -oE "TOOLRESULT .* len=[0-9]+" log | awk -F= '{s+=$NF} END{print s}'
# >200KB → Pattern 2

# Explorer early-stop not firing:
grep "S1 semantic early-stop" log | wc -l  # 0 → S1 never fired
grep "ERM all-satisfied promote" log | wc -l

# Cross-reference pollution (S2):
grep -c "cross-ref: note mentions" log  # >5 per run → leaking

# Orchestrator oscillation:
grep -oE "step=[0-9]+ stage=[a-z_]+" log | sort | uniq -c
# more than 1 explore step → check ParseOutput hasEnough promote
```

## Appendix B — Key files modified in this session

| file | parts |
|---|---|
| `internal/memory/store.go` | Part 1 (Pattern 1) |
| `internal/memory/store_test.go` | Part 1 tests |
| `internal/agent/agent.go` | Part 1 (Pattern 2) |
| `internal/agent/agent_test.go` | Part 1 tests |
| `internal/tool/builtin.go` | Part 1 (grep -H) |
| `internal/agent/explorer.go` | Parts 1, 3 (Phase 2 plumbing), 4 (S1/S2/promote) |
| `internal/agent/explorer_test.go` | Part 1 tests |
| `internal/render/renderer.go` | Part 2 |
| `internal/render/renderer_test.go` | Part 2 tests |
| `internal/agent/erm.go` | Parts 3 (Phase 2/3), 6 (stable sort) |
| `internal/agent/erm_test.go` | Parts 3, 6 tests |
| `internal/agent/evidence.go` | Part 5 (exported mergers) |
| `internal/agent/reverse_reference_repro_test.go` | Part 3 reproducer |
| `internal/agent/answer_symbol_extraction_audit_test.go` | Part 3 audit grid |
| `internal/types/context.go` | Part 3 (CurrentTaskDescription) |
| `internal/types/evidence.go` | Part 5 (MergeAnswerChains/Symbols) |
| `internal/context/builder.go` | Part 3 |
| `internal/orchestrator/orchestrator.go` | Part 5 |
| `internal/orchestrator/apply_stage_output_dedup_test.go` | Part 5 tests |

## Appendix C — Memory notes produced

- `feedback_context_length_exceeded_signatures.md` — Part 1 pattern catalogue
- `feedback_pterm_area_visual_width.md` — Part 2 lesson
- `project_reverse_reference_enumeration_bug.md` — Part 3 original single-case note
- `project_answer_symbol_extraction_audit.md` — Part 3 comprehensive plan
- `project_explorer_early_stop_audit.md` — Part 4
- `project_applystage_dedup.md` — Part 5
- `project_answer_chain_stable_sort.md` — Part 6
