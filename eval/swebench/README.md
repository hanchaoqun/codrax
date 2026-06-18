# Codrax SWE-bench Adapter

This directory adapts Codrax write mode to the official SWE-bench scoring flow.
It does not score patches itself. Codrax produces a predictions JSONL file, and
the official SWE-bench harness remains the authority for evaluation.

## Outputs

The adapter writes one JSON object per selected instance:

```json
{"instance_id":"...","model_name_or_path":"codrax","model_patch":"diff --git ..."}
```

That is the shape consumed by `swebench.harness.run_evaluation`.

## Local Smoke

The local smoke is dependency-free and does not call an LLM. It creates a tiny
Git repository, runs a fake Codrax binary that writes an applied ref, exports a
prediction, and verifies the official harness command can consume the file.

```bash
eval/swebench/smoke_local.sh
```

Use `KEEP=1` to keep the temporary workspace.

## SWE-bench Lite Smoke

Install optional eval dependencies into the ignored local venv. Use Python
3.10+; Python 3.11 is recommended and was used for the smoke run below.

```bash
/opt/homebrew/bin/python3.11 -m venv eval/results/swebench/.venv
eval/results/swebench/.venv/bin/python -m pip install --upgrade pip setuptools wheel
eval/results/swebench/.venv/bin/python -m pip install swebench datasets
```

Build Codrax, then run one to three Lite instances:

```bash
make
SWEBENCH_SMOKE_LIMIT=1 eval/swebench/smoke_lite.sh
```

If the Codrax binary is built in a different worktree from your local
`providers.yaml`, pass it explicitly:

```bash
PROVIDERS_PATH=/path/to/providers.yaml CODRAX_BIN=/path/to/codrax eval/swebench/smoke_lite.sh
```

`run_codrax_swebench.py` also accepts `--providers` directly and defaults to
`CODRAX_PROVIDERS` when that environment variable is set.

