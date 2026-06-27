# D1-F10g.173 Focused Eval Audit — read_combo runtime/source citation lane

Case: `eval/cases/read_combo_log_current_source_explanation.case`

Run directory: `eval/results/read_combo_log_current_source_explanation-20260627-180916`

Result before fix: `FAIL`

Result after fix: `PASS`

Failure reason:

- `no_regex_match:internal/(orchestrator|agent|llm|render|tool)/[^[:space:]]+\.go:[0-9]+`

Manual audit:

- The model produced a plausible mixed runtime/source explanation and emitted 10 source citations through `emit_answer_document`.
- `emit_answer_document` then logged `normalized 12 runtime-artifact citation carrier(s) to observation provenance`.
- The final rendered answer contained code concepts and line numbers in prose, but no `internal/...go:<line>` citation surface. This made the eval fail and, more importantly, degraded auditability for mixed runtime/current-source answers.

Root cause class:

- Runtime/root-cause citation cleanup treated an active runtime-grounding plan with no `CurrentSourceEvidenceOrigin` as observation-only and removed all citations.
- That was too broad for route-backed mixed runtime/source turns. When typed route/request metadata says current source is required, current-repo citations are answer-grade evidence and must not be downgraded to external observation provenance.

Fix:

- Added shared `answerDocumentRequiresCurrentSourceLane` helper using `RequestModel.CurrentSourceLaneDecision()` and `RouteBackedExternalObservationRequiresCurrentSource`.
- `normalizeRuntimeArtifactCitationRefs` no longer enters the drop-all citation branch when current source is required or current-source observation support exists.
- `preCheckRuntimeObservationRepoContamination` consumes the same helper so the hard pre-emit check and cleanup logic cannot drift.

Validation:

- Focused artifact-citation regression passed:
  `go test ./internal/tool -run 'TestNormalizeRuntimeArtifactCitationRefs|TestPreCheckArtifactObservedFrameCitations|TestNormalizeRuntimeArtifactVisibleCitationSentinels|TestPreCheckRuntimeObservationRepoContamination' -count=1`
- Full regression and build passed: `go test ./...`, `make`.
- Focused eval after final answer-display slice fix passed:
  `eval/results/read_combo_log_current_source_explanation-20260627-225350`
  - verdict: `PASS`
  - wall: `194s`
  - `read_file=6`
  - `repo_map=1`
  - `unavailable_tool_attempts=0`
  - `tool_history_prunes=0`
  - `explorer_iters=7`
  - `midloop_inject=5`
  - `finalizer_iters=1`
  - `answer_contract_violations=0`

Follow-up:

- The latest focused eval no longer leaks read audit supplement tables into the final answer and reduced the loop to `explorer_iters=7`, `midloop_inject=5`, `wall=194s`. This is acceptable for the focused correctness batch, but D1-G46 remains open for broader performance/advisory thresholds before the next 6-case representative run.
