# SWE-bench Adapter Delivery Ledger

Date: 2026-06-15

## Goal

Support official SWE-bench evaluation without changing Codrax runtime behavior.
Codrax should generate a predictions JSONL file with `instance_id` and
`model_patch`, then delegate scoring to the official SWE-bench harness.

## Current Gap

- Codrax write-mode evals produce internal JSON records and applied refs, but no
  SWE-bench-compatible predictions artifact.
- Existing eval scripts know how to materialize applied worktrees, but they are
  oriented around Codrax fixtures rather than SWE-bench instances.
- Official SWE-bench scoring needs repository checkout at `base_commit`, a model
  patch, and a separate harness run; Codrax previously had no one-command bridge.
- The local development machine may not be a suitable official scoring host
  because SWE-bench scoring is Docker-backed and commonly run on Linux/x86_64.
- The current official `swebench` Python package requires Python 3.10+ syntax;
  macOS system Python 3.9 installs older wheels or fails to import the harness.

## Architecture

The adapter is intentionally outside the product scheduler:

```text
SWE-bench dataset/local JSONL
  -> clone/cache repo
  -> checkout base_commit
  -> codrax --mode=write
  -> locate ChangePlan
  -> export applied_commit_sha/ref/worktree diff
  -> predictions.jsonl
  -> official swebench.harness.run_evaluation
```

Hard scoring remains owned by SWE-bench. Codrax only emits typed artifacts and
records adapter results.

## Files

- `eval/swebench/run_codrax_swebench.py`: batch adapter from instances to
  predictions JSONL and per-instance result ledger.
- `eval/swebench/validate_predictions.py`: dependency-free schema preflight.
- `eval/swebench/run_official_harness.sh`: thin wrapper around official scoring.
- `eval/swebench/smoke_local.sh`: no-network local smoke with a fake Codrax.
- `eval/swebench/smoke_lite.sh`: SWE-bench Lite smoke for one to three instances.
- `eval/swebench/codrax_swebench.yaml`: eval-only runtime settings.
- `eval/swebench/README.md`: operator guide.

## Red Lines

- No changes to read mode, trace/log/data, operation mode, or scheduler topology.
- No gold patch access and no SWE-bench-specific prompt branching.
- No keyword routing over user issue text or model prose.
- No product dependency on `swebench` or `datasets`; they are optional eval
  dependencies installed in ignored local Python 3.10+/3.11 venvs or scoring
  hosts.
- Empty patches are allowed for batch continuity, but validation can require
  non-empty patches for smoke tests.

## Task List

- [x] Add SWE-bench prediction validator.
- [x] Add Codrax-to-SWE-bench batch adapter.
- [x] Add official harness wrapper.
- [x] Add no-network local smoke.
- [x] Add SWE-bench Lite smoke wrapper.
- [x] Add operator README.
- [x] Install optional eval dependencies locally.
- [x] Run py/shell static validation.
- [x] Run local smoke.
- [x] Run SWE-bench Lite one-instance prediction smoke.
- [x] Run official harness dry-run consumption check.
- [x] Run Go regression check.
- [ ] Commit and push to `main`.

## Test Matrix

- Local fake-Codrax smoke: proves applied ref export and JSONL schema.
- Validator failure cases: missing file, duplicate id, missing patch field, empty
  patch under `--require-nonempty-patch`.
- Lite smoke: loads official dataset, checks out base commit, runs Codrax, writes
  predictions/results.
- Harness consumption: wrapper validates predictions and prints or runs the
  official `swebench.harness.run_evaluation` command.
- Regression: `go test ./...` confirms product code paths remain stable.

## Progress Ledger

- 2026-06-15: Added adapter skeleton, validator, harness wrapper, smoke scripts,
  README, and this ledger.
- 2026-06-15: Installed Homebrew Python 3.11 and an ignored
  `eval/results/swebench/.venv` with `swebench 4.1.0` and `datasets 5.0.0`.
  macOS system Python 3.9 was rejected because the official harness import needs
  Python 3.10+ syntax.
- 2026-06-15: Local fake-Codrax smoke passed: non-empty prediction generated,
  schema validated, and official harness command constructed.
- 2026-06-15: SWE-bench Lite smoke passed for `pallets__flask-4045` at
  `eval/results/swebench/lite-smoke-20260615-111050`. Codrax produced a
  non-empty `model_patch` of 467 bytes and the official harness dry-run consumed
  the predictions file. This run exposed that Codrax internal verify used to
  end as `verify_failed` when the checked-out Flask repo had no pytest
  environment, even though the patch was exportable and the official harness
  remains the scoring authority.
- 2026-06-15: Regression passed: script syntax checks, local smoke,
  `git diff --check`, and `go test ./...`.
- 2026-06-15: Follow-up hardening: typed `runner_missing` now lands as
  `unverified(reason=runner_missing)` instead of hard-blocking / marking the
  patch as `verify_failed`. True build/test failures still stay `verify_failed`;
  missing customer/local test dependencies preserve the patch and surface a
  transparent unverified caveat.
- 2026-06-15: Verified the hardening with SWE-bench Lite
  `pallets__flask-4045` at `eval/results/swebench/lite-smoke-20260615-113603`:
  predictions validated, official harness dry-run consumed the file,
  `codrax_exit_code=0`, `plan_status=unverified`, and the final user result
  carried an unverified caveat instead of blocking on missing pytest.
