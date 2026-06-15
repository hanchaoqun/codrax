#!/usr/bin/env bash
set -euo pipefail

# Thin wrapper around the official SWE-bench harness. The adapter generates the
# predictions JSONL; this script delegates scoring to SWE-bench without trying
# to reinterpret harness results.

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PYTHON="${PYTHON:-python3}"

DATASET_NAME="${DATASET_NAME:-SWE-bench/SWE-bench_Lite}"
SPLIT="${SPLIT:-test}"
PREDICTIONS_PATH="${PREDICTIONS_PATH:-${1:-}}"
MAX_WORKERS="${MAX_WORKERS:-1}"
RUN_ID="${RUN_ID:-codrax-swebench-smoke}"
DRY_RUN="${DRY_RUN:-0}"

if [[ -z "$PREDICTIONS_PATH" ]]; then
  echo "usage: PREDICTIONS_PATH=/path/to/predictions.jsonl $0" >&2
  exit 2
fi
if [[ ! -f "$PREDICTIONS_PATH" ]]; then
  echo "predictions file not found: $PREDICTIONS_PATH" >&2
  exit 2
fi

"$PYTHON" "$ROOT/eval/swebench/validate_predictions.py" "$PREDICTIONS_PATH"

cmd=(
  "$PYTHON" -m swebench.harness.run_evaluation
  --dataset_name "$DATASET_NAME"
  --split "$SPLIT"
  --predictions_path "$PREDICTIONS_PATH"
  --max_workers "$MAX_WORKERS"
  --run_id "$RUN_ID"
)

if [[ "$DRY_RUN" == "1" ]]; then
  printf 'official harness command:'
  printf ' %q' "${cmd[@]}"
  printf '\n'
  exit 0
fi

if ! "$PYTHON" - <<'PY'
import importlib.util
raise SystemExit(0 if importlib.util.find_spec("swebench") else 1)
PY
then
  echo "Python package 'swebench' is not installed. Install the official SWE-bench harness before scoring." >&2
  exit 3
fi

exec "${cmd[@]}"
