# Forced-Read Remediation — 4-Layer Plan

**Status:** in-flight (2026-05-10).
**Baseline:** `0513549` (post A+B qualified-name + emit-cited primary union).
**Owner:** session forensic from 2026-05-10 customer feedback + s1a deep-dive.
**Eval gate:** s1a single PASS (already verified post-A+B); 30+ case sweep PASS rate must not regress.

---

## 1. Why this exists

### 1.1 Customer-reported phenomenon

The model complained in its `<think>` blocks about the system forcing it to read 3 unrelated files multiple times even after it had already decided those files were irrelevant. Reproduced in s1a (commit `0abf6c7` baseline, before today's A+B fix):

- **Question:** "gate.Run 的 9 项检查是按什么顺序跑的？遇到 retryable reject 时后续检查是否继续执行？"
- **Files force-injected by system 2x:** `internal/analysis/logtriage/bug_class_registry.go`, `internal/types/analysis_ir.go`, `internal/types/violation_registry.go`
- **Real subject file:** `internal/analysis/gate/gate.go`
- **LLM `<think>` (verbatim):** "The most relevant file appears to be `internal/analysis/gate/gate.go`. Let me search more specifically..."
- **Outcome:** LLM ignored the force-read list, read gate.go directly, but then the evidence pool the finalizer saw was still dominated by `IsValid` noise from the force-injected `analysis_ir.go`. Hallucinated 5 of 9 check names. FAIL.

### 1.2 Three coupled root causes

The s1a forensic identified **three failure layers stacked on top of each other**:

| # | Layer | Symptom | Root in code |
|---|-------|---------|--------------|
| α | **Coarse classification** of `enumeration` kind | Any "list 9 things" question routed to declarative-registration path | `declarativeFocusRelevant` in `internal/agent/explorer.go:1252` — collapses three semantically distinct enumeration types into one boolean |
| β | **System-injected pre-read with no dedup** | Same file content re-injected on every dispatch's prompt header | `preReadRequiredFiles` (5 callsites, lines 2253/2382/2499/2618/2662 in `internal/agent/explorer.go`) has no `excludeReadSet` parameter |
| γ | **Mid-loop force-read hint ignores LLM judgment** | "Read these next: A, B, C" hint pushes files even after LLM has read+decided-irrelevant | `observeMidLoop` at lines 6470–6502 uses `coverageScopeFiles` which doesn't filter on prior-emit-evidence signal |

Layers A+B (qualified-name resolver + emit-cited primary union, shipped today as `91ab4de` + `0513549`) cured the *symptom* for s1a — they let `gate.go` win the primary-files race, which made the declarative-path mis-trigger benign (the filter still kept gate.go's evidence). But α/β/γ remain latent across other cases. This document specifies the structural fix.

### 1.3 Prescan budget concern

Independently observed: **65%** (13/20) of recent eval cases hit `Pre-scan budget reached (3 of 3 rounds used)` and were force-stopped before the analyzer could finish entity disambiguation. Default `MaxPrescanRounds=2` is too tight; the multi-topic heuristic (`+1` for ≥2 sub-topics, capped at base+2 with `PrescanRoundsCeil=4`) helps but isn't enough for the 80th-percentile case.

---

## 2. Existing infrastructure inventory

(from 4 parallel code audits run 2026-05-10; full transcripts in session log.)

### 2.1 Declarative-path code (audit #1)

| Function | File:Line | Role |
|----------|-----------|------|
| `declarativeFocusRelevant` | `explorer.go:1252` | Soft trigger; activates on `enumeration && isEnumeration` OR `registration` OR `config_mapping` |
| `shouldStartDeclarativeDepth` | `explorer.go:1970` | Phase routing; gates on `kindConfidenceFloorForNarrowing = 0.7` |
| `shouldStartDeclarativeCandidateDepth` | `explorer.go:1991` | Fallback when no anchor files |
| `declarativeAnchorFilesFromPaths` | `explorer.go:1293` | 4-tier collect; uses `declarative.Kind` whitelist |
| `structuralCandidateFilesFromPaths` | `explorer.go:1353` | Up to 4 candidate paths |
| `buildDeclarativeFocusedStartInstruction` | `explorer.go:2513` | Calls `preReadRequiredFiles(..., 2, 220)` |
| `buildDeclarativeCandidateStartInstruction` | `explorer.go:2632` | Calls `preReadRequiredFiles(..., 2, 220)` |
| `declarative.KindRegistry/.KindManifest/...` | `internal/analysis/declarative/` | Existing typed enum we can reuse |

**Reuse opportunity:** the `declarative` package already detects whether a path "looks like" a registry / route / manifest etc. We do NOT need to build a parallel detector — we can reuse it to gate the trigger.

### 2.2 emit_analysis schema + AnalyzerHints (audit #2)

| Aspect | Where | Note |
|--------|-------|------|
| Schema definition | `internal/tool/emit_analysis.go:159–309` (`buildEmitAnalysisSchema`) | Required fields: intent, scenario, complexity, keywords, entities, question_kind + 3 confidences + predicates |
| **`required_files` is NOT in schema today** | post-emit derived in `analyzer.go:1291` from entity→file resolver | This means adding it is purely additive; no migration needed |
| AnalyzerHints struct | `internal/types/analysis_ir.go:396–462` | 11 fields, no per-item confidence/rationale |
| Confidence convention | `RequestModel.{Intent,Complexity,Kind}Confidence` ∈ [0,1] | Threshold gating example: `kindConfidenceFloorForNarrowing = 0.7` |
| Retry hint composer | `analyzer.go:247–323` | Currently echoes coherence violations only |
| Skill prompt for analyzer | `internal/skill/analysis_contract.go` | AnalysisEnumChoice tables |

**Reuse opportunity:** the existing 3-confidence pattern (intent/complexity/kind) is a working precedent for per-field confidence. We can mirror it for per-file confidence.

### 2.3 Prescan budget (audit #3)

| Item | Value | Source |
|------|-------|--------|
| Default `MaxPrescanRounds` | **2** | `internal/tool/analysis_limits.go:354` |
| Multi-topic bump | `+1` per 2 estimated sub-topics, hard cap at base+2 | `analyzer.go:78–109` |
| Hard ceiling (`PrescanRoundsCeil`) | **4** | yaml: `agent_prescan_rounds_ceil` |
| Grace round hint | "Pre-scan budget reached (N of M rounds used)" | `analyzer.go:876–888` |
| Hard stop | force-emit zero-value model | `analyzer.go:893–898` |
| CLI override | **none** — yaml only | `cmd/root.go` lacks `--max-prescan-rounds` |

### 2.4 Pre-read injection state (audit #4)

| Field | Persistence | Location |
|-------|-------------|----------|
| `e.structuredEvidence` | per-dispatch (reset at entry, `explorer.go:368`) | reset on each `BuildInitialInstruction` |
| `e.investigationNotes` | **cross-dispatch within Run** | survives retries, accumulated |
| `e.preScannedFiles` | top-8 from keyword search; narrowed across dispatches | populated at line 928 |
| `e.allScoredFiles` | full ranked list; **persistent** across dispatches | populated once |
| `EvidenceClosure.ReadSet()` | **cross-dispatch within Run** (cumulative read tracking) | `Mutable.EvidenceClosure().SetReadSet` |
| `EvidenceClosure.ScannedSet()` | **cross-dispatch** | `Mutable.EvidenceClosure().SetScannedSet` |
| `preReadRequiredFiles` callsites | **5 sites, no dedup logic today** | `explorer.go:2253/2382/2499/2618/2662` |

**Reuse opportunity:** `EvidenceClosure.ReadSet()` is already cumulative across dispatches. We do NOT need a new per-explorer dedup field — we just thread the existing closure through the pre-read helper.

---

## 3. The 4-layer plan + budget tweak

| Layer | Change | Coupled fixes A+B status | Files touched |
|-------|--------|--------------------------|---------------|
| **L1** | Tighten declarative-path trigger via PrimaryEntities-look-like-registration check | A+B alone made the path benign for s1a; L1 prevents future regressions across question shapes | 1 file (`explorer.go`) + 1 test file |
| **L2** | Suppress repeated pre-read: dedup against `EvidenceClosure.ReadSet()` | Defense in depth | 1 file (`explorer.go`) + 5 callsite updates + 1 test file |
| **L3** | Analyzer LLM emits `required_files` with confidence + rationale; explorer respects threshold | New typed channel — full 6-spot sync required | 5+ files; struct + schema + skill + decoder + retry hint + cooccurrence |
| **L4** | Analyzer LLM emits `irrelevant_files` (negative channel); explorer never re-injects them | New typed channel — full 6-spot sync required | 5+ files |
| **B0** | `MaxPrescanRounds` default 2 → 3, with `--max-prescan-rounds` CLI flag | Independent, tiny | 3 files |

Order of implementation: **B0 → L1 → L2 → L3 → L4**. Each layer commits independently, eval-tested before the next starts.

---

## 4. Per-layer implementation detail

### 4.1 B0 — Prescan budget bump (3 commits)

**Goal:** raise the default ceiling to match the 80th-percentile case observed in eval.

**Changes:**

1. **`internal/tool/analysis_limits.go:354`** — change default:
   ```go
   // before
   MaxPrescanRounds: 2,
   // after
   MaxPrescanRounds: 3,
   ```
2. **`internal/agent/analyzer.go:84–107`** — keep the existing multi-topic bump heuristic but adjust the cap:
   ```go
   // change `cap := base + 2` to `cap := base + 1` (because base is now higher)
   // PrescanRoundsCeil stays at 4 by default
   ```
3. **`cmd/root.go`** — add `--max-prescan-rounds` CLI flag (precedence: code default → yaml → CLI):
   ```go
   rootCmd.PersistentFlags().IntVar(&cliMaxPrescanRounds, "max-prescan-rounds", 0,
       "Override codrax.yaml :: analysis_max_prescan_rounds (0 = use yaml/default)")
   ```
4. **`codrax.yaml`** — uncomment example with new default in description.

**Rationale (why this doesn't bloat cost):**
- Median analyzer dispatch in current eval uses 1.5 rounds (excluding budget-exhausted ones).
- Bumping default to 3 is one extra LLM call only on the ~35% of cases that used >2 rounds.
- For multi-topic (≥2 sub-topics), bump becomes 3+1=4, matching `PrescanRoundsCeil`. No further inflation.

**Test:**
- New test in `analyzer_test.go`: assert `MaxPrescanRounds=3` default, `+1` bump for 2 sub-topics caps at 4.

**Commit boundary:** 1 commit; no API change.

---

### 4.2 L1 — Tighten declarative-path trigger (2 commits)

**Goal:** prevent declarative-registration path from firing on enumeration questions whose primary entities don't actually live in registry/route/manifest files.

**Design:** add a `primaryEntitiesLookLikeRegistration(ir, graph) bool` helper. Hook it into `declarativeFocusRelevant` so the enumeration-kind branch requires it.

**Reuse:** the `declarative` package already has typed `Kind` detection (`KindRegistry`, `KindRoutes`, `KindManifest`, `KindWire`, `KindTopology`). We use the existing detector — no new heuristic.

**Changes:**

1. **`internal/agent/explorer.go`** — new helper:
   ```go
   // primaryEntitiesLookLikeRegistration returns true when at least
   // one of ir.AnalyzerHints.PrimaryEntities resolves (via the
   // multi-language qualified-name resolver from B fix) to a file
   // that the declarative package classifies as a registry / route /
   // manifest / wire / topology surface. Returns false when no
   // primary entity resolves OR no resolved file is registration-shaped.
   //
   // Cross-language: uses forEachMatchingDef → applies to all 12+
   // languages codrax's repomap supports (the dotted/scoped name
   // resolution is shared with primaryEntityFiles).
   func primaryEntitiesLookLikeRegistration(
       ir *types.AnalysisIR,
       graph *repomap.Graph,
       allowedKinds map[declarative.Kind]bool,
   ) bool {
       if ir == nil || graph == nil || len(ir.AnalyzerHints.PrimaryEntities) == 0 {
           return false
       }
       entities := make(map[string]string, len(ir.AnalyzerHints.PrimaryEntities))
       for _, e := range ir.AnalyzerHints.PrimaryEntities {
           entities[strings.ToLower(e)] = e
       }
       found := false
       forEachMatchingDef(entities, graph, func(_, _, _ string, d *repomap.Symbol) bool {
           if d == nil || d.File == "" {
               return true
           }
           kind := declarative.ClassifyPath(d.File)  // existing API
           if allowedKinds[kind] {
               found = true
               return false  // short-circuit
           }
           return true
       })
       return found
   }
   ```

2. **`internal/agent/explorer.go:1252`** — modify `declarativeFocusRelevant` to take the IR + graph:
   ```go
   func declarativeFocusRelevant(
       questionKind string,
       isEnumeration bool,
       axis types.PredicateAxis,
       ir *types.AnalysisIR,
       graph *repomap.Graph,
   ) bool {
       switch strings.ToLower(strings.TrimSpace(questionKind)) {
       case "registration", "config_mapping":
           return true
       case "enumeration":
           if !isEnumeration && axis != types.AxisRegister {
               return false
           }
           // NEW: even when isEnumeration=true, require that the
           // primary entities structurally land in a declarative
           // surface. This filters out function-body enumerations
           // (s1a: "list the 9 internal checks of gate.Run") from
           // the registry-enumeration heuristic, while still firing
           // on real registry questions ("list all ViolationKind").
           allowed := declarativeAllowedKinds(questionKind, axis)
           return primaryEntitiesLookLikeRegistration(ir, graph, allowed)
       }
       return isEnumeration && axis == types.AxisRegister
   }
   ```

3. **Update all 5 callsites** to pass `ir, graph` (small mechanical diff).

**Tests:** `internal/agent/declarative_trigger_test.go` (new):
- `TestDeclarativeFocusRelevant_FunctionBodyEnumeration_NoTrigger` — s1a regression pin.
- `TestDeclarativeFocusRelevant_RegistryEnumeration_StillFires` — "list all ViolationKind" should still fire.
- `TestDeclarativeFocusRelevant_RegistrationKind_StillFires` — explicit registration kind unchanged.
- `TestDeclarativeFocusRelevant_NilGraph_FailOpen` — nil graph returns the legacy boolean.
- `TestPrimaryEntitiesLookLikeRegistration_*` — 4 cases (Go pkg-qualified, Java class-qualified, Rust namespaced, no-primary).

**Commit boundary:** 1 commit (helper + trigger + 5 callsite updates + tests).

**Risk:** when a real registry question uses a non-registration entity name, L1 might block the path. **Mitigation:** the existing `shouldStartDeclarativeCandidateDepth` falls back to required-files if no anchors found — that path stays open via the `axis == AxisRegister` branch.

---

### 4.3 L2 — Suppress repeated pre-read (1 commit)

**Goal:** stop force-injecting `Pre-read File Content` for files the LLM has already read in any prior dispatch within the Run.

**Reuse:** `EvidenceClosure.ReadSet()` is already cross-dispatch persistent. No new tracking needed.

**Changes:**

1. **`internal/agent/explorer.go:13115`** — extend `preReadRequiredFiles`:
   ```go
   // preReadRequiredFiles formats the first `maxFiles` files into
   // markdown for prompt injection. New (2026-05-10): when
   // `excludeRead` is non-nil, files whose canonical path is in
   // that set are skipped — the LLM has already seen them and
   // re-injecting wastes tokens AND signals "this is important"
   // for files the LLM may have judged irrelevant. Suppression is
   // per-dispatch silent (no emit log) since the closure's ReadSet
   // is already authoritative for the LLM's prior actions.
   func preReadRequiredFiles(
       repoRoot string,
       files []string,
       maxFiles, maxLines int,
       excludeRead map[string]bool,  // NEW
   ) string {
       // ... existing body, plus inside the loop:
       if excludeRead != nil {
           if excludeRead[canonicalExplorerPath(f)] {
               continue
           }
       }
       // ... rest unchanged.
   }
   ```

2. **All 5 callsites** pass `ctx.Mutable.EvidenceClosure().ReadSet()`:
   ```go
   excludeRead := map[string]bool{}
   if ctx != nil && ctx.Mutable != nil {
       for f := range ctx.Mutable.EvidenceClosure().ReadSet() {
           excludeRead[canonicalExplorerPath(f)] = true
       }
   }
   pre := preReadRequiredFiles(ctx.RepoRoot, files, 2, 220, excludeRead)
   ```

3. **`observeMidLoop` at `explorer.go:6470–6502`** — extend `coverageSnapshot` filter to also subtract files that are in ReadSet but have NO emit_evidence backing (LLM read + chose not to evidence = "judged irrelevant"). Skip them in mid-loop "Read these next:" hint.

   ```go
   // Filter unread to remove files the LLM has already evaluated
   // and chose not to emit_evidence on (read but no evidence in
   // structuredEvidence). Re-pushing them via mid-loop hint
   // contradicts the LLM's own judgment.
   citedSources := make(map[string]bool, len(e.structuredEvidence))
   for _, ev := range e.structuredEvidence {
       if ev.Producer == tool.EmitEvidenceProducer && ev.Source != "" {
           citedSources[canonicalEvidenceSourcePath(ev.Source)] = true
       }
   }
   filteredUnread := make([]string, 0, len(unread))
   for _, f := range unread {
       cf := canonicalExplorerPath(f)
       if readSet[cf] && !citedSources[cf] {
           // LLM saw it, decided not to cite. Don't re-push.
           continue
       }
       filteredUnread = append(filteredUnread, f)
   }
   ```

**Tests:** `internal/agent/preread_dedup_test.go` (new):
- `TestPreReadRequiredFiles_ExcludesReadFiles` — direct unit test on the helper.
- `TestPreReadRequiredFiles_NilExclude_LegacyBehavior` — backward-compat.
- `TestObserveMidLoop_SkipsLLMJudgedIrrelevantFiles` — integration; explorer with prior read but no emit should not get those files back in the hint.

**Commit boundary:** 1 commit.

**Risk:** if the LLM read the right file, didn't emit (e.g. analysis still in progress), L2 would withhold a re-push that would have helped. **Mitigation:** the suppression only kicks in for files in `ReadSet` (read at least once). The LLM still has the read content in its context history — it just won't get a fresh forced injection. If it needs to re-read, it can call `read_file` itself.

---

### 4.4 L3 — Analyzer-emitted required_files with confidence (4 commits)

**Goal:** transfer the "which files are most relevant" judgment from the post-emit heuristic to the analyzer LLM, with confidence-gated downstream consumption.

**Why now:** post-emit entity→file resolver gets dotted/scoped names wrong (B fix mitigates but isn't perfect; e.g. when entity wasn't even in graph). The LLM has full context (user question + repo summary + pre-scan results) — it should judge.

**6-spot sync (per `feedback_typed_signal_six_spot_sync.md`):**

| Spot | Where | Change |
|------|-------|--------|
| 1. Struct definition | `internal/types/analysis_ir.go:396–462` | Add `RequiredFileHints []RequiredFileHint` to AnalyzerHints; new struct `RequiredFileHint{Path string; Confidence float64; Rationale string}` |
| 2. Schema description | `internal/tool/emit_analysis.go:buildEmitAnalysisSchema` | Add `required_files` array property with item schema {path, confidence, rationale} + LLM-facing description |
| 3. Skill prompt | `internal/skill/analysis_contract.go` | Add new section: "When you can identify specific files structurally needed for the answer, emit them with confidence. Confidence ≥ 0.8 → file will be pre-read injected; 0.5–0.79 → soft hint only; <0.5 → ignored." |
| 4. Retry hint composer | `internal/agent/analyzer.go:247–323` (`composeCoherenceRetryHint`) | On retry, echo any low-confidence required_files back: "Your prior emit listed `X` with confidence 0.4 — either confirm with rationale or drop it." |
| 5. JSON decoder error remap | `internal/tool/emit_analysis.go:Execute` | Validate per-item: confidence ∈ [0,1], path non-empty, rationale ≥ 10 chars. Reject with structured error. |
| 6. Cooccurrence rule / RepairLocus | `internal/orchestrator/repair_cooccurrence.go` | Add rule: when contract.Check fails AND no required_files emitted AND question is mechanism kind → suggest "analyzer should emit required_files for the primary file" |

**Consumer (explorer):**

In `explorer.go`, add helper `analyzerRequiredFilesWithConfidence() (high, medium []string)`:
- High: confidence ≥ 0.8 → fed into `effectivePrimaryFiles` AND eligible for pre-read injection
- Medium: 0.5–0.79 → eligible for pre-read (lower priority slot) but NOT for primary-file filter
- Low: <0.5 → discarded

The threshold floor (0.8) mirrors the established `kindConfidenceFloorForNarrowing = 0.7` pattern — slight bump because per-file judgments are easier for the LLM than meta-classification.

**Backwards compatibility:**

The new `required_files` field is **optional** in the schema. Legacy emit_analysis output without it falls through to the existing post-emit entity→file resolver. After 30+ case eval shows the LLM populates the field reliably, we can promote it to required.

**Tests:**
- `internal/types/analysis_ir_test.go` — struct shape + JSON roundtrip
- `internal/tool/emit_analysis_test.go` — schema + decoder validation
- `internal/agent/analyzer_required_files_test.go` — integration test with synthetic LLM emit
- `internal/orchestrator/repair_cooccurrence_test.go` — cooccurrence rule fires correctly

**Commit boundary:** 4 commits, sequenced:
1. **L3-T1**: types + struct definition
2. **L3-T2**: schema + decoder + skill prompt
3. **L3-T3**: explorer consumer + threshold gating
4. **L3-T4**: retry hint + cooccurrence rule

**Risk:** LLM might emit wrong `required_files` and we override the resolver. **Mitigation:** confidence gating + the resolver still runs as a fallback when `required_files` is empty.

---

### 4.5 L4 — Analyzer-emitted irrelevant_files negative channel (3 commits)

**Goal:** allow the LLM to explicitly tell downstream pipeline "do NOT inject these files into pre-reads or mid-loop hints, regardless of the resolver's opinion."

**Why:** even with L1 trigger tightening + L2 dedup, the resolver might still derive irrelevant files from over-broad RequiredFiles. The LLM has the most context to judge — let it speak.

**6-spot sync:**

| Spot | Where | Change |
|------|-------|--------|
| 1. Struct definition | `internal/types/analysis_ir.go` | Add `IrrelevantFiles []string` to AnalyzerHints |
| 2. Schema description | `internal/tool/emit_analysis.go` | Add `irrelevant_files` array of strings + LLM-facing description |
| 3. Skill prompt | `internal/skill/analysis_contract.go` | "When you have read a candidate file in pre-scan and judged it OFF-TOPIC for the user's question, list its path here. Downstream agents will not re-inject these files." |
| 4. Retry hint composer | `analyzer.go` | If retry happens and LLM previously declared irrelevant_files, remind it: "Your prior emit declared `X` irrelevant — sticking with that decision unless contradicted by new evidence." |
| 5. JSON decoder error remap | `emit_analysis.go` | Validate non-empty paths; cap at 10 entries (prevent abuse); paths must canonical via `canonicalExplorerPath`. |
| 6. Cooccurrence rule / RepairLocus | `repair_cooccurrence.go` | When extractor's hypothesis verdict is `inconclusive` AND `evidence_files ∩ readSet = ∅`, increment a counter; if it exceeds threshold across runs, suggest analyzer learn to declare irrelevant_files better. |

**Consumer:**

In `explorer.go`:
- `preReadRequiredFiles` excludes paths in `IrrelevantFiles` (in addition to `ReadSet`).
- `effectivePrimaryFiles` excludes paths in `IrrelevantFiles`.
- `observeMidLoop` "Read these next:" excludes paths in `IrrelevantFiles`.
- `coverageScopeFiles` excludes paths in `IrrelevantFiles`.

**Tests:**
- Struct + JSON roundtrip
- Schema validation
- Explorer integration: irrelevant_files honored across all 4 consumer sites

**Commit boundary:** 3 commits:
1. **L4-T1**: types + schema + decoder
2. **L4-T2**: explorer consumer (4 sites)
3. **L4-T3**: retry hint + cooccurrence rule

**Risk:** LLM might declare a file irrelevant that's actually critical. **Mitigation:** the field is optional and additive — empty IrrelevantFiles falls through to existing behavior. If the explorer LATER cites a file in emit_evidence that was declared irrelevant by analyzer, we log a warning (`[explorer] WARN: cited file %q was declared irrelevant by analyzer`).

---

## 5. Multi-language coverage matrix

Per `feedback_generalization_over_project_success.md` red line, every change must work for all 12+ languages. Per-layer matrix:

| Layer | Multi-language footprint | How |
|-------|-------------------------|-----|
| B0 | None — pure config bump | n/a |
| L1 | Reuses A+B's `forEachMatchingDef` (already multi-language) + reuses existing `declarative.ClassifyPath` (already multi-language via path patterns) | No new language-specific code |
| L2 | Reuses `EvidenceClosure.ReadSet()` (path-based, language-agnostic) | No new language-specific code |
| L3 | LLM-emitted file paths are POSIX-canonical (existing `canonicalExplorerPath` handles Windows backslashes) | Schema description must NOT bias toward any language; rationale guidance is generic |
| L4 | Same as L3 | Same |

All changes are language-agnostic at the data-flow level. Any future-added language inherits the fix automatically.

---

## 6. Test plan

### 6.1 Unit tests

| Layer | Test files | Cases |
|-------|-----------|-------|
| B0 | `analysis_limits_test.go` (existing, extend) | 1 new — default value |
| L1 | `declarative_trigger_test.go` (new) | 7 cases including s1a regression |
| L2 | `preread_dedup_test.go` (new) | 5 cases |
| L3 | `analyzer_required_files_test.go` (new) + extend struct/schema/repair tests | 12+ cases |
| L4 | `analyzer_irrelevant_files_test.go` (new) + extend struct/schema/repair tests | 8+ cases |

**Total:** ~35 new tests.

### 6.2 Integration eval

After each layer ships:
- s1a single PASS (must stay)
- 5 hand-picked cases that previously hit declarative-path mis-trigger or budget exhaustion (s5b, s11a, qf_imports, qf_architecture, qf_config_precedence)
- After L4: 30+ case sweep; pass rate must not regress below baseline (97.5% post A+B)

### 6.3 Acceptance criteria

| Criterion | Target |
|-----------|--------|
| s1a PASS | Single-run repeatable PASS |
| 30+ case sweep PASS rate | ≥ 97.5% (no regression) |
| Median analyzer dispatches per case | -10% (less budget exhaustion) |
| Forced pre-read injections per case (median) | -50% (L2 dedup) |
| Mid-loop "Read these next:" with already-judged-irrelevant files | 0 occurrences |

---

## 7. Implementation order + commit map

| Order | Layer | Commits | LOC est. | Eval gate |
|-------|-------|---------|----------|-----------|
| 1 | B0 | 1 | ~30 | s1a single PASS |
| 2 | L1 | 1 | ~150 | s1a + 3 hand-picked cases |
| 3 | L2 | 1 | ~120 | s1a + check pre-read counts dropped |
| 4 | L3-T1 | 1 | ~80 | unit only |
| 5 | L3-T2 | 1 | ~150 | unit + s1a + 1 LLM real-eval |
| 6 | L3-T3 | 1 | ~80 | s1a + 3 cases |
| 7 | L3-T4 | 1 | ~60 | unit only |
| 8 | L4-T1 | 1 | ~100 | unit only |
| 9 | L4-T2 | 1 | ~80 | s1a |
| 10 | L4-T3 | 1 | ~50 | unit |
| 11 | Final sweep | — | — | 30+ case eval, eval/parallel_all.sh PARALLEL=4 |

**Total:** ~10 commits, ~900 LOC, 1 session.

---

## 8. Red-line audit (per `feedback_prompt_redline_checklist.md`)

For every prompt-string change in this plan:

| Rule | L1 | L2 | L3 | L4 |
|------|----|----|----|----|
| **R3** Precise signals for hard gates / noisy for soft | ✅ confidence threshold = SOFT routing only; hard gate stays on existing chain | ✅ ReadSet is precise binary | ✅ confidence is per-item soft signal | ✅ irrelevant_files is precise per-path |
| **R4** No over-fitting | ✅ uses existing `declarative.ClassifyPath` (data-driven, no hardcoded names) | ✅ uses existing closure | ✅ schema is generic | ✅ schema is generic |
| **R5** Skill prompts: structural over per-question | n/a (no skill prompt change) | n/a | ✅ guidance is generic ("when you can identify…") | ✅ generic |
| **R6** No internal pipeline terms in LLM-facing text | ✅ no LLM-facing change | ✅ no LLM-facing change | ✅ "files structurally needed", not "RequiredFiles" | ✅ "OFF-TOPIC", not "irrelevant_files field" |
| **R7** mermaid library-subset: n/a | n/a | n/a | n/a | n/a |
| **SST** Skill section title vocabulary | n/a | n/a | ✅ generated via SST helper | ✅ same |
| **R2'** 6-spot sync | n/a (no new typed signal) | n/a (no new typed signal) | ✅ all 6 spots | ✅ all 6 spots |
| **CN+EN-only prompts** (per 2026-05-10 user red line) | n/a | n/a | ✅ skill prompt CN/EN only | ✅ same |

---

## 9. Risk + rollback

| Layer | Worst-case regression | Rollback |
|-------|----------------------|----------|
| B0 | One extra LLM call on simple cases | revert single commit |
| L1 | Real registry questions blocked | `axis == AxisRegister` branch + candidate-fallback path stay open; revert the trigger change leaves rest intact |
| L2 | LLM needs a file again, has to re-call read_file | mild latency hit; not incorrect; revert single commit |
| L3 | LLM emits bad required_files | confidence threshold + resolver fallback; revert L3 commits leaves L1+L2 intact |
| L4 | LLM declares wrong file irrelevant | warning on contradictory cite + revert single commit; field is optional throughout |

Each layer is **independently revertable**. A+B already shipped don't depend on L1-L4.

---

## 10. Out of scope

- **Explorer-side `not_relevant_files` self-emit** — would require new tool + tool result handling. Deferred to future session as it requires more design.
- **Reviewer-stage forced-read suppression** — finalizer doesn't currently force-read; non-issue.
- **Cross-Run learning of irrelevant files** — Failure Taxonomy already records cross-Run patterns; manual hookup deferred.

---

**Status next:** B0 → L1 → L2 → L3 → L4 implementation begins. Each layer ships + verified before next starts.
