# Bridge-literal evidence extraction gap

**Status**: open, high priority
**Discovered**: 2026-04-13, end of Phase 4 session
**Blocks**: df1 / df3 correctness on runs where the analyzer takes a
non-canonical rewrite path
**Related commit**: `7bbc93f` (Phase 4 — answer-shape-gated literal
promotion)
**Related memory**: `memory/project_bridge_literal_extraction_gap.md`

## TL;DR

Phase 4 fixes the *selection* half of L0-2 — when the strict evidence
subset contains a bridge-literal chain (`A binds NewB → B.Name()
returns "x"`), Phase 4 ensures the classifier routes through
RoleAnchor and extracts `"x"`. **But Phase 4 cannot help when the
strict subset never contained the chain in the first place.**

The REPL 2-turn verification at the end of the Phase 4 session
surfaced exactly this case. When the analyzer rewrote `"which agents
can invoke subagent"` into a Chinese title, the explorer's
LLM-driven investigation-note parser produced no recognizable
`[REGISTRATION]` tag, and the source-code-driven
`extractConcreteValues` pass produced per-file short methods without
a cross-file *join*. Neither path emitted the bridge chain. L0-2
never ran. The finalizer fell through to the explorer's free-form
synthesis prose and returned a wrong answer.

This is an **upstream** gap, structurally independent of Phase 4.

## Observed failure

Log location (temporary, may age out): `/tmp/codrax-phase4-repl-1776049372/logs/`

Turn 1 trace:

```
INFO  [orchestrator] starting pipeline: trace=trace-1776049372431533711
DEBUG [diag analyzer] iter=0 call[0] tool=todo_write params=
    {"answer_shape":"list_of_symbols",
     "question_kind":"enumeration",
     "title":"查找可以调用子代理的代理",
     "description":"Identify which agents have the capability to invoke subagents.",
     "entities":[]}
DEBUG [orchestrator] task=7c87…b958 step=0 stage=explore missing=facts
DEBUG [explorer] cross-run reset: current="查找可以调用子代理的代理" != cached=""
DEBUG [erm] enumeration(which,agents,invoke,subagent) = unsatisfied — analyzer declared question_kind=enumeration
… (explorer investigates, produces investigation notes in Chinese) …
DEBUG [erm] enumeration(which,agents,invoke,subagent) = satisfied — analyzer declared question_kind=enumeration
INFO  [orchestrator] task 7c87…b958 reached terminal stage finalize
INFO  [repl] final answer:
  • SubExplorer
  • SubAgentRuntime
  • ProposeSubAgents
  • internal/agent/subagent.go:63 — RegisterDefaultSubAgents 注册 SubExplorer。
  …
```

Notice what is **absent**:

- No `identified N answer chains (M strict)` log line
- No `L0-2 extracted N answer symbols` line
- No `answer_symbol[0]: …` lines
- No `Phase 4 gate: …` promotion

L0-2 never fired for turn 1. The finalizer built its answer from the
explorer's free-form synthesis. Correct answer is `explorer` (the
string literal returned by `SubExplorer.Name()`); the finalizer
returned three Go type names.

## Root cause

Bridge-literal extraction currently runs through two parallel paths,
neither of which delivers the `RegisterDefaultSubAgents → NewSubExplorer → SubExplorer.Name() → "explorer"` chain deterministically.

### Path 1: LLM investigation notes (`evidence.go:parseEvidenceItems`)

The explorer's ReAct loop collects assistant content messages into
`investigationNotes`. A post-loop parse scans them for lines tagged
`[DIRECT]`, `[CONDITIONAL]`, `[REGISTRATION]`, `[MECHANISM]`,
`[RELATIONSHIP]`. Each recognized line becomes an `EvidenceItem` with
`Producer="explorer.llm"`.

Failure mode: the LLM must CHOOSE to write the bridge chain in the
recognized tag format. When the task title is Chinese, the LLM's
notes are also Chinese and often skip the English tag markers, or
use a structure that doesn't produce the expected 2-hop chain. The
extraction is **LLM-variance dependent**.

