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
- [x] Run multi-instance Lite smoke and manually audit patch satisfaction.
- [x] Relax declaration-line API risk from hard high to medium while preserving
  high/critical structural safety gates.
- [x] Add best-effort per-instance Python verification environment setup for
  Lite smoke and batch adapter runs.
- [x] Route verifier parser/report failures as typed `parser_error` unverified
  outcomes instead of code-failure replan triggers.
- [x] Commit and push to `main`.

## Test Matrix

- Local fake-Codrax smoke: proves applied ref export and JSONL schema.
- Validator failure cases: missing file, duplicate id, missing patch field, empty
  patch under `--require-nonempty-patch`.
- Lite smoke: loads official dataset, checks out base commit, runs Codrax, writes
  predictions/results.
- Lite smoke with env prep: creates a per-instance Python venv, installs
  `pytest<9`/`pytest-json-report` and project deps best-effort, records
  `env_prepare.json`, and still exports predictions when setup fails.
- Harness consumption: wrapper validates predictions and prints or runs the
  official `swebench.harness.run_evaluation` command.
- Regression: `go test ./...` confirms product code paths remain stable.
- Verifier parser gap: pytest/report parser failures produce
  `FailureKindParserError`, complete applied batches as unverified, and do not
  trigger code replanning.

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
- 2026-06-15: Multi-instance Lite smoke
  `eval/results/swebench/lite-smoke-20260615-115056` exposed a write-mode safety
  gap: `pallets__flask-4992` and `psf__requests-2317` produced bounded plans but
  exported empty patches because `public_decl_line_changed` was graded high and
  non-interactive auto-safe runs paused at `pending_approval`. `pytest-dev__pytest-5221`
  produced a non-empty patch. The official harness dry-run consumed all
  predictions, and manual audit classified the two empty patches as approval
  over-blocking rather than adapter export failure.
- 2026-06-15: Fixed the approval over-blocking generically: exported
  declaration-line intersections remain precise `public_decl_line_changed`
  evidence, but grade medium/API-surface by themselves. High/critical remains
  reserved for structural blast-radius signals such as build/dependency
  manifests, CI/workflow automation, hooks, persistence schemas, secret material,
  repo escape paths, and large change sets. Targeted writeflow tests cover
  declaration-line auto execution and declaration-line-plus-manifest approval.
- 2026-06-15: Re-ran the two formerly blocked Lite instances at
  `eval/results/swebench/lite-smoke-20260615-120452`: both exported non-empty
  predictions and official harness dry-run consumed the file. Manual audit found
  `psf__requests-2317` matched the issue intent (`bytes` method decoded before
  request creation). `pallets__flask-4992` surfaced a second quality gap:
  local verify could not run without pytest, and the unverified patch used
  `mode=""`, which existing Flask tests would catch. This motivated the
  adapter environment-prep enhancement rather than making missing pytest a hard
  gate.
- 2026-06-15: Added best-effort per-instance Python venv setup to the adapter
  and enabled it by default in `smoke_lite.sh` via
  `SWEBENCH_PREPARE_PYTHON_ENV=1`. Setup installs `pytest<9` plus
  `pytest-json-report` and project dependencies when possible, records
  `env_prepare.json/log`, injects the venv into Codrax's PATH, and still
  continues prediction export on setup failure. The `<9` bound protects Codrax's
  structured pytest-json-report verifier contract from pytest 9 plugin/report
  compatibility drift.
- 2026-06-15: Hardened the core verifier route exposed by the env-prepared
  Flask run: when the runner starts but no structured report is available
  (`parsePytestJSONReport` missing report after collection/import failure),
  `run_tests` now persists `FailureKindParserError`. Both legacy and
  controller write schedulers treat this as local verification unavailable,
  mark the plan/batch `unverified`, and suppress code replanning. This keeps
  missing/unstable customer test environments from blocking exportable patches
  or wasting DAG budget on non-code failures.
- 2026-06-15: Final verification before push passed:
  `go test ./internal/writeflow ./internal/orchestrator ./internal/tool`,
  `go test ./...`, `make test`, `make`, `git diff --check`, SWE-bench adapter
  Python compile checks, shell syntax checks, local adapter smoke, and the
  env-prepared Flask Lite smoke at
  `eval/results/swebench/lite-smoke-20260615-123031`. The Flask smoke exported
  a non-empty prediction, completed Codrax with `plan_status=unverified` due to
  typed `parser_error`, and the official harness dry-run consumed the output.
