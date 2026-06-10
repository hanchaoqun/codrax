# Write Mode GitHub Issue Eval P0 Systemic Hardening

Date: 2026-06-10
Branch: main
Evidence ledger: `docs/design/write_mode_github_issue_eval_20260610.md` (four non-Go
GitHub-issue fixtures; trusted pass rate 1/4)

## 1. Objective

Close the P0/P1/P2 system gaps recorded by the 2026-06-10 GitHub-issue write eval
as generic mechanisms, not per-case patches. Each fix targets a *class* of
failures observed across C, C++, Java, and TypeScript fixtures simultaneously:

- verification keyed off language defaults instead of a typed test surface;
- controller state that can read as two phases at once and a `finish` action
  with no typed verification gate;
- failed attempts leaving no durable report / diff / command evidence;
- an unverified LLM boolean (`affects_public_api`) driving a hard approval gate;
- verify failure evidence not force-fed to replan, and a no-plan replan round
  aborting the whole workflow.

## 2. Verified code facts (audited 2026-06-10, this branch)

These were read from source, not assumed:

- `run_tests` runner choice: LLM `runner` param short-circuits manifest
  detection (`internal/tool/run_tests.go:266`); auto-detect ranks by manifest
  priority where `pom.xml`(12) outranks `Makefile`(18) regardless of which one
  actually has test work; the no-test-work check happens per-plan *after*
  selection (`run_tests.go:337`). `runTestsParams` is decoded with bare
  `json.Unmarshal` — it does NOT route through `applyStructuredPayloadCompat`.
- `ChangeReport.GeneratedAt` (`internal/types/change_plan.go:703`) is declared
  but never assigned anywhere in the codebase.
- Controller scheduler (`internal/orchestrator/write_controller_scheduler.go`):
  - `ActionFinish` (line 225) marks the run complete unconditionally — no typed
    check against the active batch's latest post-apply report.
  - verify failure sets batch `ready_to_plan` + progress reason `verify_failed`
    (lines 209-210); the controller prompt renders both
    (`internal/agent/write_controller.go:164,169`).
  - `prepareControllerPlanningState` (line 539) resets ChangeReport and
    IterationLedger before every controller plan round, so replan loses the
    typed failure unless it is carried elsewhere.
  - `runControllerPlanBatch` failure (line 109-115) returns the error and
    terminates the whole workflow run — a single no-plan replan round is fatal.
- `WriteWorkflowDecisionSchema()` (`internal/writeflow/decision.go:112`) is
  static; ModePlan blocking happens only after emission in the scheduler.
  Precedent for per-dispatch schema projection exists:
  `EmitAnswerDocument.ParametersFor(ctx)` wired at
  `internal/agent/agent.go:2853`.
- Risk gate (`internal/writeflow/risk.go:125-142`): the three WriteAnalysisIR
  booleans and `Overall=high` each contribute `RiskHigh` unconditionally;
  `RiskHigh` always forces `manual_approval` (`risk.go:160`). Path policy
  (build manifests, secrets, CI, hooks, `.git`, escapes) is already typed and
  deterministic and stays the hard backbone.
- Worktree lifecycle: discard is an unconditional outer defer
  (`internal/orchestrator/orchestrator.go:2050-2103`, red line L5). At the
  scheduler's verify-failure branch the worktree is still alive; applied bytes
  also survive in `refs/codrax/applied/<plan-id>`. No diff artifact is captured
  anywhere today.
- Structured edits (`internal/tool/structured_edit.go`): `replace`/`delete`
  with omitted `end_line` (0) fail as `invalid line range N-0`; `old_text`
  mismatch reports the line range but not the actual current bytes.
- Sibling eval workspace holds uncommitted artifacts to port: the evidence
  ledger, 4 `eval/cases/github_issue_*.case`, `eval/fixtures/github_issues/`
  (76K, 19 files), `Makefile` target `eval-github-issues`, and `eval/run.sh`
  apply-mode/worktree hardening.

## 3. Design principles (unchanged red lines)

- Hard gates read precise typed signals only: booleans, enums, ids,
  fingerprints, parsed structural data, store records. Noisy signals (LLM
  classification booleans, prose, scores) drive soft guidance only.
- Every new system-side hard gate provides a model-declarable typed escape
  lane (architecture red line §1.6).
- No keyword matching on user intent or model prose anywhere in routing.
- Model-emitted JSON payloads route through the unified repair layer
  (`applyStructuredPayloadCompat`) before strict decode.
