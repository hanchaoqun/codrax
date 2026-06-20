# Source Inventory Structural Convergence Plan

## Scope

This document is the tracking ledger for the six remaining architecture gaps in
the 2026-06-20 source-inventory/read-loop convergence audit. The goal is
commercial-grade closure through class-level fixes, not per-shape guards.

Hard gates must consume typed artifacts only: path roles, source-inventory
observations, execution budget state, proof ledgers, repair directives, tool
schemas, and evidence IDs. User prose, model rationale, visible thinking, and
rendered Markdown remain soft context.

Initial calibrated facts:

- `internal/tool/source_inventory_reconcile.go`: 5211 LOC / 211 funcs.
- `sourceInventoryExecBudget{}` zero-value bypass literals: 0.
- `internal/tool/sourceinventory/` already contains the budget and execution
  view kernel.
- Dead resumability fields: 0.
- Cross-stage repair result carrier is `types.ToolHandoffCarrier`.
- Demand-side repair carrier is `types.RepairDirective`.

Current calibrated facts after SIC-1:

- `internal/tool/source_inventory_reconcile.go`: 3924 LOC.
- `internal/tool/source_inventory_render.go`: 1308 LOC.
- Render-only helpers/functions now live in the render file; BusContext/read
  miss hint helpers remain in the reconcile file.

Current calibrated facts after SIC-2:

- Source-inventory exact-absence authority lives in
  `internal/types/source_inventory_absence.go`.
- `internal/types/source_inventory_observation.go`: 490 LOC, below its ratchet.
- Complete zero member sets prove absence only when source classes are complete,
  observation/page state is complete, and candidate execution is not truncated.

Current calibrated facts after SIC-3:

- Read proof consumption uses `loopkernel.ReadProofGuidance`.
- Weak/missing/unavailable read proof is advisory in retry checkpoints; failed
  truth remains the only hard proof state.
- Parallel lane handoff consumes the same guidance view instead of interpreting
  proof snapshots separately.

Current calibrated facts after SIC-4:

- `types.RepairDirective` now carries bounded
  `AcceptedEvidence []AcceptedEvidenceRef`.
- `MutableState.AppendEvidence` is the single accepted-evidence entry point and
  mirrors compact refs into `EvidenceClosure`.
- `EvidenceClosure.AddRepair`, clone, merge, active repair, pending repair, and
  consume paths preserve accepted-evidence refs without rendering them into
  retry prose.

## Gap List

| ID | Priority | Gap | Current evidence | Target state |
| --- | --- | --- | --- | --- |
| SIC-1 | HIGH | Source-inventory god-file remains too large; render cluster was embedded in `source_inventory_reconcile.go`. | Delivered: render-only helper structs and functions moved to `source_inventory_render.go`; reconcile file dropped from 5211 LOC to 3924 LOC. | Keep render logic isolated and lower the LOC ratchet in SIC-5. |
| SIC-2 | MEDIUM | Execution kernel is wired but not fully load-bearing for absence safety. | Delivered: exact-absence predicates now consume complete source-class, observation/page, and execution budget state; Go string-enum special path uses the budgeted execution view. | Keep new source-inventory hot paths on `sourceinventory.ExecutionView`/`Budget` and reject confident absence from partial views. |
| SIC-3 | MEDIUM | Proof ledger derivation is lane-neutral, but read consumption remains split. | Delivered: `ReadProofGuidance` is the read-side consumer view over `ProofSnapshot`; retry checkpoints and parallel lane handoff consume the same authority. | Continue to keep weak/missing/unavailable proof advisory-only; only failed truth can become hard blocking. Preserve read-mode L1. |
| SIC-4 | MEDIUM | Repair carrier is unified for result lanes, but demand-side repair lacked accepted evidence IDs. | Delivered: `RepairDirective` carries bounded accepted-evidence refs; `AppendEvidence` mirrors accepted evidence into `EvidenceClosure`; repair clone/merge/dedupe paths preserve refs without rendering them. | Keep demand-side repair and result-side `ToolHandoffCarrier` separate; consume typed refs in downstream handoff/status/report surfaces without parsing prose. |
| SIC-5 | HIGH | Source-inventory convergence tripwire has coverage and slack holes. | `sourceInventoryClusterFiles` scans only `internal/tool` and `internal/types`; `internal/tool/sourceinventory/` is outside the LOC ratchet. `source_inventory_reconcile.go` ceiling is 5236 while actual is 5211. | Expand tripwire scan to `internal/tool/sourceinventory`; add explicit ceilings for `budget.go` and `execution_view.go`; ratchet stale ceilings down to current LOC. |
| SIC-6 | HIGH | RNE tracker still has open high-water/noise gaps not covered by SIC-1..SIC-5. | Tracker still mentions RNE-C47/C48, RNE-C1/C4/C6/C12, RNE-C15, RNE-C65, and explorer high-water around 42. | Add a tracker disposition layer: classify each remaining RNE as closed by SIC work, superseded by a kernel, or still open with a concrete owner batch. High-water/runaway signals must become explicit eval/status telemetry, not scattered prose. |

