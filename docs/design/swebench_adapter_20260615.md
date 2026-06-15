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
- [x] Run additional Lite smoke on Astropy and Django issue shapes, manually
  audit patch satisfaction, and fix the zero-test / Django harness verify loop.
- [x] Run additional non-Go Lite smoke on pytest/Sphinx/SymPy issue shapes,
  manually audit patch satisfaction, and record controller/planner/verifier
  gaps.
- [x] Add plugin-free pytest text fallback for JSON/report parser failures while
  preserving collection/import startup failures as `parser_error` unverified.
- [x] Tighten completed-exploration handoff semantics so planner convergence is
  bounded and does not restart broad investigation after a high-confidence
  read-only exploration.
- [x] Preserve degraded read-only exploration evidence as typed write handoff so
  controller batches can continue to bounded planning after timeout/cancel when
  useful evidence was already emitted.
- [x] Include Python `kind=patch` changes in dry-build syntax validation by
  applying patches into the scratch overlay before `py_compile`.
- [x] Keep SWE-bench operational guardrails out of the user issue text; rely on
  typed redaction/export behavior instead.
- [x] Broaden Python verification-probe coupling to same-package public API
  imports while still rejecting isolated copied-implementation probes.
- [x] Stop verifier immediately after `run_tests` produces a typed passed or
  unavailable `ChangeReport`, preventing duplicate test runs on parser/env
  unavailable outcomes.
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
- Python harness fit: Django source trees with `tests/runtests.py` use the typed
  Django runner rather than plain pytest. Pytest reports with zero executed
  tests, including exit 2/4 selector or CLI mismatches, are preserved as
  `NoTestsRunners` and complete unverified instead of triggering replan.
- Worktree verifier isolation: Python verification prepends the active worktree
  to `PYTHONPATH`, sets subprocess `PWD` to the runner cwd, and drops inherited
  original-repo import roots so editable installs do not make tests import
  pre-patch source.
- Scoped Django verification: when the verifier omits a Django suite, `run_tests`
  derives a conservative suite from typed ChangePlan paths plus the `tests/`
  tree. Explicit test paths win; source-file inference avoids unrelated
  `test_<name>.py` matches whose test directory label does not match the source
  path tokens.
- Controller terminal behavior: once a batch is complete with typed unverified
  verify (`no_tests`, `runner_missing`, or `parser_error`), repeated plan/replan
  decisions targeting the same batch are overridden to finish, while append/split
  remains available for real follow-up batches.
- Pytest parser fallback: if `pytest-json-report` or plugin/report generation
  fails, `run_tests` runs a plugin-free verbose pytest text fallback. Text
  verdicts are accepted only when the runner exposes case-level execution rows;
  startup, collection, or import failures without executed cases remain
  `FailureKindParserError` and therefore complete as unverified rather than
  triggering code replanning.
- Exploration convergence: after a controller-requested read-only exploration
  completes and writes a typed handoff/context pack, the next planner dispatch is
  capped to bounded ChangePlan synthesis. Exploration questions remain separate
  from unresolved unknowns so investigated questions do not re-enter the planner
  as open gaps.
- Degraded exploration handoff: if read-only exploration times out or is
  canceled after `emit_evidence`/read-set progress, write mode projects the
  typed emitted evidence and closure read files into `WriteExplorationHandoff`
  and marks the active batch `ready_to_plan` while preserving the explore attempt
  status as `degraded`.
- Python patch dry-build: `kind=patch` changes are applied into the scratch
  overlay before Python syntax validation, so `py_compile` catches syntax
  failures introduced by patch hunks without requiring pytest or ruff.
- SWE request framing: the default adapter request contains only the public
  instance id, public issue text, and "Fix the repository behavior described
  above."; gold-patch avoidance, test-diff stripping, and prediction export
  rules live in typed adapter behavior and sanitized artifacts, not in the user
  request body.
- Python probe coupling: a probe for a changed production submodule may import
  the exact module, a package prefix, or a sibling public API under the same
  top-level package. The check compares deterministic imports to typed
  `changes[].path` module candidates and still rejects unrelated imports such as
  standard-library-only copied logic.
- Verifier one-shot behavior: once `run_tests` installs a typed passed or
  unavailable `ChangeReport`, the verifier loop stops before another model turn;
  after any report exists, the verifier schema filter removes `run_tests` from
  later turns while preserving `emit_test_results` for structured failed-test
  classification.

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
- 2026-06-15: Additional Lite smoke
  `eval/results/swebench/lite-smoke-20260615-141507` ran
  `astropy__astropy-14365`, `django__django-11099`, and
  `django__django-11133`. All three exported non-empty predictions and the
  official harness dry-run consumed the file. Manual audit found the patches
  matched issue intent: QDP command matching became case-insensitive, Django
  username validators switched to `\A...\Z`, and `HttpResponse.make_bytes`
  treats `memoryview` like bytes. The run exposed a controller-level verify
  loop: plain pytest on Django either executed zero tests (`exitcode=2/4`) or
  failed because Django settings were not configured, causing unnecessary
  replan and no-op `emit_change_plan` attempts after the code patch had landed.