- Read mode byte identity (L1), worktree cleanup unconditional (L5), write
  tool grounding ban (L3), and all repomap red lines stay untouched.
- Reuse existing mechanisms: runner detection, ChangeReport, WriteContextPack,
  attempt refs, `ParametersFor(ctx)` schema projection, applied-ref worktree
  evidence. No parallel workflow stack.

## 4. Architecture changes

### 4.1 Typed TestSurface (P0-1)

New typed artifact in `internal/types/test_surface.go`:

```go
type TestSurfaceCandidate struct {
    ID            string // stable: "<runner>[/<framework>]@<rel-dir>"
    Runner        string // existing whitelist value
    Framework     string // python refinement; empty otherwise
    WorkingDir    string // repo-relative; "." for root
    Command       string // canonical command template rendered for display
    Source        string // manifest path that justified this candidate
    MakeTarget    string // for runner=make: detected check/test target
    HasTestSignal bool   // typed: runnerHasNoTestWork == false
    Priority      int    // manifest priority (existing table)
}

type TestSurface struct {
    Candidates  []TestSurfaceCandidate
    SelectedID  string
    GeneratedAt time.Time
}
```

Builder `BuildTestSurface(repoRoot, walkRoot)` lives in `internal/tool` and is
a pure composition of existing detectors: `detectRunnerPlans`,
`runnerHasNoTestWork`, `detectMakeTestTarget`, `detectPythonTestFramework`,
`detectJavaBuildSystem`, `buildRunCommandForPlan` (render-only). No new
language knowledge is introduced.

Selection rule (deterministic): order by `HasTestSignal` desc, then existing
manifest priority asc, then depth, then path. This is the generalized fix for
both observed shapes: a `Makefile` with a runnable `check` target outranks a
`pom.xml` whose Maven tree has no test work, and outranks a Python
syntax-check-only path. The rule is "runnable test work dominates manifest
priority", not anything fixture-specific.

`run_tests` integration:

- LLM-supplied `runner` stays authoritative when it resolves (model freedom,
  existing escape lane).
- New deterministic escalation: when the merged report ends with
  `NoTestsRunners` non-empty AND the surface contains an unexecuted candidate
  with `HasTestSignal=true`, execute the top such candidate once and merge its
  report. Gate inputs are typed report fields and typed surface fields only.
- Auto-detect path (`runner` empty) selects through the same ordering.
- The verifier prompt renders the surface (candidates, commands, sources,
  signal booleans) as typed facts so the model's first choice is informed —
  soft guidance, same hard validation as today.
- Planner `dry_run=true` probes flow through the same selection automatically
  (P1-3 collapses into this batch).
- `run_tests` params route through `applyStructuredPayloadCompat` like other
  model-facing payloads.

ChangeReport gains typed execution evidence (also feeds P0-3 and P0-5):

```go
type ExecutedCommand struct {
    Runner     string
    Framework  string
    WorkingDir string // repo-relative
    Command    string
    ExitCode   int
    DurationMS int64
    Source     string // "selected_surface" | "llm_choice" | "auto_detect" | "no_tests_escalation"
}
// ChangeReport: ExecutedCommands []ExecutedCommand, TestSurfaceID string
```

`installRunTestsReport` stamps `GeneratedAt` (fixes the never-assigned field);
`saveChangeReport` backfills when zero.

### 4.2 Canonical verify attempt state + finish gate + ModePlan mask (P0-2, P1-2)

`internal/writeflow/attempt_state.go`:

```go
type BatchAttemptPhase string // "needs_exploration","ready_to_plan","planned",
                              // "pending_approval","applying","needs_replan",
                              // "verifying","complete","blocked"
type BatchAttemptState struct {
    Phase        BatchAttemptPhase
    Cause        string // typed reason code from the latest attempt, e.g. "tests_failed"
    PlanID       string
    ReportID     string
    VerifyFailed int    // count of failed post-apply verify attempts
}
func DeriveBatchAttemptState(batch types.WriteWorkflowBatch) BatchAttemptState
```

Derivation reads only `batch.Status` + `batch.Attempts` (typed refs). A batch
whose status is `ready_to_plan` with a latest failed verify attempt derives
`Phase=needs_replan, Cause=<attempt reason code>` — one canonical state. The
controller prompt renders the active batch through this derivation; the
progress ledger line is renamed to an events label so state and events cannot
be conflated.

