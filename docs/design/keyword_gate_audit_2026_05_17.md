# Keyword-gate audit — 2026-05-17

**Question**: Does the code use keyword matching on user intent (raw
question text) OR on LLM response content to drive hard logic
decisions (branches, gates, retries)?

**Why this matters** (CLAUDE.md architectural red line):

> Precise signals for hard gates, noisy signals for soft guidance.
> Hard structural gates (emit-time hard rejects, contract.Check soft
> fails) MUST read PRECISE signals — single boolean flags, single
> integer comparisons, verbatim string substring matches against a
> typed field, schema-validated typed enums. Noisy signals (ranker
> scores, grep candidate counts, similarity heuristics,
> frequency-weighted lists) MUST drive only SOFT guidance —
> skill-prompt directives, advisory log lines, retry-hint
> suggestions. The reverse (hard gates on noisy signals) fires on
> signal-side noise and produces user-visible failures for
> STRUCTURALLY fine questions.

**Audit method**: grep-based scan + manual verification of
`strings.Contains` / `strings.HasPrefix` / `regexp.MatchString` /
hardcoded keyword arrays applied to `Objective` / `RawRequest` /
`Content` / `Body` / `Reasoning` / `Detail` / `Summary` fields, then
classified by whether the match drives a hard branch or only soft
guidance.

## Architectural verdict

**Main pipeline respects the red line.** Several recent refactors
explicitly enforce it; the relevant ones leave breadcrumbs:

- `internal/repl/turn_policy.go:23-25` —
  > "Schema is the load-bearing contract; no Go-side keyword
  > matching lives here. The structural enum + its description does
  > the disambiguation."
- chitchat classifier + continuation classifier already migrated
  away from hardcoded keyword tables to schema-driven LLM emits
- analyzer reads typed predicates
  (`Predicates.IsCountQuestion` / `IsCrossComponent` /
  `IsCategoryEnumeration`) instead of grepping the raw question
- evidence grounding / contract check route on typed
  `Violation.Kind` enum

**Residual violations: 2 HIGH + 1 MEDIUM**. None on user-intent /
analyzer / REPL path; all inside orchestrator's fallback / reflector
plumbing. Severity is bounded — they affect "which fallback path is
chosen" or "how the planner hint is worded", not "does the user's
question get processed at all".

## Findings

### HIGH-1: fallback target keyword-matched on Violation.Detail prose

**File**: `internal/orchestrator/fallback_policy.go:550-556`

```go
detail := strings.ToLower(v.Detail)
if strings.Contains(detail, "appears 0 time(s)") {
    if strings.Contains(detail, "kind=diagram") || strings.Contains(detail, "kind=table") {
        return FallbackFinalizerOnly
    }
}
```

**Why it violates**:

- `v.Detail` is a free-form string composed by violation-producing
  code via `fmt.Sprintf(...)` (see e.g.
  `denied_token_answer_check.go:73` and other validators)
- the return value `FallbackFinalizerOnly` vs the default
  `FallbackBackToExtract` controls which pipeline stage gets
  re-run — a hard structural decision
- the typed signal `v.Kind == ViolBlockCoverageMissing` is already
  populated; the missing block kind (`"diagram"` / `"table"`) is
  only available through prose grepping
- if a future refactor changes the Detail template (e.g.
  i18n, wording polish), the gate silently fails and over-rewinds
  the pipeline

**Fix surface**:

Add a typed field on the `Violation` struct that the
validator producing `ViolBlockCoverageMissing` populates verbatim:

```go
// internal/types/violation.go
type Violation struct {
    // ...
    MissingBlockKind string // "diagram" / "table" / "" (only meaningful for ViolBlockCoverageMissing)
}
```

Validators that emit `ViolBlockCoverageMissing` fill it with the
typed `types.AnswerBlockKind` value. `fallback_policy.go`
switches on `v.MissingBlockKind` instead of grepping prose. The
`Detail` field continues to carry the human-readable
description for logs / debug.

**Estimated effort**: ~100 LOC across `types/violation.go`,
validators emitting the violation kind, `fallback_policy.go`, +
tests pinning the fallback branch for diagram / table / overflow
cases.

### HIGH-2: reflector critique prose keyword-rewrites planner hint

**File**: `internal/orchestrator/stage_hooks.go:1057`

```go
critique := out.Observation  // LLM-generated prose
if strings.Contains(critique, "Preserve:") {
    heuristic = strings.ReplaceAll(heuristic,
        "Files modified by the previous plan (suspect list — ...",
        "Files modified by the previous plan (review for compatibility; the critic above identified what to preserve):",
    )
}
```

**Why it violates**:

- `out.Observation` is free-form LLM prose from the reflector
  critic
- the keyword `"Preserve:"` makes a hard branch that rewrites the
  planner's hint, changing what the next LLM dispatch sees
- a false positive (the substring appears in a code example, in a
  variable name, or in a Chinese translation that still uses
  "Preserve:" verbatim) fires the rewrite incorrectly
- a false negative (the critic uses different wording —
  "保留", "Keep these:", "Don't touch:") leaves the planner with
  contradictory guidance

**Fix surface**:

Extend the reflector's structured response schema with a typed
field:

```go
// internal/orchestrator/reflector.go (or wherever ReflectionResult lives)
type ReflectionResult struct {
    Observation       string   // existing prose
    PreservationClauses []string // NEW — typed list of symbols / aspects to preserve
}
```