- 2026-06-15: Fixed that gap generically in typed verification. The Python
  runner now detects Django `runtests.py` by file structure and dispatches that
  harness. Pytest zero-test reports with exit 2/4/5 flow through
  `NoTestsRunners`, not `tests_failed`. The controller treats `NoTestsRunners`
  as terminal unverified, suppresses retry, and overrides later same-batch
  plan/replan/apply/verify decisions to finish instead of re-entering planning.
  Targeted tests cover parser classification, Django framework normalization,
  legacy suppress behavior, and controller no-tests terminal behavior.
- 2026-06-15: Follow-up SWE-bench Django verification hardening landed after
  re-running `django__django-11099` and `django__django-11133`. The first rerun
  proved the Django runner path but exposed that editable installs could still
  import the original checkout instead of the worktree; Python runner env now
  pins worktree import precedence and records `PWD` precisely. The next run
  `eval/results/swebench/lite-smoke-20260615-152027` exported two non-empty
  predictions and official harness dry-run consumed them; `django__django-11099`
  verified with `auth_tests.test_validators` from the worktree. It also exposed
  a scoped-suite false positive for `django__django-11133`
  (`template_tests.test_response`). The final rerun
  `eval/results/swebench/lite-smoke-20260615-152909` confirmed the generic
  fix: source-only `django/http/response.py` changes infer suite `responses`,
  execute `python3 tests/runtests.py responses -v 1`, run 32 tests, import
  Django from the active worktree, finish `all_verified`, and export a non-empty
  prediction consumed by the official harness dry-run.
- 2026-06-15: Additional non-Go Lite smoke
  `eval/results/swebench/lite-smoke-20260615-153954` ran
  `pytest-dev__pytest-5413`, `sphinx-doc__sphinx-10451`, and
  `sympy__sympy-11400`. The adapter validated three predictions and official
  harness dry-run consumed the file; two patches were non-empty and one was
  empty. Manual audit found the pytest patch likely satisfies the issue
  (`ReprFileLocation` no longer truncates at the first newline). The Sphinx run
  exposed a controller/planner convergence gap: read-only exploration had already
  identified the `modify_field_list` / `augment_descriptions_with_types`
  `*args`/`**kwargs` fix, but the planner treated the batch as another broad
  investigation and timed out without a `ChangePlan`. The SymPy run exposed
  verification-environment fragility: the local checkout hit pytest/report
  parser startup failures, so Codrax could not prove the generated patch.
- 2026-06-15: Hardened both gaps generically. Pytest verification now has a
  plugin-free verbose text fallback after JSON/report parser failures; it
  accepts typed pass/fail only from case-level rows and leaves startup/collection
  failures as `parser_error` unverified, preserving patch export in imperfect
  customer environments. Completed exploration attempts are persisted on the
  workflow batch, exploration questions are carried as their own context-pack
  item rather than unresolved unknowns, and the subsequent planner dispatch gets
  an exploration-convergence hint plus a bounded soft cap so it synthesizes the
  smallest ChangePlan instead of restarting broad exploration.
- 2026-06-15: Re-ran the pytest Lite case at
  `eval/results/swebench/lite-smoke-20260615-161816`. The first planner round
  produced no plan, the typed-anchor no-plan retry fired once, the retry emitted
  a bounded `ChangePlan`, apply succeeded, local pytest verification remained
  unverified due to parser/startup failure, and the adapter exported a non-empty
  patch consumed by the official harness dry-run.
- 2026-06-15: Re-ran the Sphinx Lite case at
  `eval/results/swebench/lite-smoke-20260615-162411`. The run still exported an
  empty patch, but the logs isolated a stronger system gap: explorer had emitted
  four grounded evidence items for `sphinx/ext/autodoc/typehints.py` before the
  write-mode wall-time cancel, yet `runWriteExplorationSubflow` returned before
  projecting those typed artifacts into `WriteExplorationHandoff`; the controller
  recorded `exploration_degraded` and lost the chance to plan from useful
  evidence.
- 2026-06-15: Fixed the degraded-exploration evidence loss generically. The
  write exploration subflow now projects typed artifacts before returning
  errors; when TurnA handoff is unavailable, it constructs a minimal
  `TurnAArtifacts` snapshot from `Mutable.EmittedEvidence`, `BusContext`
  evidence, and `EvidenceClosure.CanonicalReadFiles`. The controller preserves
  the attempt as `degraded` but advances the batch to `ready_to_plan` when the
  resulting handoff has planning material, and the same bounded planner
  convergence cap applies to usable degraded handoffs. Targeted tests cover
  fallback projection, degraded-handoff controller continuation, and complete vs
  degraded status semantics.
