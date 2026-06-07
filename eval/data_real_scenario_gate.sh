#!/usr/bin/env bash
#
# Opt-in real-scenario data-lane gate.
#
# This script is intentionally not part of the default eval sweep. It is for
# customer-like local data directories that are too large or private to live in
# eval/fixtures. It keeps stdout reserved for the final data answer and writes
# progress/log inspection to stderr.
#
# Required:
#   DATA_REAL_SCENARIO_DIR       directory to run codrax from
#   DATA_REAL_SCENARIO_REQUEST   request text, or a path to a request file
#
# Optional:
#   DATA_REAL_SCENARIO_EXPECTED  exact final stdout after whitespace trim
#   DATA_REAL_SCENARIO_TIMEOUT   seconds, default 1800
#   CODRAX_BIN                   binary path, default ./codrax
#   CODRAX_SETTINGS              settings path passed through to codrax

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CODRAX_BIN="${CODRAX_BIN:-$ROOT/codrax}"
SCENARIO_DIR="${DATA_REAL_SCENARIO_DIR:-}"
REQUEST_SOURCE="${DATA_REAL_SCENARIO_REQUEST:-}"
EXPECTED="${DATA_REAL_SCENARIO_EXPECTED:-}"
TIMEOUT_SECONDS="${DATA_REAL_SCENARIO_TIMEOUT:-1800}"

if [[ -z "$SCENARIO_DIR" || -z "$REQUEST_SOURCE" ]]; then
  echo "usage: DATA_REAL_SCENARIO_DIR=<dir> DATA_REAL_SCENARIO_REQUEST=<text-or-file> $0" >&2
  exit 2
fi
if [[ ! -d "$SCENARIO_DIR" ]]; then
  echo "DATA_REAL_SCENARIO_DIR is not a directory: $SCENARIO_DIR" >&2
  exit 2
fi
if [[ ! -x "$CODRAX_BIN" ]]; then
  echo "CODRAX_BIN is not executable: $CODRAX_BIN" >&2
  exit 2
fi

if [[ -f "$REQUEST_SOURCE" ]]; then
  REQUEST_TEXT="$(cat "$REQUEST_SOURCE")"
else
  REQUEST_TEXT="$REQUEST_SOURCE"
fi
if [[ -z "$(printf '%s' "$REQUEST_TEXT" | tr -d '[:space:]')" ]]; then
  echo "DATA_REAL_SCENARIO_REQUEST is empty" >&2
  exit 2
fi

RUN_ID="$(date +%Y%m%d-%H%M%S)"
RUN_ROOT="$SCENARIO_DIR/.codrax/real-scenario-gates"
mkdir -p "$RUN_ROOT"
OUT="$RUN_ROOT/$RUN_ID.out"
ERR="$RUN_ROOT/$RUN_ID.err"
REQ="$RUN_ROOT/$RUN_ID.request.txt"
printf '%s' "$REQUEST_TEXT" >"$REQ"

echo "[data-real-gate] run_id=$RUN_ID dir=$SCENARIO_DIR" >&2
echo "[data-real-gate] request=$REQ" >&2

run_cmd=("$CODRAX_BIN" --mode=data --request "$REQUEST_TEXT")
if command -v timeout >/dev/null 2>&1; then
  (
    cd "$SCENARIO_DIR"
    timeout "$TIMEOUT_SECONDS" "${run_cmd[@]}" >"$OUT" 2>"$ERR"
  )
else
  (
    cd "$SCENARIO_DIR"
    "${run_cmd[@]}" >"$OUT" 2>"$ERR"
  )
fi

ANSWER="$(sed -e 's/[[:space:]]*$//' "$OUT" | sed -e ':a' -e '/^$/{$d;N;ba' -e '}')"
if [[ -z "$(printf '%s' "$ANSWER" | tr -d '[:space:]')" ]]; then
  echo "[data-real-gate] FAIL empty stdout" >&2
  echo "[data-real-gate] stdout=$OUT stderr=$ERR" >&2
  exit 1
fi
if [[ -n "$EXPECTED" && "$(printf '%s' "$ANSWER" | tr -d '\n\r\t ')" != "$(printf '%s' "$EXPECTED" | tr -d '\n\r\t ')" ]]; then
  echo "[data-real-gate] FAIL expected mismatch" >&2
  echo "[data-real-gate] expected=$EXPECTED" >&2
  echo "[data-real-gate] actual=$ANSWER" >&2
  echo "[data-real-gate] stdout=$OUT stderr=$ERR" >&2
  exit 1
fi

if ! grep -aq '\[cli/data\] terminal full path=.*terminal\.json' "$ERR"; then
  echo "[data-real-gate] FAIL missing terminal audit path in stderr/log stream" >&2
  echo "[data-real-gate] stdout=$OUT stderr=$ERR" >&2
  exit 1
fi
if ! grep -aq '\[cli/data\] data task result.*contributions=[1-9]' "$ERR"; then
  echo "[data-real-gate] FAIL missing non-empty contribution ledger signal" >&2
  echo "[data-real-gate] stdout=$OUT stderr=$ERR" >&2
  exit 1
fi
if ! grep -aq '\[cli/data\] data task result.*reconcile=pass' "$ERR"; then
  echo "[data-real-gate] FAIL missing reconcile=pass signal" >&2
  echo "[data-real-gate] stdout=$OUT stderr=$ERR" >&2
  exit 1
fi

echo "[data-real-gate] PASS answer=$ANSWER" >&2
echo "[data-real-gate] stdout=$OUT stderr=$ERR" >&2
printf '%s\n' "$ANSWER"