Lite smoke defaults to a best-effort per-instance Python venv so Codrax's local
verify stage can run `pytest` when the checkout supports it. The adapter installs
`pytest<9` with `pytest-json-report`, records `env_prepare.json` and
`env_prepare.log` for each instance, and then runs Codrax with that venv on
`PATH`. Legacy Python projects that still import `pkg_resources` are checked
inside the venv; if the import is missing, the adapter installs a compatible
`setuptools<81` and records the check/recheck steps. When `pyproject.toml`
declares `[build-system].requires`, the adapter installs those structured build
requirements into the same venv before editable install. Python 3.9 eval
drivers use `tomli` when present, or a narrow `[build-system].requires`
fallback parser, so historical pyproject build helpers are not skipped merely
because stdlib `tomllib` is unavailable. Common runtime/test
requirements files are installed best-effort with discovered constraints passed
through to pip; dev requirements are used only as a bounded fallback when no
test-focused requirements file exists. Legacy projects that only declare
dependencies in `setup.py` or `setup.cfg` are parsed with Python AST or
ConfigParser; structured `install_requires` / `setup_requires` entries are
installed best-effort before editable install, and ConfigParser-extracted
requirement values have inline comments stripped before pip sees them. For
historical projects with broad lower-bound-only
dependency declarations, the adapter may add temporary compatibility constraints
to the eval venv, such as `numpy<2` when the project declares NumPy without a
major-version ceiling; pass `--disable-python-compat-constraints` to turn this
off. If editable installation still fails because pip's isolated
build environment did not inherit that legacy compatibility, the adapter
performs one bounded `--no-build-isolation` retry. Finally, a non-blocking import
probe records whether discovered checkout import roots are usable in the venv.
Environment setup failures never block prediction export; they leave the run
unverified and the official harness remains the scoring authority. `env_prepare`
telemetry is stable both as a nested object and as top-level result fields:
`env_prepare_status`, `env_prepare_success`, `env_prepare_env_available`,
`env_prepare_failure_kind`, `env_prepare_pytest_available`,
`env_prepare_pytest_json_report_available`, `env_prepare_import_probe_ok`,
`env_prepare_import_roots`, `env_prepare_source_roots`,
`env_prepare_python_compat_constraints`, `env_prepare_venv_python`, and
`env_prepare_failed_step_names`. Those fields are observational only; missing
pytest or partial dependency setup is not a hard code-failure gate.
`results.jsonl` also includes Codrax's typed local verifier verdict
(`verify_status`, `verify_failure_kind`, `verify_failure_reason_code`,
`verify_summary`, `verify_test_count`, `verify_confidence_reason_codes`) plus plan audit fields (`plan_target_paths`,
`plan_change_paths`, `plan_test_change_paths`,
`plan_verification_probe_count`, `workflow_run_id`, `workflow_status`,
`plan_context_paths`, `plan_context_covered_paths`,
`plan_context_uncovered_paths`, `plan_context_coverage_ratio`,
`plan_context_missing_source_paths`,
`delivery_candidate_status`, `delivery_candidate_reason_code`,
`delivery_candidate_relation`, `delivery_patch_fingerprint`,
`delivery_primary_source_plan_id`,
`delivery_source_owner_plan_ids`, `delivery_source_paths`,
`delivery_report_plan_id`, `delivery_source_plan_covers_exported_source_patch`,
`delivery_context_missing_source_paths`,
`plan_owner_boundary_signals`, `plan_owner_boundary_reason_codes`,
`plan_patch_review_status`, `plan_patch_review_hard_block`,
`plan_patch_review_coverage_verdict`,
`plan_patch_review_reason_codes`,
`plan_patch_review_semantic_unverified_codes`,
`plan_patch_review_semantic_unverified_telemetry_codes`,
`plan_patch_review_block_reason`,
`export_allowed_patch_paths`, `dropped_unowned_patch_paths`,
`exported_patch_paths`, `exported_patch_source_paths`,
`exported_patch_test_paths`, `final_plan_source_paths`,
`final_plan_test_only`, `final_plan_covers_exported_source_patch`,
`prediction_audit_block_reason`,
`prediction_confidence_downgrade_reason`, `prediction_verdict`,
`prediction_local_confidence`, and `prediction_blocks_local_acceptance`) so
environment dead-ends, test-edit drift, planner handoff coverage, actual-diff
patch review coverage, failed local verification, and
final-plan-vs-exported-source drift are auditable without changing the official
predictions JSONL shape. Export/local-confidence logic is bound to a typed
delivery candidate: exported source paths must be owned by one or more applied
source plans, and a later test-only validation plan is accepted only through the
explicit `source_plan_with_later_test_followup` relation. If exported source
paths have no applied source-plan owner, the adapter still writes the official
prediction but marks local confidence as `predicted_audit_blocked` with
`prediction_blocks_local_acceptance=true`. Prediction export itself is
restricted to typed-owned applied plan paths; environment/build artifacts
changed during setup are dropped and recorded in `dropped_unowned_patch_paths`.
Context coverage is derived only from persisted
workflow/context-pack typed fields when present; it is audit telemetry, not an
apply/verify gate. Patch-review blockers are derived only from structured
`ChangePlan.patch_review.coverage_summary` when present, with
`ChangePlan.patch_review.findings` retained as a backward-compatible typed
source: hard patch-review errors and unverified semantic coverage findings block
local acceptance telemetry, but never block the official SWE-bench prediction
export. When a `ChangePlan` carries
`verification_probes[]`, Codrax
runs those bounded typed probes before any project-level suite; passing probes
become the local behavior
verdict while the project suite is retained as typed `TestSurface` diagnostics
instead of a hard gate. Failing probes remain real `tests_failed` evidence, and
unavailable probes fall back to the normal runner/unverified path. Adapter
confidence treats any structured `verification_probe/<language>` suite as probe
evidence, not only Python. When a passed
local verdict depends only on verification probes that do not carry typed
contract/symbol coverage, the adapter lowers local confidence via
`prediction_confidence_downgrade_reason` but still exports the patch for the
official harness. Required behavior contracts sourced from
`expected_outcome_fallback` also participate in this downgrade, so a weak
probe cannot produce high local confidence without explicit contract coverage.
Newer Codrax reports include `verification_confidence[]`; the adapter consumes
those report-native reason codes first and also exposes a deduped
`verify_confidence_reason_codes` list in `results.jsonl`. Unavailable or
failed local verification remains non-blocking for official SWE-bench export
when a source patch exists, but the adapter records the typed reason in
`prediction_confidence_downgrade_reason` so local confidence never looks high
when the project environment, probe, build, or tests could not produce a
behavior verdict. A probe-only passed verdict is also downgraded when the
changed source file has no prior P0/P1 context-pack coverage; this consumes only
typed ChangePlan paths and durable context-pack paths and is audit telemetry, not
an apply/verify hard gate.
The adapter also emits owner-boundary audit telemetry for Python plans whose
structured edits wrap an existing call result in a return-shape/type adapter
such as `np.ravel(...)`, `np.asarray(...)`, or `.to_numpy()`. That signal is
derived from ChangePlan edit ASTs, not issue text or model prose. It lowers
local confidence when no stronger verifier/environment downgrade already
applies, because such patches can be symptom-site repairs that leave the callee
or wrapper owner boundary unresolved.
The same AST-only audit lane now flags two additional symptom-workaround shapes:
conditionally suppressing an existing diagnostic call such as `logger.warning`
and writing an external object's private attribute (for example `obj._state =`
outside `self`). These appear in `plan_owner_boundary_signals` with typed reason
codes such as `diagnostic_signal_conditionally_suppressed` and
`external_private_state_sync_workaround`; they lower local confidence but do not
block exporting the official SWE-bench prediction.
Verification telemetry is deliberately split into a normalized typed verdict and
raw executor telemetry. `verify_passed=true` means
`verify_status=passed`: Codrax produced a typed local verifier pass. Treat it
as high-confidence local acceptance only when `prediction_local_confidence=high`
and `prediction_confidence_downgrade_reason` is empty. The raw ChangeReport
`passed` bit is preserved separately as `verify_report_passed_raw`; it can be
true for no-tests/unavailable outcomes and must not be used as a
functional-correctness pass rate.
For internal reporting only, the adapter can also merge a typed human audit file
into `results.jsonl`:

```bash
eval/results/swebench/.venv/bin/python eval/swebench/run_codrax_swebench.py \
  --instances-jsonl /path/to/instances.jsonl \
  --manual-audit-jsonl /path/to/manual_audit.jsonl \
  --workdir eval/results/swebench/custom-run
```

Each manual audit row should contain `instance_id` plus `verdict` or
`manual_audit_verdict` with one of `pass`, `fail`, or `unknown`; optional
`reason_code`, `source`, and `notes` are copied as audit telemetry. The adapter
then emits `manual_audit_*` fields and `local_acceptance_verdict/source`.
`local_acceptance_verdict=pass` means either typed local verification passed
with no confidence downgrade, or local verification was unavailable/unknown/low
confidence and an explicit manual audit passed. True failed local verification,
local audit blockers, and typed manual audit `fail` rows stay `fail`;
free-form manual notes never drive logic. This combined local acceptance proxy
is useful for triage dashboards, but it is still not the official SWE-bench
score. Only the official harness `resolved/total` result should be called pass
rate.
When no `--manual-audit-jsonl` is supplied, `manual_audit_verdict` is empty for
every row. Dashboards should label that as "no typed manual audit recorded",
not as a human-audit pass or fail rate; no manual correctness conclusion exists
until per-instance typed audit rows are actually recorded.
By default
the exported SWE-bench prediction strips repository test/spec path changes and
records them in `dropped_test_patch_paths`; pass `--include-test-patches` only
when debugging Codrax's own generated test edits. The adapter keeps SWE-bench
operational guardrails out of the user issue text; test-diff stripping and audit
fields are typed exporter behavior instead of prompt instructions. Codrax's
Python verifier also recognizes Django source trees with `tests/runtests.py` and
uses that typed harness instead of plain pytest when the repository advertises it
through file structure. During worktree verification, Codrax prepends the active
worktree plus discovered `src/` or `lib/` source roots to `PYTHONPATH` so editable
installs or inherited environments cannot accidentally import the original
checkout. The SWE-bench adapter mirrors those structural source roots into the
per-instance Python environment and records them as typed telemetry; a partial
project install still exports a patch instead of becoming a hard gate. During
verify, Codrax runs a syntax preflight over plan-touched Python source files
before the project runner; syntax/parse failures become typed `build_failure`
results, while missing pytest, dependencies, plugins, or harness support remain
`unavailable` and do not block prediction export. A runner that discovers no
executable tests is persisted as `verification_status=unavailable` with
`failure_kind=no_tests`, even when legacy parser compatibility keeps
`Passed=true` in the raw report. When a Django suite is not explicitly
supplied, the verifier derives a conservative scoped suite from typed ChangePlan
paths plus the repository `tests/` tree before falling back to a wider run.
When an explicit Python/pytest surface is selected and a pytest config is
present, zero discovered tests stay in the typed `no_tests/unavailable` lane
instead of escalating to standard-library unittest discovery.

Per-instance run artifacts intentionally redact SWE-bench gold fields such as
`patch`, `test_patch`, `FAIL_TO_PASS`, and `PASS_TO_PASS` from
`instances/<id>/instance.json` before Codrax runs. This keeps local audit files
near the checkout from exposing oracle information to repository tools while
still preserving the public problem statement and metadata used to build the
request. Python environment prep discovers import roots under the repository
root, `src/`, and legacy `lib/` layouts before running its non-blocking import
probe.