- 2026-06-15: Re-ran Sphinx after the degraded-handoff fix at
  `eval/results/swebench/lite-smoke-20260615-164323`. The adapter exported a
  non-empty 2978-byte patch and official harness dry-run consumed it, proving
  the controller no longer loses useful exploration/planning context. Manual
  audit found a new quality gap: the generated Python patch was exported even
  though local verification ended `tests_failed`, and the patch form inserted a
  helper inside the `setup()` return dict shape. Root cause: Python dry-build
  already ran `py_compile` for full-content create/modify changes but skipped
  `kind=patch`, so patch hunks could bypass cheap syntax validation.
- 2026-06-15: Fixed the Python patch validation gap generically. The shared
  `stageOverlay` now applies `kind=patch` changes into the scratch tree before
  dry-build, and `dryBuildPython` includes `.py` patch targets in
  `py_compile`. Focused tests cover both invalid and valid Python patch hunks,
  preserving the principle that missing pytest is not a hard gate while
  syntactically invalid Python remains a typed plan-time rejection.
- 2026-06-15: Ran another non-Go Lite smoke with
  `matplotlib__matplotlib-18869`, `scikit-learn__scikit-learn-10297`, and
  `pylint-dev__pylint-5859` at
  `eval/results/swebench/lite-smoke-20260615-nongo-0903`. Matplotlib and
  Pylint exported non-empty predictions; the scikit-learn clone was manually
  interrupted during the large mirror fetch, exposing an eval-infra progress
  visibility gap for huge repositories. The exported `predictions.jsonl`
  validated locally with 2 predictions and 0 empty patches, and the official
  SWE-bench harness dry-run accepted the file. Manual patch audit found the
  Matplotlib prediction plausibly correct but locally unverified because the
  checkout environment shadowed stdlib modules during pytest startup. The
  Pylint prediction was semantically wrong: it emitted `(?<!\w)` where the
  intended behavioural boundary is `(?!\w)`, so punctuation-only note tags pass
  but word note tags like `YES:` fail.
- 2026-06-15: Root-cause fixes from that smoke were landed generically rather
  than as case patches. `BaseAgent` now performs a post-tool terminal-stop
  check after successful write-mode structured emit tools
  (`emit_change_plan`, `emit_plan_change`, `emit_test_results`,
  `emit_write_analysis`, `emit_write_workflow_decision`), preventing the extra
  LLM round that previously let the planner re-open investigation and
  duplicate/rewrite a successful `emit_change_plan`. The guard is intentionally
  scoped to write-mode terminal emit tools so read-mode evidence gathering is
  not retimed.
- 2026-06-15: Added typed `ChangePlan.verification_probes[]` and a bounded
  verify executor for environment-imperfect cases. Planner may emit explicit
  Python inline probes with repo-relative `working_dir`, short timeout, and
  optional exact stdout fragments. `run_tests` consumes only this typed field,
  never `acceptance_tests` prose, and runs probes on `no_tests`,
  `runner_missing`, and `parser_error` dead-ends before falling back to
  unverified. Probe pass/fail is persisted in `ChangeReport.TestResults` and
  `ExecutedCommands`, so controller/handoff consume typed verdicts while
  missing pytest remains an unverified environment caveat instead of a hard
  code-failure gate.
- 2026-06-15: Tightened unverified terminal semantics. Active batches already
  completed with typed `runner_missing`, `parser_error`, or `no_tests`
  verifier outcomes and no verify-failure handoff now normalize same-batch
  `verify_batch`, `ask_user`, `block`, and `replan_batch` decisions to
  `finish` with `finish_disposition=accept_unverified`. This prevents the
  controller from restarting verification/planning over unchanged bytes after a
  local infrastructure dead-end.
- 2026-06-15: Re-ran `pylint-dev__pylint-5859` under
  `eval/results/swebench/lite-smoke-20260615-pylint-probe-0946` after the
  terminal-stop/probe changes. The exported patch switched to the correct
  forward boundary shape `(?![a-zA-Z0-9_])` and official harness dry-run
  accepted the one-row predictions file. The run still ended locally as
  `verify_failed` because the model-authored inline probe contradicted the
  issue behaviour: it tested an isolated copied regex and expected `YES:` to
  fail while the actual defect requires word tags like `YES:` to pass. That is
  now recorded as a handoff/evidence problem, not a case-specific Pylint
  problem.
- 2026-06-15: Generalized the follow-up fix. `VerifyFailureHandoff` now carries
  resolved `read_file` paths for failed-attempt patch and test-surface
  artifacts in addition to the durable short refs, including resume rebuilds.
  The planner retry prompt renders those paths so replan can inspect the exact
  applied diff/test surface instead of guessing the plan directory. Verification
  probe raw output now includes language, working directory, timeout, expected
  stdout, and a bounded source snippet, so a retry can distinguish bad code from
  a bad inline probe without parsing prose. Planner soft guidance now asks
  probes to exercise the changed code and the external behaviour directly; hard
  gates still consume only typed fields.
