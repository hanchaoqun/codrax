# Eval Hardening Task Tracking

This document is the durable task tracker for the staged commercial-grade
hardening work started from the 2026-05-11 full eval sweep.

## Current Evidence

- Sweep: `PARALLEL=4 TIMEOUT=1200 CASES_GLOB="eval/cases/*.case eval/cases/harmony/*.case" bash eval/parallel_all.sh`
- Completed at audit time: 20 / 65 cases, all PASS.
- Main observed risks:
  - eval harness portability and workspace pollution
  - finalizer support-lane/block-kind contract conflicts
  - over-strict enumeration label grounding
  - coarse `must_include` matching without term provenance
  - oversized mixed repair bundles
  - explorer long-tail loops after enough answer evidence exists

## Batch 1: Eval Harness Commercialization

- [x] Fix macOS Bash 3.2 `set -u` empty-array crash in `eval/run.sh`.
- [x] Add portable timeout fallback for macOS in parallel eval runners.
- [x] Skip per-case `make` when `CODRAX_BIN` points at a sweep snapshot.
- [x] Make parallel runner use sliding-window concurrency instead of batch barrier.
- [x] Add first-class per-case telemetry columns to `parallel_all_summary.md`.
- [x] Keep eval-generated artifacts out of the main working tree.
- [x] Add shell-level regression checks for runner portability.

## Batch 2: Finalizer Contract Conflict Resolution

- [x] Audit block-kind requirements for current-status diagnostics.
- [x] Add deterministic conflict detection for mutually exclusive block-kind demands.
- [x] Introduce/route a support lane that allows `decision` for verdict blocks.
- [x] Add regression covering `current_status_verdict_missing` plus lane-kind mismatch.

## Batch 3: Typed Label And Must-Include Provenance

- [x] Split answer item labels into symbol/runtime/display/role/user-phrase intent.
- [x] Make `enumeration_label_ungrounded` hard only for symbol/runtime labels.
- [x] Let display/role labels pass when the item is citation-grounded.
- [x] Extend `must_include` with term kind and provenance.
- [x] Ensure runtime frames and user phrases satisfy in appropriate surfaces.
- [x] Add negative tests that real hallucinated identifiers still fail.

## Batch 4: Repair Layering And Deterministic Small Fixes

- [x] Partition finalizer retry repairs into shape, grounding/consistency, topic/coverage, and enrichment groups.
- [ ] Apply deterministic small fixes for safe structural cases.
- [x] Prevent wrong-topic repairs from bundling underfilled/enrichment requests in the same required-change list.
- [ ] Add retry-loop guard for repeated identical patch failures.

## Batch 5: Explorer Answer-Ready Early Stop

- [ ] Define an answer-ready checklist from TaskGraph/EvidencePlan obligations.
- [ ] Record which answer faces already have enough typed evidence.
- [ ] Stop or soften unread-file pushes once all required faces are covered.
- [ ] Add long-tail regression for architecture/mechanism questions.

## Batch 6: Prompt Audit And Regression

- [ ] Audit LLM-facing prompt changes before editing prompt text.
- [ ] Keep prompt changes contract-oriented, not keyword hacks.
- [ ] Verify no prompt violates repository red lines.
- [ ] Run focused unit tests and `go test ./...`.
- [ ] Re-run targeted eval cases that previously exposed long-tail failures.