`finish` hard gate in the scheduler: reject `ActionFinish` when the active
batch's latest post-apply verify attempt failed and the batch is not complete.
Escape lane (red line §1.6): a new typed decision field

```go
FinishDisposition string // "", "all_verified", "accept_unverified"
```

`finish` with `accept_unverified` completes the run and stamps a typed caveat
into the result; otherwise the rejection repair lists the typed options
(`replan_batch`, `split_batch`, `block`, or finish with the declared
disposition). The gate reads attempt records only — never prose.

ModePlan action mask: `EmitWriteWorkflowDecision.ParametersFor(ctx)` projects
the action enum by typed mode — ModePlan drops `apply_plan` and `verify_batch`
from the schema (mirrors `EmitAnswerDocument.ParametersFor`). Execute-time
validation rejects masked actions with a typed repair; the scheduler guard
stays as the third layer.

### 4.3 Durable failure evidence bundle (P0-3)

At the scheduler verify-failure branch (worktree still alive, before
`continue`):

- capture the applied-vs-base unified diff from the worktree
  (`git diff` against the worktree base; falls back to
  `refs/codrax/applied/<plan-id>` when the tree is already clean) and persist
  to the plan dir as `<stem>.attempt-<n>.diff`;
- persist the selected `TestSurface` as `<stem>.surface.json`;
- the ChangeReport already persists via `saveChangeReport`; the new
  `ExecutedCommands` rows make the command transcript durable, with full
  output already blobbed via `FailureSummaryBlobRef`;
- batch verify attempt `ArtifactRef` points at the diff artifact; the report
  ref stays on `ReportID`.

Cleanup stays unconditional (L5 untouched); evidence is captured before, never
by skipping discard. A controller-workflow regression test pins that a failed
verify leaves report JSON + diff + surface on disk.

### 4.4 Approval risk decomposition (P0-4)

`AssessWriteRisk` keeps its deterministic backbone (path policy, change kinds,
scale, secrets, `.git`/escape critical). The four LLM-derived contributions
are re-graded per the precise/noisy red line:

- `AffectsPublicAPI`, `ChangesPersistence`, `ChangesBuildSystem`,
  `Overall=high`: each contributes `RiskMedium` with reason code
  `<axis>_uncorroborated` — visible to the user and to prompts, auto-executable
  under `auto_safe`, but no longer a hard manual-approval gate by itself.
- New precise corroboration source: typed declaration-span intersection.

```go
// internal/types (interface), implemented by a repomap graph adapter:
type DeclSpanSource interface {
    // ExportedDeclSpans returns exported/public declaration line spans for a
    // repo-relative file known to the graph; ok=false when unindexed.
    ExportedDeclSpans(relPath string) (spans []LineSpan, ok bool)
}
```

`AssessmentInput` gains `Decls DeclSpanSource` (nil-safe). For `kind=patch`
changes the touched spans come from deterministic `@@ -a,b +c,d @@` hunk
header parsing and from structured-edit `start_line/end_line` ranges; an
intersection with an exported declaration span contributes `RiskHigh`
(`public_decl_line_changed`, with file/line detail). `delete` stays High;
`create`/full `modify` keep path-class risk. When the graph or file index is
unavailable, no decl contribution is made and the medium-grade LLM axes stand
— uncorroborated claims can no longer hard-block a body-only two-line literal
fix, while a patch that rewrites an exported signature line still requires
manual approval through a precise signal.

Build-system corroboration already exists as path policy
(`isBuildOrDependencyManifest` → High). Persistence corroboration is path
policy too: a small `isPersistenceSchemaPath` class (migration directories,
`.sql` files) joins the existing taxonomy.

The write-analyzer skill wording is adjusted (soft) to describe the booleans
as advisory classification corroborated by typed diff analysis.

### 4.5 Verify-failure handoff forced into replan (P0-5, P1-5)

New typed carrier that survives `prepareControllerPlanningState`:

```go
type VerifyFailureHandoff struct {
    PlanID, BatchID  string
    Attempt          int
    FailureKind      types.FailureKind
    Executed         []types.ExecutedCommand // from the failing report
    FailingTests     []types.TestResult      // bounded: failed rows only
    BuildErrors      []types.BuildError      // bounded
    FailureSummary   string
    BlobRef          string
    DiffArtifactRef  string
    NextSurface      string // top unexecuted surface candidate ID, if any
}
```

