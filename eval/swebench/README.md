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

Lite smoke defaults to a best-effort per-instance Python venv so Codrax's local
verify stage can run `pytest` when the checkout supports it. The adapter installs
`pytest<9` with `pytest-json-report`, records `env_prepare.json` and
`env_prepare.log` for each instance, and then runs Codrax with that venv on
`PATH`. Legacy Python projects that still import `pkg_resources` are checked
inside the venv; if the import is missing, the adapter installs a compatible
`setuptools<81` and records the check/recheck steps. When `pyproject.toml`
declares `[build-system].requires`, the adapter installs those structured build
requirements into the same venv before editable install. Common runtime/test
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
`exported_patch_paths`, `exported_patch_source_paths`,
`exported_patch_test_paths`, `final_plan_source_paths`,
`final_plan_test_only`, `final_plan_covers_exported_source_patch`,
`prediction_audit_block_reason`,
`prediction_confidence_downgrade_reason`, `prediction_verdict`,
`prediction_local_confidence`, and `prediction_blocks_local_acceptance`) so
environment dead-ends, test-edit drift, planner handoff coverage, failed local
verification, and final-plan-vs-exported-source drift are auditable without
changing the official predictions JSONL shape. When the exported source patch is
not owned by the final durable plan (for example a later test-only replan
verified successfully), the adapter still writes the official prediction but
marks local confidence as `predicted_audit_blocked` with
`prediction_blocks_local_acceptance=true`. Context coverage is derived only from persisted
workflow/context-pack typed fields when present; it is audit telemetry, not an
apply/verify gate. When a `ChangePlan` carries `verification_probes[]`, Codrax
runs those bounded typed probes before any project-level suite; passing probes
become the local behavior
verdict while the project suite is retained as typed `TestSurface` diagnostics
instead of a hard gate. Failing probes remain real `tests_failed` evidence, and
unavailable probes fall back to the normal runner/unverified path. When a passed
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

By default the script validates predictions and prints the official harness
command with `DRY_RUN=1`. To run the official Docker-backed scorer on a prepared
Linux/x86_64 SWE-bench host:

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
  --prepare-python-env \
  --limit 10
```

Then score with the official harness:

```bash
PREDICTIONS_PATH=eval/results/swebench/custom-run/predictions.jsonl \
  eval/swebench/run_official_harness.sh
```

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