- 2026-06-15: Final adapter smoke validation passed for
  `eval/results/swebench/lite-smoke-20260615-pylint-probe-0946/predictions.jsonl`:
  local validator reported 1 prediction and 0 empty patches, and
  `DRY_RUN=1 eval/swebench/run_official_harness.sh` accepted the official
  harness command.
- 2026-06-15: Ran another non-Go Lite smoke under
  `eval/results/swebench/lite-smoke-20260615-astropy-django-pytest-212327`
  for `astropy__astropy-12907`, `django__django-11039`, and
  `pytest-dev__pytest-11143`. Astropy exported a plausible unverified patch;
  Django paused at `pending_approval` because a root `tests/migrations/...`
  fixture was over-classified as production schema risk; Pytest produced an
  empty patch because degraded `write_analyze` fallback left the stage in an
  error state. The predictions JSONL still passed local validation and official
  harness dry-run consumption, proving the adapter remained structurally valid
  while surfacing write-mode gaps.
- 2026-06-15: Fixed those gaps generically and reran the affected Django/Pytest
  instances under
  `eval/results/swebench/lite-smoke-20260615-django-pytest-after-fix-213847`.
  Both exported non-empty predictions (`empty_patch=0`), local validation
  passed, and official harness dry-run accepted the file. A follow-up verifier
  gap was found and fixed: structured Python `framework=django|pytest|unittest`
  now implies `runner=python` when no runner is supplied, so scoped Django
  suites bypass unrelated manifest auto-detect lanes without parsing prose.
- 2026-06-15: Ran a fresh issue-shaped non-Go Lite batch under
  `eval/results/swebench/lite-smoke-20260615-seaborn-xarray-pytest-sympy-220257`
  for `mwaskom__seaborn-3010`, `pydata__xarray-5131`,
  `pytest-dev__pytest-7168`, and `sympy__sympy-12454`. All four predictions
  were structurally valid and official harness dry-run consumed the combined
  file, but manual audit exposed two generalized quality/infra gaps:
  Seaborn contained an adjacent duplicated four-line insertion block, and
  Xarray/SymPy local verification was unavailable due to checkout environment
  compatibility (`pkg_resources` missing after newest setuptools, and old SymPy
  importing `collections.Mapping` under Python 3.11). The controller correctly
  normalized parser-error follow-up actions to `finish(accept_unverified)`.
- 2026-06-15: Landed the generalized follow-up. `emit_change_plan` now shares a
  structural source-patch quality gate with the skeleton/change path: adjacent
  exact duplicate inserted source blocks of 3+ lines are rejected before a plan
  is installed, with the rejection returned as a typed tool result for bounded
  correction. This consumes only unified-diff structure, path extension, and
  exact line equality; it does not parse issue text, model rationale, or
  `<think>` output. The SWE-bench adapter now probes each prepared Python venv
  for `pkg_resources` and installs `setuptools<81` only when that legacy runtime
  import is missing, recording check/install/recheck steps in `env_prepare`.
  If compatibility setup still fails, the prediction export remains allowed and
  the run stays `unverified`; the official harness remains the scoring authority.
- 2026-06-15: After-fix two-case rerun under
  `eval/results/swebench/lite-smoke-20260615-dup-env-after-fix-223715`.
  Seaborn exported a smaller non-duplicated patch (`patch_bytes=536`), local
  scoped pytest passed (`projects=1 passed=true total=2 failed=0`), and the run
  ended `plan_status=applied`. Xarray exported the correct no-double-newline
  patch (`patch_bytes=1576`); `env_prepare` records `pkg_resources_available=true`
  after installing `setuptools<81`. Local verify still ended
  `parser_error/unverified`, now because old Xarray imports `np.unicode_` under
  NumPy 2, and the controller normalized the model's attempted `replan_batch`
  to `finish(accept_unverified)`. Local prediction validation reported
  `validated 2 prediction(s); empty_patch=0`, and official harness dry-run
  accepted the file.
- 2026-06-15: Ran a fresh non-Go Lite batch under
  `eval/results/swebench/lite-smoke-20260615-astropy-mpl-pytest-224631` for
  `astropy__astropy-7746`, `matplotlib__matplotlib-24149`, and
  `pytest-dev__pytest-7220`. All three exported non-empty harness-consumable
  patches and the official harness dry-run accepted the predictions file.
  Manual audit exposed two generalized local-verifier gaps: reports could carry
  legacy `passed=true` for zero-test/no-assertion runs while the workflow
  correctly marked the plan `unverified`, and the adapter did not surface the
  report verdict into `results.jsonl`. Old checkout environments also showed
  more isolation drift: Astropy's setup imported `pkg_resources` inside pip's
  isolated build env, while pytest's old runner failed before collection under
  Python 3.11.
