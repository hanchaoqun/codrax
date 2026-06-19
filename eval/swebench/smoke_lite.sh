#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VENV="${SWEBENCH_VENV:-$ROOT/eval/results/swebench/.venv}"
PYTHON="${PYTHON:-$VENV/bin/python}"

DATASET_NAME="${DATASET_NAME:-SWE-bench/SWE-bench_Lite}"
SPLIT="${SPLIT:-test}"
SWEBENCH_SMOKE_LIMIT="${SWEBENCH_SMOKE_LIMIT:-1}"
WORKDIR="${WORKDIR:-$ROOT/eval/results/swebench/lite-smoke-$(date +%Y%m%d-%H%M%S)}"
PREDICTIONS_PATH="${PREDICTIONS_PATH:-$WORKDIR/predictions.jsonl}"
RESULTS_PATH="${RESULTS_PATH:-$WORKDIR/results.jsonl}"
CODRAX_BIN="${CODRAX_BIN:-$ROOT/codrax}"
PROVIDERS_PATH="${PROVIDERS_PATH:-${CODRAX_PROVIDERS:-}}"
MAX_STEPS="${MAX_STEPS:-50}"
CODRAX_TIMEOUT="${CODRAX_TIMEOUT:-1800}"
SWEBENCH_PREPARE_PYTHON_ENV="${SWEBENCH_PREPARE_PYTHON_ENV:-1}"
SWEBENCH_ENV_PREPARE_TIMEOUT="${SWEBENCH_ENV_PREPARE_TIMEOUT:-600}"
SWEBENCH_ISOLATE_GIT_HISTORY="${SWEBENCH_ISOLATE_GIT_HISTORY:-1}"
SWEBENCH_FAIL_ON_INSTANCE_ERROR="${SWEBENCH_FAIL_ON_INSTANCE_ERROR:-1}"
SWEBENCH_FAIL_ON_EMPTY_PATCH="${SWEBENCH_FAIL_ON_EMPTY_PATCH:-1}"
SWEBENCH_REQUIRE_NONEMPTY_PATCH="${SWEBENCH_REQUIRE_NONEMPTY_PATCH:-1}"
SWEBENCH_CHECK_OFFICIAL_IMPORT="${SWEBENCH_CHECK_OFFICIAL_IMPORT:-1}"
MAX_WORKERS="${MAX_WORKERS:-1}"
SWEBENCH_RUN_OFFICIAL="${SWEBENCH_RUN_OFFICIAL:-0}"
INSTANCE_ID="${INSTANCE_ID:-}"
INSTANCE_IDS_FILE="${INSTANCE_IDS_FILE:-}"

if [[ ! -x "$PYTHON" ]]; then
  echo "SWE-bench Python not found: $PYTHON" >&2
  echo "Create it with: python3 -m venv $VENV && $VENV/bin/python -m pip install swebench datasets" >&2
  exit 2
fi
if [[ ! -x "$CODRAX_BIN" ]]; then
  echo "Codrax binary not found: $CODRAX_BIN" >&2
  echo "Run 'make' or set CODRAX_BIN=/path/to/codrax." >&2
  exit 2
fi
if [[ -z "$PROVIDERS_PATH" && -f "$ROOT/providers.yaml" ]]; then
  PROVIDERS_PATH="$ROOT/providers.yaml"
fi
if [[ -n "$PROVIDERS_PATH" && ! -f "$PROVIDERS_PATH" ]]; then
  echo "Providers config not found: $PROVIDERS_PATH" >&2
  exit 2
fi

env_prepare_args=(--env-prepare-timeout "$SWEBENCH_ENV_PREPARE_TIMEOUT")
if [[ "$SWEBENCH_PREPARE_PYTHON_ENV" == "1" ]]; then
  env_prepare_args+=(--prepare-python-env)
fi

cmd=(
  "$PYTHON" "$ROOT/eval/swebench/run_codrax_swebench.py"
  --dataset-name "$DATASET_NAME"
  --split "$SPLIT"
  --limit "$SWEBENCH_SMOKE_LIMIT"
  --workdir "$WORKDIR"
  --predictions-path "$PREDICTIONS_PATH"
  --results-path "$RESULTS_PATH"
  --codrax-bin "$CODRAX_BIN"
  --max-steps "$MAX_STEPS"
  --codrax-timeout "$CODRAX_TIMEOUT"
)
if [[ -n "$PROVIDERS_PATH" ]]; then
  cmd+=(--providers "$PROVIDERS_PATH")
fi
if [[ -n "$INSTANCE_ID" ]]; then
  cmd+=(--instance-id "$INSTANCE_ID")
fi
if [[ -n "$INSTANCE_IDS_FILE" ]]; then
  cmd+=(--instance-ids-file "$INSTANCE_IDS_FILE")
fi
cmd+=("${env_prepare_args[@]}")
if [[ "$SWEBENCH_ISOLATE_GIT_HISTORY" == "1" ]]; then
  cmd+=(--isolate-git-history)
fi
if [[ "$SWEBENCH_FAIL_ON_INSTANCE_ERROR" == "1" ]]; then
  cmd+=(--fail-on-instance-error)
fi
if [[ "$SWEBENCH_FAIL_ON_EMPTY_PATCH" == "1" ]]; then
  cmd+=(--fail-on-empty-patch)
fi

"${cmd[@]}"

validate_args=("$PREDICTIONS_PATH")
validate_args+=(--results-jsonl "$RESULTS_PATH")
if [[ "$SWEBENCH_REQUIRE_NONEMPTY_PATCH" == "1" ]]; then
  validate_args+=(--require-nonempty-patch)
fi
if [[ "$SWEBENCH_REQUIRE_FINAL_REPORT_AUDIT" == "1" ]]; then
  validate_args+=(--require-final-report-audit)
fi
"$PYTHON" "$ROOT/eval/swebench/validate_predictions.py" "${validate_args[@]}"

if [[ "$SWEBENCH_RUN_OFFICIAL" == "1" ]]; then
  DATASET_NAME="$DATASET_NAME" \
    SPLIT="$SPLIT" \
    PREDICTIONS_PATH="$PREDICTIONS_PATH" \
    MAX_WORKERS="$MAX_WORKERS" \
    RUN_ID="${RUN_ID:-codrax-swebench-lite-smoke}" \
    PYTHON="$PYTHON" \
    "$ROOT/eval/swebench/run_official_harness.sh"
else
  DATASET_NAME="$DATASET_NAME" \
    SPLIT="$SPLIT" \
    PREDICTIONS_PATH="$PREDICTIONS_PATH" \
    MAX_WORKERS="$MAX_WORKERS" \
    RUN_ID="${RUN_ID:-codrax-swebench-lite-smoke}" \
    DRY_RUN=1 \
    CHECK_HARNESS_IMPORT="$SWEBENCH_CHECK_OFFICIAL_IMPORT" \
    PYTHON="$PYTHON" \
    "$ROOT/eval/swebench/run_official_harness.sh"
fi

echo "SWE-bench Lite smoke complete"
echo "predictions: $PREDICTIONS_PATH"
echo "results: $RESULTS_PATH"
