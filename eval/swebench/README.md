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
`setuptools<81` and records the check/recheck steps. Environment setup failures
never block prediction export; they leave the run unverified and the official
harness remains the scoring authority. Codrax's Python verifier also recognizes
Django source trees with `tests/runtests.py` and uses that typed harness instead
of plain pytest when the repository advertises it through file structure. During
worktree verification, Codrax prepends the active worktree to `PYTHONPATH` so
editable installs or inherited environments cannot accidentally import the
original checkout. When a Django suite is not explicitly supplied, the verifier
derives a conservative scoped suite from typed ChangePlan paths and the
repository `tests/` tree before falling back to a wider run.

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