### Path 2: Source-code concrete values (`explorer.go:extractConcreteValues`)

A deterministic pattern scanner runs over files in
`phase0.scoredFiles + readSet`. It finds short methods and
registrations and emits `EvidenceItem`s with
`Producer="concrete_values"`. Key shapes it recognizes:

- `return "literal"` / `return 'literal'`
- Inline `func() { return X }`
- Arrow functions `() => "x"`
- `Register(NewFoo())` constructor-passing
- Map literal `AgentExplorer: NewExplorerAgent`
- Decorators `@route("/path")`
- YAML/JSON config leaf values

Failure mode: these are PER-FILE extractions. The bridge pattern
`RegisterX(NewS) + S.Name() returns "lit"` requires **cross-file
JOIN**: one fact lives in `registry.go` (the binding), the other
lives in `sub_explorer.go` (the `.Name()` method). No current pass
does this join when producing evidence for L0-1 consumption.

`erm.go:resolveConcreteValues` DOES do a cross-file join as a 5-pass
fixpoint on concrete-value symbol references, but it runs on the
ERM *resolution* side — it improves ERM status reporting and
synthesis-prompt facts, it does not back-propagate its join into new
`EvidenceItem`s feeding L0-1 / L0-2.

## Why this matters

The `A binds NewB → B.Name() returns "lit"` pattern is not niche —
it is the canonical Go (and Java, Python) registry idiom. Codrax
uses it for:

- `RegisterDefaultSubAgents → SubExplorer.Name() → "explorer"`
- `RegisterDefaults → [ExecCommand, GrepTool, ReadFile, …] → ToolName()` returns
- Any plugin system, any DI container, any handler registry

Any "which X has identity Y" question ultimately resolves to this
pattern. The current pipeline is structurally capable of answering
them (the facts are in the source code, the extractors parse them
individually, the finalizer understands the shape) but the PRODUCTION
of the joined chain is non-deterministic.

## Fix direction

**A new deterministic pass**, `extractBridgeLiteralChains`, added to
the same phase as `extractConcreteValues`, with the following
contract:

1. **Scan** all files in `phase0.scoredFiles + readSet + bridgeWalkSet`,
   where `bridgeWalkSet` is computed from step 2 below. This set
   explicitly reaches beyond the files the LLM chose to read.
2. **Pass A — binding collection**: for each `Register*` / `binds` /
   map-literal-registration / function-call-registration pattern
   in the scoped files, extract `(bindingFn, targetClass)` pairs.
   Strip `New` prefix from constructor calls to get `targetClass`.
   For every unique `targetClass`, locate its definition file via
   `repomap.Graph.SymbolDefs` and add to `bridgeWalkSet`.
3. **Pass B — identity-method scan**: in each file in
   `bridgeWalkSet`, scan for methods matching the regex
   `(Name|ID|Key|Type|Label|Slug|Kind)\(\)\s+(\w+\s+)?\{\s*return\s+"([^"]+)"`.
   Emit `(targetClass, methodName, literal)` triples.
4. **Pass C — join**: for each binding `(bindingFn, targetClass)`
   that has at least one identity-method `(targetClass, method,
   literal)`, emit an `EvidenceItem`:

   ```go
   types.EvidenceItem{
       Kind:      types.EvidenceConcrete,
       Subject:   bindingFn,
       Predicate: "binds ONLY",
       Object:    "New" + targetClass + "(...)",
       Summary:   fmt.Sprintf("`%s()` binds ONLY New%s(...) → `%s.%s()` returns %q",
                              bindingFn, targetClass, targetClass, method, literal),
       Source:    bindingFile,
       LineStart: bindingLine,
       Producer:  "bridge_literal",
       Confidence: 0.9,
   }
   ```

5. **Provenance**: `Producer="bridge_literal"` — distinct from
   `concrete_values` so we can audit which fix shipped it and so
   L0-1 ranking can optionally weight it.