- 2026-06-15: Landed typed local-verifier smoothing. `ChangeReport` now
  persists `verification_status=passed|failed|unavailable`; controller,
  verify-attempt classification, user-facing apply summaries, report loading,
  and SWE-bench result export consume that typed verdict instead of inferring
  terminal state from legacy `Passed` alone. SWE-bench `--prepare-python-env`
  now retries editable installs once with `--no-build-isolation` after legacy
  `pkg_resources` compatibility has been prepared. It also parses
  `pyproject.toml` and installs structured `[build-system].requires` into the
  verifier venv before editable install, so no package-name or issue-text
  special cases are needed. `results.jsonl` records `verify_status`,
  `verify_failure_kind`, `verify_summary`, no-test runners, and test count.
  Missing pytest / parser / zero-test / legacy runtime dead-ends still export
  patches and stay `unverified`; they are not hard code-failure gates.
- 2026-06-15: Re-ran `matplotlib__matplotlib-24149` after the structured
  build-requirements enhancement at
  `eval/results/swebench/lite-smoke-20260615-buildreq-after-fix-231957`.
  Prediction validation passed, official harness dry-run accepted the patch,
  and `results.jsonl` recorded `patch_bytes=494`, `plan_status=unverified`,
  `verify_status=unavailable`, `verify_failure_kind=parser_error`. Env prep
  recorded `[build-system].requires` values
  (`certifi>=2020.06.20`, `numpy>=1.19`, `setuptools_scm>=7`) and successful
  `install_pyproject_build_requires`, while editable/native build remained
  partial. This keeps local sandbox confidence best-effort without blocking
  delivery.
- 2026-06-15: Ran a new cross-repo Lite batch under
  `eval/results/swebench/lite-smoke-20260615-crossrepo-gap-232726` for
  `django__django-10914`, `pydata__xarray-3364`,
  `scikit-learn__scikit-learn-10297`, and `sympy__sympy-11870`. Prediction
  validation passed and official harness dry-run accepted the combined file
  (`validated 4 prediction(s); empty_patch=1`). Manual audit found:
  - Django and Xarray patches included repository test/spec path changes, which
    are unnecessary for SWE-bench hidden-test scoring and make patch quality
    harder to audit.
  - Scikit-learn's legacy `setup.py` declared `install_requires` with
    `numpy>=...` / `scipy>=...`, but the adapter only parsed `pyproject.toml`,
    leaving local verification unable to import `numpy`.
  - Scikit-learn and SymPy both triggered repeated read-mode repairs because
    `scope=negative` with `negative_scope=range` still scanned the whole file,
    so an absence claim bounded to a docstring/function range failed when the
    pattern appeared elsewhere in the same file.
- 2026-06-15: Landed generalized follow-up:
  - `scope=negative` + `negative_scope=range` now scans only the typed
    `line_start`/`line_end` bounds instead of the whole file. Hard gating still
    reads only typed file/path/range/pattern fields.
  - SWE-bench env prep parses `setup.py` with Python AST and installs
    structured `install_requires` / `setup_requires` entries best-effort before
    editable install.
  - SWE-bench prediction export strips repository test/spec path diffs by
    default and records them in `dropped_test_patch_paths`; `--include-test-patches`
    keeps the old debug behavior when explicitly requested.
- 2026-06-16: Re-ran `scikit-learn__scikit-learn-10297` after the range/env
  follow-up at
  `eval/results/swebench/lite-smoke-20260616-scikit-range-setup-after-fix-000555`.
  Prediction validation passed, official harness dry-run accepted the patch,
  and env prep recorded `setup_declared_requires=['numpy>=1.8.2',
  'scipy>=0.13.3']` plus successful `install_setup_declared_requires`. The
  exported patch stayed source-only (`dropped_test_patch_paths=[]`,
  `patch_bytes=1555`). Local verify remained `unavailable` because the scoped
  pytest surface returned `no_tests_runners=['python']`, now exported as
  `verify_failure_kind=no_tests` for audit clarity.
- 2026-06-16: Added stable SWE-bench environment telemetry after multi-instance
  audit showed nested env-prep fields were hard to consume consistently. The
  adapter now finalizes `env_prepare` with observational fields
  `success`, `env_available`, `failure_kind`, `pytest_available`,
  `pytest_json_report_available`, `import_probe_ok`, `import_roots`,
  `venv_python`, `failed_step_names`, and `hard_gate=false`, and mirrors the
  same data into top-level `results.jsonl` fields prefixed with
  `env_prepare_`. This preserves the principle that missing pytest or partial
  dependency setup is not a code-failure hard gate while giving eval dashboards
  stable typed signals for environment quality.
