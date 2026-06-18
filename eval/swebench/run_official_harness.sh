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
REPORT_DIR="${REPORT_DIR:-.}"
CHECK_HARNESS_IMPORT="${CHECK_HARNESS_IMPORT:-0}"

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
  --report_dir "$REPORT_DIR"
)

check_harness_import() {
  "$PYTHON" - <<'PY'
import platform
import sys

try:
    import swebench.harness.run_evaluation  # noqa: F401
except Exception as exc:
    print(
        "Official SWE-bench harness import failed for "
        f"{sys.executable} (Python {platform.python_version()}): "
        f"{type(exc).__name__}: {exc}",
        file=sys.stderr,
    )
    print(
        "Install the official SWE-bench package in a Python 3.10+ environment; "
        "see eval/swebench/README.md.",
        file=sys.stderr,
    )
    raise SystemExit(1)
PY
}

if [[ "$DRY_RUN" == "1" ]]; then
  if [[ "$CHECK_HARNESS_IMPORT" == "1" ]]; then
    check_harness_import
  fi
  printf 'official harness command:'
  printf ' %q' "${cmd[@]}"
  printf '\n'
  exit 0
fi

if ! check_harness_import
then
  exit 3
fi

exec "${cmd[@]}"
