# Prompt glossary — user-facing vocabulary for LLM-facing strings

This document defines the canonical replacements for implementation
jargon that has historically leaked into LLM-facing prompt surfaces
(skill configs, tool schema descriptions, retry hints, BuildInitial-
Instruction bodies). The `TestNoInternalTermsInX` lints in
`internal/skill`, `internal/tool`, and `internal/agent` mechanically
enforce the blocklist in `internal/skill/glossary.go`.

## When this table applies

- **Applies**: any Go string literal that is rendered into a prompt
  the LLM actually reads — `skill.Config.Goal / Workflow / OutputFormat
  / Prohibitions`, every `emit_*` tool's `Description()` return and
  every `"description"` property inside `Parameters()`, every
  `LoopSignal.Hint` body, every `BuildInitialInstruction` return, any
  `StageOutput.Error` / `StageReport` that the orchestrator renders
  into the next dispatch's Retry Directive.
- **Does not apply**: Go identifiers, struct field references, import
  paths, Go comments, logger calls (`logging.Debug/Info/Warning/Error`,
  `log.Printf`, `fmt.Fprintf`). The lints filter those out.

## Section 1 — Core IR / data-structure type names

Never mention these Go type names to the LLM. The LLM does not know
or need to know about codrax's internal data shapes.

| Leave out of prompts | Use instead |
|---|---|
| `AnalysisIR` | (silently drop; if you need to reference "what this stage produces", say "the classification" or "the analyzer's output") |
| `RequestModel` | "classification" |
| `AnswerContract` | "answer contract", or unpack to "the target answer shape and the fields it requires" |
| `AnalyzerHints` | (silently drop) |
| `TermGraph` / `TaskGraph` / `EvidencePlan` / `QualityGate` / `RiskMatrix` / `HypothesisSet` | (silently drop — these are system-derived, the LLM never writes them) |
| `MutableState`, `BusContext`, `StageOutput`, `EvidenceClosure`, `ReadSet`, `PendingReads`, `RepairDirective` | (silently drop; describe observable behavior instead) |
| `GroundingStatus`, `AnchorKind` | describe the constraint directly (e.g. "an evidence item whose anchor type is `definition`", not "an AnchorKind=definition evidence item") |

## Section 2 — Contract-field leakage (Go constants surfacing as prose)

| Leave out of prompts | Use instead |
|---|---|
| `MustInclude` / `must-include floor` / `must-include count` / `must-include list` | "required-symbol floor" |
| `cardinality baseline` / `cardinality cross-check` | "expected answer count", or describe the rule: "the count is cross-checked against…" |
| `terminal-evidence count` | "the investigation's count" (context clarifies which count) |
| `effective floor` | "expected answer count" |
| `Ground Truth evidence` | "evidence from the investigation" |

