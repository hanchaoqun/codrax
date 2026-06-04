# Multi-Repo Focus Selection Redesign

Date: 2026-06-04

## Problem

Read mode currently chooses the multi-repo active set before the analyzer runs.
When the user has not pinned a focus sub-repo, the routing fold falls through to
a biggest-first fallback and treats that as the active scope. In large workspaces
this often picks the wrong repositories, then the prompt tells the model that
file-system tools must stay inside that mistaken scope. The model can notice the
need for another sub-repo only after being refused by the active-set gate.

The current behavior has three gaps:

- User-pinned focus is not strict. It is added first, but fallback may still fill
  other sub-repos up to the cap.
- No-focus selection is heuristic-first, not model-recommended. The system uses
  file count and language hints before the model has a chance to inspect the
  workspace topology and the actual question.
- The hard cap is still 3 in config/docs/tests. Product direction is default 2,
  ceiling 5.

## Existing Code Path

- `cmd/root.go` discovers multi-repo topology and builds `MultiGraph`.
- `internal/orchestrator/orchestrator.go::Run` calls `RouteActiveSet` before
  `StageAnalyze`.
- `internal/tool/repomap/multigraph/multigraph.go::RouteActiveSet` consumes
  channels A/B/C/D/E:
  - A: focus pins
  - B/C: precise files from analyzer/logs, normally unavailable at Run entry
  - D: noisy language affinity
  - E: biggest-first fallback
- `internal/context/builder.go::formatMultiRepoActiveSetAdvisory` teaches the
  model that only the active set is available to file-system tools.
- `/repos cap` and yaml clamping use `config.MultiRepoMaxActiveCeiling`.

## Red Lines

- Single-repo and `multi-repo=false` must not run a selector or change prompt
  content.
- User focus from explicit UI/config (`--focus`, `/repos focus`) is a precise
  signal and must be obeyed strictly. Do not auto-fill additional sub-repos.
- Natural-language focus in the request may be treated as explicit only when it
  resolves exactly to a topology `RootRel`/path. Do not use keyword matching for
  hard routing.
- Model output is accepted only through a typed schema. No prose parsing for
  routing decisions.
- Biggest-first remains only a degraded fallback and must be labeled as such in
  prompt/UX.
- Read-mode source gates, trace/log/MCP lanes, and write-mode flows must stay
  unchanged outside the multi-repo active-set routing path.

## Design

### Focus Sources

Introduce typed focus source metadata:

- `user_pinned`: `--focus` or `/repos focus`. Strict, no fill.
- `user_explicit_in_request`: model selector named a sub-repo/path that the
  system validates as an exact topology path mention in the current request.
- `model_recommended`: model selector recommendation from compact topology.
- `fallback_preview`: selector unavailable, invalid, or low-confidence; old
  biggest-first/language fallback is used only as a preview.

### Selection Flow

1. Build or reuse topology metadata only. Do not build repo_map indexes for all
   sub-repos.
2. If single-repo or multi-repo disabled, skip this flow entirely.
3. If user pinned focus:
   - validate every focus slug exists;
   - reject clearly if count exceeds `multi_repo_max_active`;
   - active set is exactly the pinned set.
4. If focus count is zero:
   - dispatch a lightweight multi-repo focus selector before analyzer;
   - input is the current user request plus compact topology summary;
   - output is typed `MultiRepoFocusRecommendation`;
   - valid recommendations become the active set, bounded by
     `multi_repo_max_active`.
5. If selector fails, returns empty, or confidence is too low:
   - use existing language/biggest-first routing as fallback;
   - mark the source as `fallback_preview`.
6. Refresh `PendingSubRepos` from the final routing decision as before.

### Selector Contract

The selector is a read-only agent with one tool:

`emit_multi_repo_focus(recommended_focus_subrepos, confidence, source, rationale)`

Each candidate must include:

- `root_rel`: exact sub-repo root relative to workspace root.
- `confidence`: 0.0-1.0.
- `reason`: one short reason tied to the current request.
- `source`: `model_recommended` or `user_explicit_in_request`.

The tool validates `root_rel` against topology and enforces exact request-path
presence for `user_explicit_in_request`. Invalid candidates are discarded with a
recoverable advisory so the model can retry.

### Prompt / UX

The active-set advisory must name the source:

- user pinned: "The user pinned these sub-repos."
- model recommended: "Selected from topology pre-scan."
- fallback preview: "Fallback preview; if wrong, ask the user to pin focus."

REPL/CLI should surface:

- selecting focus;
- selected focus and source;
- number of sub-repos being indexed.

### Configuration

- `MultiRepoMaxActiveDefault` remains 2.
- `MultiRepoMaxActiveCeiling` becomes 5.
- Values above 5 are rejected for explicit user focus and clamped for config as
  existing config behavior expects. `/repos focus` over active cap must show a
  clear error instead of silently trimming.

## Efficiency

The selector adds one small LLM call only for multi-repo/no-focus read-mode
runs. It does not build repo_map for all sub-repos. In common cases it should
reduce latency by avoiding expensive indexing of the wrong 2-3 repositories.
Default active count remains 2; max 5 is opt-in by config/command.

## Task Ledger

- [ ] Update config cap ceiling and docs from 3 to 5 while preserving default 2.
- [ ] Add typed focus source/recommendation structs.
- [ ] Add strict-focus routing inputs and tests.
- [ ] Add model-recommended routing channel and fallback-preview source.
- [ ] Add lightweight selector agent/tool/skill with schema and JSON repair
      coverage.
- [ ] Wire selector before analyzer only for multi-repo/no-focus/read-mode.
- [ ] Update active-set advisory and REPL/CLI render messages.
- [ ] Update `/repos` behavior for focus overflow and cap display.
- [ ] Add tests:
      - single-repo and multi-repo disabled bypass selector;
      - user focus strict, no auto-fill;
      - user focus > cap rejected;
      - no focus uses typed model recommendation;
      - selector failure uses fallback preview;
      - scan/index happens only for selected repos.
- [ ] Run focused tests and push each completed batch.