The join key is the class identifier. No keyword matching, no
English/Chinese semantics, no NL involvement. Pure source-code AST
walk.

## Over-fit audit

1. **Reverse**: swap binding/returns direction → produces the correct
   reverse chain. Rule is symmetric under direction flip.
2. **Deletion**: remove any eval case from the dataset → rule still
   fires structurally for any register+returns-literal pair.
3. **Class specificity**: zero hardcoded symbol names. Rule is
   pattern `class C receives from a register-family caller` AND
   `class C has an identity method returning a string literal`.
4. **No-bait**: rule only fires when BOTH halves of the join exist.
   A registration without an identity method produces no bridge
   chain (legacy Terminal path handles it). An identity method
   without a registration also produces nothing (the ordinary
   `extractConcreteValues` pass already covers it).
5. **No-contamination**: no eval-specific logic, no per-framework
   hacks. Runs on any Go codebase (and trivially extends to
   Java/Python with a per-language method-name list).

Passes all five.

## Scope and risk

**Scope**:
- `internal/agent/explorer.go` — new `extractBridgeLiteralChains`
  pass, called from the same place as `extractConcreteValues`
- `internal/agent/explorer.go` — `bridgeWalkSet` computation (a
  small helper that walks binding targets via `repomap.Graph`)
- New tests: `internal/agent/bridge_literal_chain_test.go` with
  a Go fixture, a Python fixture, a Java fixture, and the 5
  over-fit cells
- df1 + df3 full eval grid for regression
- Updated memory: `project_bridge_literal_extraction_gap.md`
  marked shipped

**Risk**:
- The rule reads files outside the LLM's `readSet`. Need to cap
  `bridgeWalkSet` size to avoid scanning the whole repo on a bad
  grep match. Suggested cap: 20 additional files, keyed by
  `repomap.Graph.SymbolDefs` lookup so we only add files that
  *define* the target class (not arbitrary mentions).
- The concrete-value extractor produces a lot of noise today.
  Adding bridge chains on top may flood L0-1 if the binding
  pattern is overly broad (e.g. any `r.Register(X)` call matches).
  Mitigation: restrict Pass A to function names matching
  `^Register|^register.*Defaults?$|^init(Registry|Handlers)` or
  similar — but even that is a heuristic the audit needs to
  cover.
- Existing `resolveConcreteValues` does a similar multi-pass join
  on the ERM-resolution side. If the two diverge in behavior,
  users may see the ERM status claim "satisfied" while L0-2 still
  misses the chain. A cleaner architecture would have
  `extractBridgeLiteralChains` produce evidence, and
  `resolveConcreteValues` consume it — single source of truth.
  Estimate adds 1–2 hours for refactor.

**Estimate**: 4–8 hours plus a full eval grid run.

## Why this is orthogonal to Phase 4

Phase 4 fixes the *selection* layer: given a strict subset that
includes a bridge chain, ensure the classifier extracts the literal.
Proven correct by unit tests + 3 real single-shot runs (1 through
Phase 4 gate, 2 through existing F3 path).

This gap fixes the *production* layer: ensure the strict subset
CONTAINS the bridge chain on every run, regardless of which language
the analyzer rewrote the title into. When both layers are correct,
the answer is correct on every analyzer rewrite path.

The two are complementary and must both ship for the "which X has
identity Y" question class to be reliably answered.

## Queue position

This issue goes after #3 (entity cleanup — entity extraction
tightening) in the REPL-audit follow-up queue. Rationale: a cleaner
entity set from #3 may surface a better-ranked strict subset in
L0-1, which could reduce (though not eliminate) the frequency of
the bridge-literal miss. Running this fix before #3 risks measuring
against noisy L0-1 ranking.

Not a blocker for shipping — the current pipeline still answers
correctly on the English→English canonical rewrite path (Phase 4
handles it, as proven by single-shot runs). It fails specifically
on the English→Chinese rewrite path, which is infrequent but
reproducible.