```bash
SWEBENCH_PREPARE_PYTHON_ENV=0 eval/swebench/smoke_lite.sh
```

Pick specific instances when you want a smaller or more targeted smoke:

```bash
INSTANCE_ID=pallets__flask-4045 SWEBENCH_SMOKE_LIMIT=1 eval/swebench/smoke_lite.sh
```

By default the smoke script validates predictions, requires every selected
instance to export a non-empty patch, fails on adapter setup errors, and prints
the official harness command with `DRY_RUN=1`. Lite dry-run also checks that
the configured Python can import `swebench.harness.run_evaluation`, so
"harness-consumable" means both prediction JSONL validation and importable
official entrypoint. Use a Python 3.10+ eval venv; Python 3.9 can install a
package named `swebench` yet fail to import the current harness. Set
`SWEBENCH_CHECK_OFFICIAL_IMPORT=0` only when you intentionally want a
command-shape smoke without official package validation.

This keeps hardening runs from silently treating empty patches or an unusable
harness environment as success while still leaving the JSONL artifacts on disk
for audit. Set `SWEBENCH_FAIL_ON_INSTANCE_ERROR=0`,
`SWEBENCH_FAIL_ON_EMPTY_PATCH=0`, or `SWEBENCH_REQUIRE_NONEMPTY_PATCH=0` only
when deliberately collecting negative fixtures. To run the official
Docker-backed scorer on a prepared Linux/x86_64 SWE-bench host:

```bash
SWEBENCH_RUN_OFFICIAL=1 SWEBENCH_SMOKE_LIMIT=3 eval/swebench/smoke_lite.sh
```

## Batch Runs

Run against a local JSONL fixture:

```bash
eval/results/swebench/.venv/bin/python eval/swebench/run_codrax_swebench.py \
  --instances-jsonl /path/to/instances.jsonl \
  --workdir eval/results/swebench/custom-run \
  --predictions-path eval/results/swebench/custom-run/predictions.jsonl
```

Run against Hugging Face SWE-bench Lite:

```bash
eval/results/swebench/.venv/bin/python eval/swebench/run_codrax_swebench.py \
  --dataset-name SWE-bench/SWE-bench_Lite \
  --split test \
  --isolate-git-history \
  --fail-on-instance-error \
  --fail-on-empty-patch \
  --prepare-python-env \
  --limit 10
```

Then score with the official harness:

```bash
PREDICTIONS_PATH=eval/results/swebench/custom-run/predictions.jsonl \
  eval/swebench/run_official_harness.sh
```

For a command-shape dry-run, set `DRY_RUN=1`. Add
`CHECK_HARNESS_IMPORT=1` when the dry-run should also prove that the configured
Python can load the official harness module:

```bash
DRY_RUN=1 CHECK_HARNESS_IMPORT=1 \
  PREDICTIONS_PATH=eval/results/swebench/custom-run/predictions.jsonl \
  eval/swebench/run_official_harness.sh
```

Summarize the official harness JSON result with explicit denominators:

```bash
eval/results/swebench/.venv/bin/python eval/swebench/summarize_official_results.py \
  --run-report codrax.<run_id>.json \
  --run-id <run_id> \
  --predictions-jsonl eval/results/swebench/custom-run/predictions.jsonl \
  --output-json eval/results/swebench/custom-run/official-summary.json
```

The summary reports `resolved/submitted`, `resolved/completed`, and
`resolved/total` separately. Use `resolved/submitted` for a selected subset run,
and reserve `resolved/total` for full-suite official runs where the report's
`total_instances` denominator matches the intended benchmark scope. Non-empty
patch rate, high-confidence local verifier pass, low-confidence local verifier
pass, and typed manual audit remain separate telemetry and must not be called
official SWE-bench pass rate.

## Isolation

All run artifacts live under `eval/results/swebench/`, which is ignored by Git.
The adapter invokes Codrax through its public CLI and exports the already
materialized applied ref or worktree diff, so read mode, trace/log/data paths,
and the write-mode runtime remain unchanged.

Fair-eval git history isolation is opt-in on the adapter with
`--isolate-git-history` and enabled by default only in `smoke_lite.sh`
(`SWEBENCH_ISOLATE_GIT_HISTORY=1`). When enabled, the per-instance checkout
stays at the SWE-bench `base_commit`, but branch/tag refs are deleted, reflogs
are expired, unreachable objects are pruned, and Codrax is invoked with
`--eval-disable-git-history` so structured git history tools and shell git
history commands refuse before exposing future fix commits. Normal Codrax runs
and direct adapter runs without this flag keep git history tools available.