- 2026-06-16: Manual audit of the
  `lite-smoke-20260616-astropy-django-pytest-sympy-024926` Django run exposed
  a generalized handoff-coverage gap: the priority context pack carried P1
  model-layer and form-layer targets, but the emitted plan changed only the form
  layer and the bounded probe validated only that layer, so the controller
  accepted a locally green but likely incomplete patch. Landed two non-hard-gate
  mitigations: planner context-pack rendering now includes soft coverage
  guidance for P0/P1 scope/target/evidence rows, and the SWE-bench adapter
  mirrors persisted workflow/context-pack coverage into
  `plan_context_paths`, `plan_context_covered_paths`,
  `plan_context_uncovered_paths`, and `plan_context_coverage_ratio` when a
  durable workflow is available. These fields are typed eval telemetry only;
  ChangePlan validation, approval, and verifier verdicts remain the hard
  decision artifacts.
- 2026-06-16: Ran the targeted Django/pytest context batch at
  `eval/results/swebench/lite-smoke-20260616-django-pytest-context-032114`.
  Django reproduced a planner convergence gap: after `emit_change_plan` was
  rejected by the structured line-edit builder for an exact `old_text`
  mismatch, the planner correctly attempted to `read_file` the current bytes
  but hit the soft iteration cap before that read-only repair step could run,
  so the adapter exported an empty patch. The fix is scheduler-generic: planner
  now observes typed failed structured emit ToolResults and opens a bounded
  soft-to-hard-cap repair window for read-only diagnostic tools followed by a
  corrected emit. It does not parse validator prose, does not relax the
  validator, and the hard cap still stops non-converging loops. The same batch
  also confirmed that current CLI single-shot artifacts did not expose a
  durable workflow JSON for context-coverage telemetry (`workflow_path=""`);
  that remains a separate persistence-chain follow-up rather than an
  apply/verify hard gate.
- 2026-06-16: The rebuilt Django single-instance regression
  `lite-smoke-20260616-django-emit-repair-033210` showed an adjacent planner
  budget gap before reaching any emit repair: after a usable typed exploration
  handoff, the planner still needed several read-only byte/line fetches to
  synthesize the patch and hit the normal soft cap at iteration 8 with no
  ChangePlan, causing the scheduler to start another planning dispatch. Landed
  a controller-only synthesis floor plus planner-side handoff synthesis window:
  when the active batch has a completed or degraded typed exploration handoff,
  the planner soft cap is raised to at least `base+6` (still bounded by
  `PlannerScaledIterMax`), and read-only synthesis tools such as `read_file`,
  `grep`, `list_files`, and `repo_map` may continue after the soft cap until
  the hard cap. This is typed-state driven, does not inspect user keywords or
  model prose, and does not affect read/log/trace/data/operation modes.
- 2026-06-16: Re-ran the Django single-instance regression after the handoff
  synthesis window at
  `eval/results/swebench/lite-smoke-20260616-django-handoff-window-034623`.
  The planner first hit the same structured `old_text` mismatch, then consumed
  typed tool failure state, re-read exact bytes inside the soft-to-hard repair
  window, emitted a corrected `ChangePlan`, applied it, and verified it as
  `passed`. The SWE-bench adapter exported a non-empty source patch
  (`patch_bytes=2136`, `status=predicted`, `plan_status=applied`,
  `verify_status=passed`) and the official harness dry-run accepted one
  prediction. This confirms the generalized fix for "emit rejected then soft
  cap stops the repair read" and "usable exploration handoff still needs
  bounded synthesis reads." The same run still had `workflow_path=""` because
  the CLI artifact set exposed plan/report JSON under the blob session but no
  durable workflow JSON; context coverage therefore remains opportunistic eval
  telemetry when persisted workflow/context-pack data exists, not an
  apply/verify hard gate.
- 2026-06-16: Ran a three-instance non-Go Lite smoke at
  `eval/results/swebench/lite-smoke-20260616-requests-pytest-mpl-035729`
  for `psf__requests-2674`, `pytest-dev__pytest-5692`, and
  `matplotlib__matplotlib-23299`. The adapter produced three non-empty
  predictions and the official harness dry-run accepted them. Manual audit
  found three systemic verifier/planner gaps: Python verification probes that
  spawn subprocesses need the active worktree source roots (`src/`, `lib/`) in
  inherited `PYTHONPATH`; unhandled verification-probe exceptions such as
  missing temporary XML files are probe/runtime failures and should be
  `parser_error/unavailable`, not product `tests_failed`; and replan no-op
  structured edits need a typed path to `run_tests(dry_run=true)` plus the
  existing `no_change_required` sentinel instead of repeated no-op patch emits.
- 2026-06-16: Landed generalized follow-up for those verifier/planner gaps:
  Python runner environments now prepend the active worktree's conventional
  source roots (`src/`, `lib/`) ahead of inherited import roots and drop the
  original main-repo root, so verification probes and their Python subprocesses
  import applied worktree code rather than installed or pre-patch code.
  Verification probes now reserve `tests_failed` for explicit
  `AssertionError`/non-zero `SystemExit`; import/syntax/unhandled exceptions
  are `FailureKindParserError` and normalize to `verification_status=unavailable`,
  allowing project runners or unverified delivery to continue. Structured
  no-op edit rejections during verify-failure replan still reject the plan, but
  now point at the typed dry-run-probe + `changes: []` sentinel path.
