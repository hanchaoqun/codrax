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

## Gap List

| ID | Priority | Gap | Current evidence | Target state |
| --- | --- | --- | --- | --- |
| SIC-1 | HIGH | Source-inventory god-file remains too large; render cluster was embedded in `source_inventory_reconcile.go`. | Delivered: render-only helper structs and functions moved to `source_inventory_render.go`; reconcile file dropped from 5211 LOC to 3924 LOC. | Keep render logic isolated and lower the LOC ratchet in SIC-5. |
| SIC-2 | MEDIUM | Execution kernel is wired but not fully load-bearing for absence safety. | `sourceinventory.ExecutionView` exists, but inactive budget paths can still scan `graph.Files`; absence predicates must not accept truncated/partial views as confident absence. | First make exact-absence predicates consume typed truncation/incomplete state and refuse confident absence when view execution is budget-truncated. Then install real budgets at remaining hot call sites. B before A is forbidden because it can revive false absence. |
| SIC-3 | MEDIUM | Proof ledger derivation is lane-neutral, but read consumption remains split. | `loopkernel.DeriveProofCoverageAuthority` and read snapshots exist; write truth-ledger decisions consume proof more directly than read sufficiency/status paths. | Fold proof coverage into read sufficiency/status as soft guidance by default. Only `TruthLedgerFailed` can become hard blocking; weak/missing proof should guide exploration and status cards without over-firing hard gates. Preserve read-mode L1. |
| SIC-4 | MEDIUM | Repair carrier is unified for result lanes, but demand-side repair lacks accepted evidence IDs. | `ToolHandoffCarrier` carries accepted evidence for tool/result handoff; `RepairDirective` has tools and repair metadata but no typed accepted-evidence carrier. | Add bounded typed accepted evidence refs to `RepairDirective`. Only enforcers/EvidenceClosure populate it from accepted evidence state. Keep advisory semantics and do not merge `RepairDirective` into `ToolHandoffCarrier`. |
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

- Add bounded `AcceptedEvidence []AcceptedEvidenceRef` to `RepairDirective`.
- Populate only from `EvidenceClosure` / enforcer paths that already hold
  accepted evidence state.
- Ensure clone/merge/dedupe/render paths preserve IDs without rendering noisy
  prose.
- Add tests for repair merge, handoff bounds, and restricted tool surfaces.

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
| SIC-2 exec kernel absence safety | pending |  |
| SIC-3 proof consumption | pending |  |
| SIC-4 repair accepted evidence | pending |  |
| SIC-5 tripwire coverage | pending |  |
| SIC-6 tracker disposition | pending |  |