The reflector LLM emits `preservation_clauses` as a JSON array via
its tool schema. `stage_hooks.go` reads the typed slice
(`len(out.PreservationClauses) > 0`) instead of grepping the prose.

**Estimated effort**: ~80 LOC across reflector tool schema, parse
path, `stage_hooks.go`, + tests pinning both routes (with / without
preservation clauses).

### MEDIUM: embedded-tool-call detection on LLM content prose

**File**: `internal/agent/agent.go:1403`

```go
if looksLikeEmbeddedToolCall(resp.Content) {
    // grep "```json" + emit_* tool name in content
    continue // inject correction hint and retry
}
```

**Why it's MEDIUM, not HIGH**:

- the typed primary signal is `len(resp.ToolCalls) == 0` —
  the keyword-on-prose check only fires when the LLM emitted NO
  tool_use blocks AND non-empty content
- in that branch the LLM has effectively failed to call a tool;
  the keyword check distinguishes "model tried to call a tool via
  text" (worth a corrective hint) from "model gave up and wrote
  prose" (different recovery path)
- false positive cost is bounded: an extra retry with a
  "use tool_use blocks" hint. No reject, no user-visible failure

**Suggested action**: keep as-is, but add a comment block referencing
this audit and noting the typed precondition
(`len(resp.ToolCalls) == 0`). Promotion to HIGH only warranted if
production logs show false-positive retries.

### LOW: soft-guidance keyword matches (allowed by red line)

These match prose to drive ONLY soft signals (caveats / log
lines / retry hints), not hard gates. Listed for completeness:

| File | Line | Drives | Notes |
|---|---|---|---|
| `internal/orchestrator/denied_token_answer_check.go` | ~159 | SOFT violation (caveat marker) | matches answer prose against `"unverified"` / `"未验证"` etc. False positives result in spurious caveats, not rejects |
| `internal/agent/explorer.go` | 7622, 14137 | structural match | `strings.Contains(r.Summary, "matching lines")` — `r.Summary` is a tool-emitted fixed string, not LLM prose; structured match |
| `internal/orchestrator/contract_check.go` | (multiple) | various soft signals + typed enum matches | spot-checked: matches `doc.Summary` against names from typed evidence anchor set, not free user/LLM prose |

## Numerical summary

| Severity | Count | User-visible failure risk |
|---|---|---|
| CRITICAL (hard gate on user question keyword) | **0** | none |
| HIGH (hard gate on LLM response prose) | **2** | bounded — wrong fallback path / wrong planner hint |
| MEDIUM (boundary case, typed precondition) | **1** | none (extra retry only) |
| LOW (soft guidance, allowed by red line) | several | none |

## Scheduling

Both HIGH items are single-session, low-risk schedulable:

- HIGH-1 (`fallback_policy.go`): ~100 LOC + tests. Stand-alone
  session. Could pair with §2.3 finalizer typographic fix
  (similar shape: extend a typed structure, replace prose grep).
- HIGH-2 (`stage_hooks.go`): ~80 LOC + tests + reflector tool
  schema bump. Stand-alone session.

Both are independent of Phase 2B (evaluator refactor) and the
forensic follow-up items in
`docs/design/post_phase2a_forensic_followups.md`. Schedule whenever
operator chooses; the red-line violation is structural debt, not
an active user-facing bug.

## Why nothing on user-intent / REPL / analyzer

The audit specifically looked for prose grep on `Objective` /
`RawRequest` / user-message channels. None found in non-test code.
The `turn_policy.go` comment block (line 23-25) makes the
architectural intent explicit, and the chitchat classifier +
continuation classifier follow the same schema-driven pattern.
Analyzer routes on typed `Predicates.*` booleans and
`Intent` / `Scenario` / `QuestionFamily` typed enums.

The residual HIGH findings sit in implementation layers downstream
of analyzer's typed classification, in plumbing the LLM never sees.
Misclassification here doesn't cause "structurally fine question
fails" — it causes "fallback re-runs a different stage than ideal".

## Open items for future audits

This audit covered prose keyword matching. Adjacent
"noisy → hard gate" patterns worth a separate sweep someday:

1. **Heuristic counter thresholds driving hard rejects** — e.g.
   a `count >= N` check where the `count` is derived from a noisy
   source (ranker score, grep candidate count). Spot-check
   suggests these are mostly SOFT (drive prompt hints), but
   exhaustive verification was out of scope here.

2. **Path canonicalisation matches** — the multi-repo forced-read
   bug in `post_phase2a_forensic_followups.md` §2.1.B is a typed-
   path-resolution issue, not a keyword-gate issue, but related
   in spirit (downstream code assumed a stable path shape that
   the upstream layer didn't actually guarantee).

3. **Tool name string matching across the codebase** — many
   places `strings.Contains(toolResult.ToolName, "emit_")`. This
   is structural (tool names ARE the typed enum surface) but
   worth a sweep to confirm none stray from the canonical tool
   registry.

Cross-references:

- `docs/design/post_phase2a_forensic_followups.md` — separate
  forensic findings (P1A typographic, phantom-path, midloop
  saturation, evaluator singleton)
- `docs/design/phase_2b_explorer_parallel_dispatch.md` — Phase 2B
  big refactor plan
- `CLAUDE.md` "Architectural principle (red line)" — the source of
  the rule this audit enforces