- 2026-06-16: Re-ran `pytest-dev__pytest-5692` at
  `eval/results/swebench/lite-smoke-20260616-pytest5692-probe-env-042524`.
  Prediction validation passed, official harness dry-run accepted the patch,
  and the exported source patch added `hostname`/`timestamp` in
  `src/_pytest/junitxml.py` (`patch_bytes=835`). Local verify remained
  `unavailable/parser_error` because the generated bounded probe could not
  parse the temporary XML artifact, but this correctly did not hard-block
  prediction export.
- 2026-06-16: A second `pytest-dev__pytest-5692` run at
  `eval/results/swebench/lite-smoke-20260616-pytest5692-probe-exception-unavailable-043223`
  exposed two more typed-contract gaps. The planner copied a typed
  `TestSurfaceCandidate.ID` (`python/pytest@.`) into `run_tests.suite`, which
  produced an invalid pytest selector. `run_tests` now rejects a suite selector
  that exactly equals a current typed candidate id and tells the model to use
  `runner` / `framework` / `working_dir` instead. The same run also showed a
  final-plan-vs-worktree drift: the final plan touched only a repository test
  file while the adapter exported production worktree diff after stripping test
  paths. This remains a follow-up for eval hardening: production-scope coverage
  and "final live plan matches exported source patch" should become typed
  telemetry/retry input, without forbidding legitimate customer requests to add
  tests.
- 2026-06-16: Added the eval telemetry side of that follow-up. Prediction export
  now records `exported_patch_paths`, `exported_patch_source_paths`,
  `exported_patch_test_paths`, `final_plan_source_paths`, `final_plan_test_only`,
  and `final_plan_covers_exported_source_patch` in `results.jsonl`. These fields
  are derived from `git diff --name-only` selected paths plus the final
  `ChangePlan`, not from model prose, and remain audit telemetry rather than a
  hard product gate. They let batch dashboards flag cases where the official
  prediction contains source changes but the final live plan drifted to test-only
  or otherwise failed to cover the exported source patch.
- 2026-06-16: Ran a three-instance non-Go Lite smoke at
  `eval/results/swebench/lite-smoke-20260616-django-sympy-sphinx-telemetry`
  for `django__django-14752`, `sphinx-doc__sphinx-8506`, and
  `sympy__sympy-13031`. The adapter produced three non-empty predictions and
  the official harness dry-run accepted them. Manual audit found one good
  verified Django patch, one Sphinx false positive where the verifier probe
  copied the proposed regex instead of importing the changed module, and one
  Sympy unverified case where the old checkout failed under the local Python
  runtime before the patch could be proven. This keeps reinforcing the product
  boundary: missing or incompatible project dependencies must remain
  `unverified`, while a locally failed behavior probe must not look equivalent
  to a verified prediction in eval telemetry.
- 2026-06-16: Landed a generalized ChangePlan hard gate for Python verification
  probes. When a plan changes Python production files and supplies Python
  probes, the probe set must include at least one import declaration that
  structurally covers a changed production module candidate derived from the
  typed `changes[].path` list. The gate runs in both `emit_change_plan` and
  final `emit_plan_change` promotion, after probe normalization and before plan
  installation. It does not inspect user intent keywords, model rationale, or
  natural-language summaries; it compares typed file paths with deterministic
  Python import declarations. Extra probes remain allowed once one coupled probe
  exercises the changed module, so CLI/smoke probes can still complement the
  behavior check without becoming the only proof.
- 2026-06-16: Re-ran `sphinx-doc__sphinx-8506` after the probe-coupling gate at
  `eval/results/swebench/lite-smoke-20260616-sphinx-probe-coupling-after-fix`.
  The first copied-regex probe was rejected, the planner retried with
  `import sphinx.domains.std as std_module`, and verify then failed on typed
  behavior evidence: `AssertionError: Wrong match for [enable=]PATTERN:
  [enable`. The controller carried that failure into P2 handoff and attempted a
  replan, but the exported patch still came from the failed local plan. The
  adapter now adds `prediction_verdict`, `prediction_local_confidence`, and
  `prediction_blocks_local_acceptance` to `results.jsonl` so a non-empty patch
  with failed local verification is auditable as `predicted_failed_verify`
  without changing the official SWE-bench predictions JSONL shape. Remaining
  follow-up: workflow export/replan convergence should prefer a later verified
  or applicable plan, or surface terminal failed workflow state more strongly in
  controller UX and batch dashboards.