Set by the scheduler in the verify-failure branch; cleared when the batch
completes or the run finishes; deliberately NOT cleared by
`prepareControllerPlanningState`. The planner prompt renders it as the lead
section ("latest post-apply verification failure") ahead of the generic
context pack, with typed rows only; the directive asks for a bounded repair
patch against those rows — soft guidance over precise data.

No-plan replan rounds stop being fatal: when the plan stage ends with no
ChangePlan while a `VerifyFailureHandoff` is active, the scheduler grants one
bounded re-dispatch whose planning hint cites the previous round's typed emit
failure (mirrors the read-pipeline "soft-stop hint cites prior emit failure"
pattern). The second consecutive empty round keeps today's terminal behavior.

Context pack dedupe upgrade (P1-5): items carrying an `EvidenceRef` collapse
on `(priority, kind, source_stage, file, line)` regardless of text wording, so
repeated retries cannot flood consumer views with re-worded duplicates of the
same fact. Existing key semantics stay for items without refs.

### 4.6 Structured edit reliability (P1-1)

- `end_line` omitted (0) on `replace`/`delete` defaults to `start_line`
  (single-line edit); the schema documents the default instead of requiring
  copied arithmetic.
- `old_text` mismatch diagnostics include the bounded actual current bytes of
  the cited range (`got: %q`, capped) plus the drift repair hint, so the model
  can correct without re-reading blindly.
- Byte-level trailing-newline normalization: `old_text` matching tolerates a
  missing/extra final `\n` on the last line of the range — a documented,
  structural normalization, not fuzzy matching.
- Regression tests: single-line replace without `end_line`, missing-newline
  old_text, exact-byte mismatch reporting, and existing overlap/dup cases.

### 4.7 Eval regression port + harness hygiene (P2)

Port from the eval workspace into this repo: the evidence ledger doc, the four
`eval/cases/github_issue_*.case`, `eval/fixtures/github_issues/`, the
`eval-github-issues` Makefile target, and the `eval/run.sh` hardening
(apply-mode must fail when no plan is written or apply is skipped; post-apply
inspection supports worktrees whose `.git` is a file). Expected trusted
outcomes are tracked separately from observed outcomes in the case ledger.

## 5. Delivery batches

Each batch: code + focused tests + `go test ./...` + commit + push + ledger
update here.

### Batch 0 — this design ledger
- [x] Audit code, write this document, commit, push.

### Batch 1 — Typed TestSurface + selection + escalation (P0-1, P1-3)
- [x] `types.TestSurface` / `TestSurfaceCandidate`; `ExecutedCommand` +
      `TestSurface` + `ExecutedCommands` on ChangeReport (surface embedded in
      the report instead of a separate `TestSurfaceID`, so the report JSON is
      a self-contained durable artifact).
- [x] `BuildTestSurface` composing existing detectors over the new
      `detectRunnerPlanCandidates` (full per-root inventory; the executor's
      one-runner-per-root collapse in `detectRunnerPlans` is unchanged).
- [x] `run_tests`: typed dead-end escalation (zero-test outcome or missing
      runner binary → top unexecuted candidate with test work, bounded to one
      per Execute), `ExecutedCommands` provenance rows on every report path,
      `GeneratedAt` stamped at the install seam, params through
      `applyStructuredPayloadCompat`.
- [x] Verifier prompt renders the detected surface; planner probe history
      renders executed commands.
- [x] Tests: test-work-outranks-priority, priority tiebreak, make-target
      signal, unconfigured cmake, python framework, escalation picker, e2e
      zero-test escalation (pass + fail directions), auto-detect non-escalation;
      full `go test ./...` green.