## Delivery Order

1. **Design ledger**: land this document with the gap list, tasks, acceptance
   criteria, and progress table.
2. **SIC-1 / target #4**: split render cluster out of the god-file.
3. **SIC-2 / target #3**: make exec kernel absence-safe, then load-bearing.
4. **SIC-3 / target #5**: unify read proof-ledger consumption softly.
5. **SIC-4 / target #6**: add demand-side accepted evidence to repair carrier.
6. **SIC-5**: close tripwire coverage/slack holes.
7. **SIC-6**: refresh RNE tracker and telemetry ownership.

## Detailed Tasks

### SIC-1 Render Cluster Split

- Move render-only helper structs:
  `sourceInventoryObservationScopeGroup`,
  `sourceInventorySuggestedFileGroup`,
  `sourceInventorySuggestedFile`, and
  `sourceInventorySuggestedFileCandidate`.
- Move render-only functions from the render cluster into
  `internal/tool/source_inventory_render.go`.
- Keep `sourceInventoryReadFilePathMissHint` and
  `sourceInventoryReadFileRequestedRel` in `source_inventory_reconcile.go`.
- Preserve exported function names and package-private call sites.
- Validate with source-inventory focused tests, convergence ratchet, and
  `go test ./...`.

### SIC-2 Exec Kernel Load-Bearing

- Audit exact-absence predicates and all complete-zero inventory checks.
- Add typed execution state checks so `CandidateBudgetTruncated`,
  incomplete source-class universe, incomplete sets, or budget-truncated
  execution cannot prove confident absence.
- Add focused tests for truncated zero-row observations refusing exact absence.
- Replace remaining inactive budget hot-path calls with real budgets only after
  absence predicates are safe.
- Validate with source-inventory exact-absence tests and representative eval
  cases that previously risked false absence.

### SIC-3 Proof Ledger Consumption

- Locate read sufficiency/status-card consumers that still infer proof state
  separately from `ProofSnapshot` / `TruthLedger`.
- Add a read-side projection that consumes the shared proof authority as soft
  next-action guidance.
- Treat `TruthLedgerFailed` as hard; treat weak/missing/unavailable as
  advisory exploration/status signals unless another typed gate already blocks.
- Add tests that weak proof does not force loops while failed proof remains
  actionable.

### SIC-4 RepairDirective Accepted Evidence

- Delivered: added bounded `AcceptedEvidence []AcceptedEvidenceRef` to
  `RepairDirective`.
- Delivered: `MutableState.AppendEvidence` mirrors accepted evidence into
  `EvidenceClosure`; `EvidenceClosure.AddRepair` snapshots that typed state
  into new repairs.
- Delivered: duplicate repairs merge accepted-evidence refs instead of dropping
  later context.
- Delivered: clone/merge/pending/consume/active repair paths preserve refs.
- Delivered: render paths do not print accepted-evidence refs into retry prose.
- Delivered tests cover repair merge, render non-leakage, closure
  accepted-evidence snapshotting, and MutableState-to-repair propagation.