- 2026-06-16: Ran a four-instance non-Go Lite smoke at
  `eval/results/swebench/lite-smoke-20260616-astropy-scikit-xarray-requests-current`
  for `astropy__astropy-6938`, `psf__requests-3362`,
  `pydata__xarray-4094`, and `scikit-learn__scikit-learn-10949`. The adapter
  produced four non-empty predictions, `validate_predictions.py` accepted all
  rows, and the official SWE-bench harness dry-run accepted the predictions
  path. Manual patch audit: Astropy assigns the `chararray.replace` return value
  back to `output_field`; Requests falls back from `r.encoding` to
  `r.apparent_encoding` before stream unicode decoding; Xarray drops the stacked
  coordinate before rebuilding the `Dataset`; Scikit-learn preserves DataFrame
  dtype before conversion so `warn_on_dtype` can fire. All four local verifies
  remained `predicted_unverified` because old-project dependency/import
  compatibility failed under the local Python runtime; this is acceptable
  delivery telemetry, not a hard code-failure gate.
- 2026-06-16: The same four-instance run exposed two smoothness gaps. First,
  three long symptom-style issues spent two failed `write_analyzer` dispatches
  before falling back to typed read-analysis anchors. The orchestrator now
  distinguishes no-emit from schema rejection: a validator rejection still gets
  one retry with an exact rejection hint, but a full no-emit round immediately
  installs fallback `WriteAnalysisIR` and lets the controller spend budget on
  code exploration. Second, the Python verification-probe coupling gate was too
  narrow for public APIs: `import xarray as xr` exercises
  `xarray.core.dataarray.DataArray.to_unstacked_dataset`, but the old gate only
  accepted direct imports of `xarray.core.dataarray`. The gate now treats an
  import declaration as coupled when it imports the changed module or a package
  prefix of that module. This still rejects isolated copied-implementation
  probes such as `import re` for a Sphinx parser change; it remains based only on
  typed target paths and import declarations, not user intent keywords or model
  prose.
- 2026-06-16: Re-ran `pydata__xarray-4094` after those fixes at
  `eval/results/swebench/lite-smoke-20260616-xarray-public-api-probe-after-fix`.
  The run produced a non-empty prediction, prediction validation passed, and the
  official harness dry-run accepted it. Log audit confirmed one fast no-emit
  fallback (`installing fallback IR; no-emit retry would repeat classification
  work`) and zero probe-coupling rejects (`public_api_false_reject=0`). The
  rerun also surfaced a separate read/exploration convergence gap: the
  exploration subflow needed 13 iterations and one downgraded completion before
  closing, despite having already located the relevant `to_unstacked_dataset`,
  `to_stacked_array`, and `MergeError` evidence. Follow-up: bounded write
  exploration should close from sufficient P1 evidence sooner and avoid generic
  same-lane widening after the controller has requested one focused batch.
- 2026-06-16 follow-up ledger: the first Xarray run also showed the planner
  calling `run_tests(dry_run=true)` without a bounded selector/probe, which
  still caused project-level pytest/unittest discovery and produced large
  unavailable output during planning. The current fix leaves run_tests semantics
  untouched; the next hardening batch should split planner dry-run into a typed
  runner/test-surface feasibility probe that cannot execute broad suites unless
  the caller supplies a bounded selector or explicit verification probe.
- 2026-06-16: Ran a three-instance non-Go Lite smoke at
  `eval/results/swebench/lite-smoke-20260616-mpl-seaborn-pylint-current` for
  `matplotlib__matplotlib-22835`, `mwaskom__seaborn-3190`, and
  `pylint-dev__pylint-7114`. All three produced non-empty predictions and the
  adapter results classified them without treating local environment/parser
  unavailability as a hard code failure. `mwaskom__seaborn-3190` verified
  locally as `passed`; Matplotlib and Pylint exported patches with
  `prediction_verdict=predicted_unverified` because old-project import/parser
  setup failed under the local environment. Manual audit found the Matplotlib
  `BoundaryNorm.inverse` fallback and Pylint module-file selection patches
  plausible, while Seaborn exposed a handoff-priority risk: the accepted patch
  fixed the boolean subtraction failure in `ContinuousBase._setup`, but the
  earlier read-only exploration had identified the more semantic public-property
  route. This keeps "handoff P1 conclusion should dominate backup plans" as a
  controller/planner quality follow-up rather than an adapter hard gate.
- 2026-06-16: The same run exposed three generalized smoothness gaps and their
  fixes. First, the adapter's default request appended SWE-bench operational
  guardrails to the public issue text; read analysis could misclassify those as
  current-request source quotes or exclusions. The default request now contains
  only the public instance id, public problem statement, and a short fix
  directive; gold redaction and test-diff stripping remain typed adapter/export
  behavior. Second, the Python probe-coupling gate accepted `import xarray` for
  a changed submodule but still rejected sibling public APIs such as
  `import seaborn.objects` exercising `seaborn._core.scales`; coupling now
  accepts exact-module, package-prefix, or same-top-level-package imports while
  still rejecting unrelated imports such as copied `re` probes. Third, Pylint
  showed the verifier could call `run_tests` again after an unavailable
  parser/error report. The verifier now stops immediately after `run_tests`
  installs a typed passed or unavailable `ChangeReport`, and filters `run_tests`
  out of subsequent turns once any report exists.
