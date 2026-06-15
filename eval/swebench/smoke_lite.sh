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
MAX_STEPS="${MAX_STEPS:-50}"
CODRAX_TIMEOUT="${CODRAX_TIMEOUT:-1800}"
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

instance_args=()
if [[ -n "$INSTANCE_ID" ]]; then
  instance_args+=(--instance-id "$INSTANCE_ID")
fi
if [[ -n "$INSTANCE_IDS_FILE" ]]; then
  instance_args+=(--instance-ids-file "$INSTANCE_IDS_FILE")
fi

"$PYTHON" "$ROOT/eval/swebench/run_codrax_swebench.py" \
  --dataset-name "$DATASET_NAME" \
  --split "$SPLIT" \
  --limit "$SWEBENCH_SMOKE_LIMIT" \
  "${instance_args[@]}" \
  --workdir "$WORKDIR" \
  --predictions-path "$PREDICTIONS_PATH" \
  --results-path "$RESULTS_PATH" \
  --codrax-bin "$CODRAX_BIN" \
  --max-steps "$MAX_STEPS" \
  --codrax-timeout "$CODRAX_TIMEOUT"

"$PYTHON" "$ROOT/eval/swebench/validate_predictions.py" "$PREDICTIONS_PATH"

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
    PYTHON="$PYTHON" \
    "$ROOT/eval/swebench/run_official_harness.sh"
fi

echo "SWE-bench Lite smoke complete"
echo "predictions: $PREDICTIONS_PATH"
echo "results: $RESULTS_PATH"
