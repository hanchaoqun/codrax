# Write Mode Unified Gap Audit

Date: 2026-06-11
Baseline: `230bdbfc` (after the GitHub-issue P0 hardening + live-eval fix waves)
Method: three parallel deep audits (scheduler state machine, content-risk
signals, artifact persistence) cross-checked against live-run logs and code;
every load-bearing claim re-verified by hand before classification.

## 1. Verified-healthy surfaces (no action)

- **Content-risk hard gates are red-line compliant.** All five
  `safety.AnalyzeWriteContent` signals mapped to High/Critical are
  structurally precise (PEM boundary lines, JSON-parsed package.json
  lifecycle keys, YAML-parsed workflow permission escalation, XML-parsed
  AndroidManifest permissions, path-gated shell download-pipe). None can
  plausibly fire on ordinary Java/C source; the sibling workspace's
  "full-file modify → pending_approval" observation is NOT explained by
  content signals (watch item below).
- **Replan-round report persistence is durable** (live log evidence): every
  verify saves a report; pre-F3 rounds could drift the directory to the
  WorkDir fallback, which the F3 live-plan mirror has since stabilized by
  re-anchoring PlanPath each round.
- The typed mechanisms shipped this session render and act in live runs
  (surface, escalation, canonical state, handoff lead section, attempt
  diffs, completion lane, mirror).

## 2. Gap ledger

| # | Gap | Severity | Disposition |
|---|---|---|---|
| G1 | Resume rebuilds nothing: `verifyFailures` resets (retry budget silently over-runs), `Mutable.ChangePlan` not hydrated from the active batch's plan artifact (`apply_plan` fails on a resumed run), `VerifyFailureHandoff` not rebuilt from the latest failed verify attempt (replan loses its lead evidence) | P0 | FIX |
| G2 | Exploration-budget exhaustion kills the whole run (`return error`) instead of rejecting the action and letting the controller choose plan/replan/finish — asymmetric with the finish-gate reject-and-continue pattern | P1 | FIX |
| G3 | `VerifyFailureHandoff` leaks across batch switches: after `append_batch`/`split_batch`, the new batch's planner prompt opens with the OLD batch's failure section | P1 | FIX |
| G4 | `ask_user` questions are folded into a pipe-joined progress string; the blocked result the operator sees carries only reason prose, not the typed `questions_for_user` | P1 | FIX |
| G5 | Completion-lane verify passes `len(batch.Attempts)` as the diff attempt ordinal; the normal path uses the failed-verify count — same artifact family, two numbering schemes | P2 | FIX |
| G6 | `runPhaseGroup` / `isMultiPhaseRun` (stage II PlanGroup machinery) have zero call sites after the controller-first migration; REPL `/phase` operates a store the controller never writes. Sequential-phase SEMANTICS are not lost — `WorkflowSeedFromWriteAnalysis` converts `PhaseProposal` into planned batches — but the PlanGroup lane is dead code with a live UX surface | P1 | RETIRED (operator approved option B, 2026-06-11): dead scheduler deleted (16 functions + dedicated tests); the live exploration subflow extracted to `write_exploration_subflow.go`; `/phase` re-pointed at the active workflow run's batch view with read-only legacy fallback + banner; the fake-queue verbs next/rollback/resume/skip retired with guidance; `/merge group` + `/reject group` kept as legacy settlement verbs; PhaseGroupID/PhaseIndex fields and the single-pending sibling exemption kept. Recon found one real capability difference: the per-phase LLM acceptance verdict gated phase advancement in the old lane — an LLM judge as a hard gate conflicts with §1.5, so it was deliberately not ported (acceptance criteria still reach the verifier prompt and the reflector); the `acceptance_checker` agent infrastructure remains registered but undispatched, available for a future advisory lane |
| G7 | High/Critical content signals lack negative (lookalike) tests pinning their precision: package-lock.json with lifecycle keys, YAML comments containing `write-all`, `curl \| jq`, shell-pattern text in non-script paths, PEM-header lookalikes inside string literals | P2 | FIX (tests only) |
| G8 | Pre-F3 report-directory drift (verify-N reports landing under the WorkDir fallback while verify-1 landed beside the plan): stabilized by F3; needs a pinning regression so the fallback chain cannot silently regress | P2 | FIX (test only, asserted via the mirror test) |
| G9 | ModeVerify presents the full controller action enum (plan/apply/explore all offered); verify-only semantics are unguarded — same class as the ModePlan mask gap, same SST fix point (`WorkflowActionsForMode`) | P1 | FIX |
| G10 | A batch verified as `unverified` (NoTests outcome on non-test code) completes silently; `finish` carries no caveat, so a run can end "complete" with zero executed assertions and no surface signal | P2 | FIX |
| W1 | Sibling-workspace observation: full-file source+test `modify` plan once landed `pending_approval` at `a89ce192` despite medium-grade axes; not reproduced in this clone (all live plans were `kind=patch`), content signals ruled out by G-audit | watch | needs a typed repro before touching the gate |
| W2 | Edit formatting quality (functional pass compressed `return` onto the `if` line): any detector is a style heuristic — noisy signal — so per the architecture red line it can only ever be soft advisory | watch | candidate: advisory-only validator, never a gate |

## 3. Fix notes (typed-signal discipline)

- G1 hydration reads only typed records: failed-verify counts from
  `DeriveBatchAttemptState`, the plan from the plan artifact next to the
  report directory (or the live mirror file), the handoff from the persisted
  `<stem>.report.json` via the same `BuildVerifyFailureHandoff` projection
  used live.
- G2/G9 reuse existing reject-and-continue and `WorkflowActionsForMode`
  patterns — no new mechanism.
- G3 is enforced on both sides: scheduler clears the carrier when the active
  batch changes; the planner section additionally refuses to render a
  handoff whose BatchID does not match the active batch.
- G10 reads attempt records (`status=="unverified"`) — the same channel the
  finish gate reads for failures.

## 4. Progress

- 2026-06-11: audit recorded; fixes for G1-G5, G7-G10 implemented with
  tests in the same change; G6/W1/W2 recorded for operator decision.
- 2026-06-11: G6 executed per operator approval (option B) — see the G6 row
  for the full disposition. W1/W2 remain open watch items.