The four terms `cardinality baseline` / `terminal-evidence count` /
`must-include count` / `effective floor` are a family — they all name
the same scalar (the max of the investigation's count and the
classification's declared floor). Use **one** replacement per prompt
surface: `expected answer count (the larger of the investigation's
count and the classification's declared required floor)`.

## Section 3 — Internal pipeline vocabulary (stage / phase / tier labels)

| Leave out of prompts | Use instead |
|---|---|
| `Turn A` / `TurnA` | "investigation stage", or context-free "earlier stage" |
| `Turn B` / `TurnB` | "synthesis stage", or "this stage" |
| `Phase 0` / `Phase 1` / `Phase 2` | describe the actual activity: "breadth scan", "depth investigation" |
| `Tier 1` / `Tier 2` / `T1a` / `T1b` / `T1c` | delete; if a threshold distinction matters, state the threshold concretely |

Exception: `## PHASE 1:` / `## PHASE 2:` as a numbered section label
inside a Workflow (e.g. explore-skill's two-phase workflow) is fine
because it describes the workflow's own numbered steps, not an
internal pipeline stage. The blocklist matches case-sensitive
`Phase 0` etc. with a space; `PHASE 1:` as a header does not match.

## Section 4 — Design-doc / commit-tracking acronyms

| Leave out of prompts | Use instead |
|---|---|
| `CGEC` | describe the invariant: "the evidence-closure check" or the specific rule |
| `ERM` | "the evidence requirement check" |
| `HDP` | "hypothesis planning" |
| `C0'` / any `C[0-9]` / `B[0-9]a?` | delete; name the rule |

## Section 5 — Validator / gate machinery disclosure

Never tell the LLM "the system does X" or name an internal gate.
Describe the **rule** as a fact the LLM must follow.

| Leave out of prompts | Use instead |
|---|---|
| `findings_validator` | (no substitute — describe what the check enforces) |
| `literal-grounding gate` | "a citation whose cited line does not contain the literal will be rejected" |
| `coverage gate` / `phase1_unread gate` / `phase1-unread gate` | "the file-coverage rule" |
| `the system does X` / `the system will X` / `the system validates X` | "this tool validates X" / "X will be rejected" / "X is required" |
| `deterministic validator` | (drop; describe the behavior) |

## Section 6 — Log-triage internal layers

Never name `Layer 1` / `Layer 2` / `Layer 3` / `Layer 4` in prompts.
Describe the fields by function.

| Leave out of prompts | Use instead |
|---|---|
| `Layer 1 (Meta)` | "meta fields (lang, signals, summary)" |
| `Layer 2 (Errors)` | "errors tree" |
| `Layer 3 (Residue)` | "unknown_chunks" |
| `Layer 4` | never mentioned — it is system-derived |

## Section 7 — Keyword-example red line

Enum descriptions in `AnalysisIntentChoices` / `AnalysisComplexity-
Choices` / `AnalysisQuestionKindChoices` / `AnalysisAnswerShape-
Choices` must **not** contain quoted user-wording examples. Red line:
`feedback_no_custom_keyword_matching.md`.

Forbidden surface forms:

| Forbidden example | Structural replacement |
|---|---|
| `"how many X"`, `"size of Y"` | "the answer is a single scalar: a count, a size, a function return, a literal value" |
| `"list all X"`, `"list every X"` | "the answer is a set of distinct named items" |
| `"what is X"`, `"X 是什么"` | "the subject is a literal lookup" |
| `"how does X work"`, `"X 怎么工作"` | "the subject is a single-component mechanism question" |
| `"compare A and B"`, `"对比 A 和 B"` | "the subject relates or compares two distinct components" |
| Chinese imperative cues (`统计`, `多少`, `几个`, `怎么`, `什么时候`) | describe structural intent instead |

Enforced by `TestNoKeywordExamplesInEnums` (batch 4A).

## Parser-contract headers (do NOT rename)

These section titles are parsed by downstream stages. Renaming any of
them without a corresponding parser update causes silent failures.
This table is informational — the refactor never edits these strings.

- `Prior Stage Findings` (and its sub-headers `## Resolution Chains`,
  `## Answer Symbols`, `### Resolution Chains`)
- `Structured Evidence`
- `Hypothesis Verdicts`
- `Attached Runtime Log`
- `Log Triage — Validated Extraction`
- `Raw Tool Outputs from the Investigation`
- `Extracted Answer Symbols (deterministic, authoritative)`
- `Answer Symbols (deterministic floor, may extend with cited evidence)`
- `Unverified Leads (not for citation)`
- `Unverified Analyzer Findings`
- `Dataflow Findings`
- `Relevant Files`
- `Known Facts`
- `Subject Match Summary`
- `User Request`, `User Preferences`, `Constraints`, `Retry Directive (READ FIRST)`, `Prior Conversation (reference only)`, `Agent Identity`, `Reasoning Hygiene`, `Think Aloud`, `Pipeline State`, `Skill Goal`, `Workflow`, `Output Format` / `Exploration Contract`, `Prohibitions`

## Batch lifecycle of the lint

1. **Batch 1** (this commit): lint created in report-only mode
   (`t.Log`). Lists the 58 baseline violations.
2. **Batch 2A/2B**: purge of jargon categories §1–§6; lint tightened
   per category as each ships.
3. **Batch 3A/3B**: structural section reorganization (analyzer
   Pre-scan migrates to Workflow, resolves three contradictions).
4. **Batch 4A**: purge of keyword examples §7 and new
   `TestNoKeywordExamplesInEnums` gate (t.Fatal).
5. **Batch 4B**: all three `TestNoInternalTermsInX` tests promoted to
   `t.Fatal`. Regressions become CI red.

## How to propose a new blocklist entry

1. Add the token to `InternalTermsBlocklist` (or `KeywordExamplePhrases`
   for the enum-only lint) in `internal/skill/glossary.go`.
2. Run `go test ./internal/skill/... ./internal/tool/... ./internal/agent/...
   -run TestNoInternalTerms -v` and review the new hits.
3. Add a row to this file explaining the replacement.
4. Ship in the same commit as the prompt edit that removes the
   existing violations; the lint is cross-package so a one-sided
   commit breaks CI for everyone.