### Batch 2 — Attempt state, finish gate, ModePlan mask (P0-2, P1-2)
- [x] `DeriveBatchAttemptState` (ready_to_plan + latest failed verify attempt
      derives `needs_replan` with the attempt's typed cause) + controller
      prompt canonical rendering; progress ledger renders as labeled events.
- [x] `FinishDisposition` typed field (schema enum with semantics in the
      description) + scheduler finish gate evaluated BEFORE
      `ApplyWorkflowDecisionToRun` mutates the run; accept_unverified
      completes with a typed caveat in the result.
- [x] `EmitWriteWorkflowDecision.ParametersFor(ctx)` mode-masked action enum
      via `WorkflowActionsForMode` (single source of truth) + execute-time
      typed rejection (`workflow_action_not_in_mode` repair); scheduler guard
      stays as the third layer.
- [x] Tests: derived-state unit matrix, finish gate blocked/escape/clean
      paths e2e, ModePlan schema masking + runtime rejection, canonical
      prompt rendering; full `go test ./...` green.

### Batch 3 — Durable failure evidence (P0-3, P2 timestamps)
- [x] `worktree.CaptureCommitPatch` (read-only `git show` of the apply
      checkpoint commit) + `persistVerifyFailureEvidence` at the scheduler's
      verify-failure branch: report saved through the standard path even when
      stage hooks are bypassed, attempt patch written as
      `<stem>.attempt-N.diff`, cleanup behaviour untouched (L5).
- [x] Test surface persistence is satisfied by the report embedding from
      Batch 1 (`ChangeReport.TestSurface` serializes into `.report.json`);
      the diff artifact ref attaches to the batch's latest verify attempt.
- [x] `GeneratedAt` backfill in `saveChangeReport`; regression tests: direct
      evidence persistence against a real git worktree, and the controller
      workflow leaving a durable failed report when hooks are bypassed.

### Batch 4 — Approval risk decomposition (P0-4)
- [x] Deterministic pre-image line parser (unified-diff '-' rows walked per
      hunk + structured-edit ranges; pure insertions excluded) +
      `types.DeclSpanSource` + `repomap.NewDeclSpanSource` (declaration LINES
      of exported symbols only — body spans never count; parse tiers 3/4
      report ok=false) + orchestrator wiring via `writeRiskAssessmentInput`
      (nil-safe, both plan-post and phase scheduler sites).
- [x] Risk re-grading: all three IR booleans + Overall=high now grade Medium
      with corroborated/uncorroborated reason codes; `public_decl_line_changed`
      is the precise High; `isPersistenceSchemaPath` joins the typed path
      taxonomy as a hard High. REPL /approve keeps the stricter recorded
      plan-time decision for the same plan fingerprint (graph asymmetry
      cannot silently downgrade manual to auto).
- [x] Analyzer skill wording marks the axes as advisory-and-corroborated;
      tests: body-only public-header patch auto-executes, decl-line patch
      requires approval, nil-graph degrades to medium, unindexed files skip,
      structured-edit spans, path-policy hard grades unchanged (manifest
      High / .git Critical / migration High). Old axes-high contract test
      re-pinned to the new behavior. Full `go test ./...` green.

### Batch 5 — Verify-failure handoff + replan resilience (P0-5, P1-5)
- [x] `types.VerifyFailureHandoff` (typed projection of the failed report:
      bounded failing rows, build errors, executed commands, blob/diff
      artifact refs, next unexecuted surface candidate) + MutableState
      channel that deliberately survives `prepareControllerPlanningState`;
      scheduler sets it per failure and clears on green verify / finish.
- [x] Planner replan prompt opens with "Latest verification failure
      (authoritative)" rendered from the carrier; one bounded re-dispatch
      when a planning round installs no ChangePlan while the carrier is
      active (typed condition — no error-text matching), with the planning
      hint citing the empty round.
- [x] EvidenceRef-anchored context items dedupe on the fact (file:line),
      not wording, so retries cannot crowd consumer Top-N views.
- [x] Tests: handoff projection/bounds/next-candidate, lifecycle e2e
      (survives reset at replan, cleared on green), no-plan single retry +
      first-attempt terminal behavior, lead-section rendering and ordering.
      Full `go test ./...` green.

### Batch 6 — Structured edit reliability (P1-1)
- [x] Omitted `end_line` defaults to `start_line` for replace/delete (the
      `invalid line range N-0` class is gone); `old_text` mismatches echo the
      bounded current bytes with the re-read repair direction; matching
      tolerates exactly one byte-level normalization — the final trailing
      newline. Schema descriptions for start_line/end_line/old_text state
      the defaults and the echo behavior precisely; both validator and
      apply-side recompile share the single seam. Regression tests cover
      single-line replace/delete defaults, newline tolerance both ways,
      mismatch echoes for range and insert anchor, and the byte-rule matrix.

### Batch 7 — Eval port + regression matrix (P2)
- [x] Ported from the eval workspace byte-identically: the evidence ledger
      (with an added expected-vs-observed outcomes section), the four
      `eval/cases/github_issue_*.case`, `eval/fixtures/github_issues/`, the
      `eval-github-issues` Makefile target, and the `eval/run.sh` apply-mode
      + git-file-worktree hardening. `bash -n eval/run.sh` clean.
- [x] Fixed a pre-existing origin/main regression that broke
      `make eval-runner-test` on both clones: the finalizer reject counter
      used a multibyte bracket expression (`[校交]`) whose grep semantics are
      locale-dependent (byte-decomposed → silent zero in non-UTF-8 locales);
      replaced with locale-stable literal alternation. `make eval-runner-test`
      green.
- [x] `docs/architecture.md` deltas: §8.2 controller hardening list (action
      masking, canonical attempt state, finish gate + typed escape,
      VerifyFailureHandoff, durable failure evidence, risk decomposition,
      structured edit defaults) and §8.7 typed TestSurface + dead-end
      escalation.
- [x] Full `go test ./...` + `make test` green; task-10 hygiene pin
      (`TestFinishBlockedReason_DoesNotReadProse`) proves the finish gate
      ignores prose in both directions. Live `make eval-github-issues`
      requires LLM API access and is recorded in the evidence ledger as the
      follow-up re-run command.

## 6. Acceptance criteria

- A repo whose only runnable test contract is a Makefile/script verifies
  through it; a zero-test runner result never stands as verification while an
  unexecuted runnable candidate exists.
- Model-facing batch state is single-valued; `finish` cannot bypass a failed
  post-apply verify without a typed declared disposition.
- Every failed batch attempt leaves report JSON + worktree diff + executed
  command rows + selected surface on disk before cleanup.
- A body-only patch inside a public header auto-executes under `auto_safe`;
  a patch touching an exported declaration line still requires approval.
- Replan rounds open with the typed failure evidence; one empty planner round
  is retried with a typed citation instead of aborting the workflow.
- No new hard gate reads prose or keywords; all new model-facing payload
  fields go through the unified repair layer; read mode and non-write lanes
  untouched (`go test ./...` green).

## 7. Progress ledger

- 2026-06-10: Batch 0 — code audit complete; this design recorded.
- 2026-06-10: Batch 1 — typed TestSurface shipped. Selection rule "runnable
  test work dominates manifest priority"; typed dead-end escalation in
  run_tests (zero tests / missing binary → next candidate with test work,
  once per Execute); ExecutedCommands + TestSurface embedded on every
  ChangeReport path; GeneratedAt stamped at the install seam; run_tests
  params now route through the unified payload repair layer; verifier prompt
  lists detected candidates. `go test ./...` green.
- 2026-06-10: Batch 2 — canonical attempt state + finish gate + ModePlan mask
  shipped. One derived phase per batch (needs_replan carries the typed verify
  cause); finish hard-gated on typed attempt records with the
  finish_disposition=accept_unverified escape lane; ModePlan action enum
  masked at schema-projection, emit-validation, and scheduler layers from one
  action-set source. `go test ./...` green.
- 2026-06-10: Batch 3 — durable failure evidence shipped. Failed verify
  attempts now persist the typed report (with backfilled GeneratedAt) and the
  apply-checkpoint patch as `<stem>.attempt-N.diff` before any cleanup; the
  diff ref attaches to the verify attempt record. Worktree cleanup semantics
  unchanged. `go test ./...` green.
- 2026-06-10: Batch 4 — approval risk decomposition shipped. LLM risk axes
  are advisory medium; hard High comes from typed declaration-line
  intersection and path policy (now including persistence schemas); REPL
  approve applies stricter-wins against the recorded fingerprint. Full
  `go test ./...` green.
- 2026-06-10: Batch 5 — verify-failure handoff shipped. Typed carrier
  survives the planning reset and leads the replan prompt; empty replan
  rounds get one typed-context re-dispatch instead of aborting the
  workflow; evidence-anchored pack items dedupe by fact. Full
  `go test ./...` green.
- 2026-06-10: Batch 6 — structured edit reliability shipped. end_line
  defaults, byte-echoing mismatch diagnostics, trailing-newline tolerance,
  precise schema wording. Full `go test ./...` green.
- 2026-06-10: Batch 7 — eval regression suite ported (4 GitHub-issue cases +
  fixtures + harness hardening + evidence ledger with expected outcomes);
  locale-dependent runner-lib counter regression fixed; architecture.md
  updated; full `go test ./...` + `make test` + `make eval-runner-test`
  green. All seven batches delivered.