### SIC-5 Tripwire Coverage

- Expand `sourceInventoryClusterFiles` to scan `internal/tool/sourceinventory`.
- Add ceilings for `budget.go` and `execution_view.go`.
- Ratchet `source_inventory_reconcile.go` ceiling to current or lower after
  SIC-1.
- Add a test path assertion so future kernel files cannot silently escape.

### SIC-6 RNE Tracker Disposition

- Add a tracker table mapping RNE-C47/C48, RNE-C1/C4/C6/C12, RNE-C15, and
  RNE-C65 to concrete owner batches.
- Add high-water telemetry criteria for explorer iteration/runaway signals.
- Refresh `read_mode_noise_convergence_eval_gap_20260620.md` so stale
  "remaining" text does not contradict delivered kernel work.
- Pick six representative eval cases for the next validation batch after the
  structural work lands.

## Acceptance Criteria

- `source_inventory_reconcile.go` shrinks and does not gain new render logic.
- Broad/truncated source-inventory observations cannot prove exact absence.
- Proof-ledger read consumers share the lane-neutral authority without making
  weak proof a new hard gate.
- Repair directives carry accepted evidence IDs across demand-side repair
  handoff without merging demand/result carriers.
- Tripwire scans the sourceinventory kernel subpackage and has no stale slack.
- RNE tracker states are current and actionable.
- `go test ./...` passes after each code batch.

## Progress

| Batch | Status | Notes |
| --- | --- | --- |
| Design ledger | delivered | Initial gap list and task breakdown recorded and pushed. |
| SIC-1 render split | delivered | Render-only helper structs and functions moved into `internal/tool/source_inventory_render.go`; `sourceInventoryReadFilePathMissHint` and `sourceInventoryReadFileRequestedRel` remain in `source_inventory_reconcile.go`. `source_inventory_reconcile.go` is down from 5211 LOC to 3924 LOC on latest main. Focused `go test ./internal/tool -run 'TestRepoLanguageCensus_BuilderInScope|TestSourceInventoryConvergence|TestSourceInventory|TestPublishSourceInventoryObservationFromLens|TestRenderSourceInventory'` passed. Full `go test ./...` passed. |
| SIC-2 exec kernel absence safety | delivered | Added `internal/types/source_inventory_absence.go`; source-inventory exact absence now blocks on incomplete source-class universe, incomplete observation, incomplete page, or `CandidateBudgetTruncated`. Pre-emit and contract twin gates share the same typed authority. Go string-enum candidate special path now uses a budgeted execution view and carries truncation. Focused source-inventory/absence tests passed, LOC ratchet passed, and full `go test ./...` passed. |
| SIC-3 proof consumption | delivered | Added `loopkernel.ReadProofGuidance`; weak read proof renders as `mode=advisory` in retry checkpoints and does not hard-block. Parallel lane handoff now consumes the same guidance view. Focused proof/retry tests passed and full `go test ./...` passed. |
| SIC-4 repair accepted evidence | delivered | Added demand-side accepted-evidence carrier to `RepairDirective`; `MutableState.AppendEvidence` seeds `EvidenceClosure`; `AddRepair` snapshots refs and duplicate repair merge preserves them. Focused `go test ./internal/types -run 'Test(MergeRepairs_MergesAcceptedEvidenceCarrier|RepairDirectiveRender_DoesNotRenderAcceptedEvidenceCarrier|EvidenceClosure_AddRepairCarriesAcceptedEvidenceSnapshot|MutableStateAppendEvidenceSeedsRepairAcceptedEvidence|EvidenceClosure_ExploreForkMergeDoesNotDuplicateBaselineEvents|AddRepair_ReadFile_MirrorsToPendingReads|ConsumeRepairs_RetainsLivePendingReadRepairs)$'` passed; full `go test ./internal/types` passed. |
| SIC-5 tripwire coverage | pending |  |
| SIC-6 tracker disposition | pending |  |
