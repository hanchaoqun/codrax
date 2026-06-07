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
#   DATA_REAL_SCENARIO_RUNS      repeat count for volatility checks, default 1
#   DATA_REAL_SCENARIO_TIMEOUT   seconds, default 1800
#   DATA_REAL_SCENARIO_REQUIRE_LEDGER     require contribution signal, default 1
#   DATA_REAL_SCENARIO_REQUIRE_RECONCILE  require reconcile=pass signal, default 1
#   CODRAX_BIN                   binary path, default ./codrax
#   CODRAX_SETTINGS              settings path passed through to codrax

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CODRAX_BIN="${CODRAX_BIN:-$ROOT/codrax}"
SCENARIO_DIR="${DATA_REAL_SCENARIO_DIR:-}"
REQUEST_SOURCE="${DATA_REAL_SCENARIO_REQUEST:-}"
EXPECTED="${DATA_REAL_SCENARIO_EXPECTED:-}"
RUNS="${DATA_REAL_SCENARIO_RUNS:-1}"
TIMEOUT_SECONDS="${DATA_REAL_SCENARIO_TIMEOUT:-1800}"
REQUIRE_LEDGER="${DATA_REAL_SCENARIO_REQUIRE_LEDGER:-1}"
REQUIRE_RECONCILE="${DATA_REAL_SCENARIO_REQUIRE_RECONCILE:-1}"

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
if ! [[ "$RUNS" =~ ^[0-9]+$ ]] || [[ "$RUNS" -lt 1 ]]; then
  echo "DATA_REAL_SCENARIO_RUNS must be a positive integer: $RUNS" >&2
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

BASE_RUN_ID="$(date +%Y%m%d-%H%M%S)"
RUN_ROOT="$SCENARIO_DIR/.codrax/real-scenario-gates"
mkdir -p "$RUN_ROOT"
REQ="$RUN_ROOT/$BASE_RUN_ID.request.txt"
SUMMARY="$RUN_ROOT/$BASE_RUN_ID.summary.tsv"
printf '%s' "$REQUEST_TEXT" >"$REQ"
printf 'run\tanswer\tstdout\tstderr\tterminal_audit\n' >"$SUMMARY"

echo "[data-real-gate] run_id=$BASE_RUN_ID runs=$RUNS dir=$SCENARIO_DIR" >&2
echo "[data-real-gate] request=$REQ" >&2

normalize_answer() {
  printf '%s' "$1" | tr -d '\n\r\t '
}

truthy() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes|on) return 0 ;;
    *) return 1 ;;
  esac
}

run_once() {
  local run="$1"
  local run_id="${BASE_RUN_ID}-r${run}"
  local out="$RUN_ROOT/$run_id.out"
  local err="$RUN_ROOT/$run_id.err"
  local run_cmd=("$CODRAX_BIN" --mode=data --request "$REQUEST_TEXT")
  local rc

  if command -v timeout >/dev/null 2>&1; then
    set +e
    (
      cd "$SCENARIO_DIR"
      timeout "$TIMEOUT_SECONDS" "${run_cmd[@]}" >"$out" 2>"$err"
    )
    rc=$?
    set -e
  elif command -v gtimeout >/dev/null 2>&1; then
    set +e
    (
      cd "$SCENARIO_DIR"
      gtimeout "$TIMEOUT_SECONDS" "${run_cmd[@]}" >"$out" 2>"$err"
    )
    rc=$?
    set -e
  else
    set +e
    (
      cd "$SCENARIO_DIR"
      "${run_cmd[@]}" >"$out" 2>"$err"
    )
    rc=$?
    set -e
  fi
  if [[ "$rc" -ne 0 ]]; then
    echo "[data-real-gate] FAIL run=$run command exited rc=$rc" >&2
    echo "[data-real-gate] stdout=$out stderr=$err" >&2
    exit 1
  fi

  local answer
  answer="$(sed -e 's/[[:space:]]*$//' "$out" | sed -e ':a' -e '/^$/{$d;N;ba' -e '}')"
  if [[ -z "$(printf '%s' "$answer" | tr -d '[:space:]')" ]]; then
    echo "[data-real-gate] FAIL run=$run empty stdout" >&2
    echo "[data-real-gate] stdout=$out stderr=$err" >&2
    exit 1
  fi
  if [[ -n "$EXPECTED" && "$(normalize_answer "$answer")" != "$(normalize_answer "$EXPECTED")" ]]; then
    echo "[data-real-gate] FAIL run=$run expected mismatch" >&2
    echo "[data-real-gate] expected=$EXPECTED" >&2
    echo "[data-real-gate] actual=$answer" >&2
    echo "[data-real-gate] stdout=$out stderr=$err" >&2
    exit 1
  fi

  local terminal_path
  terminal_path="$(sed -n 's/.*\[cli\/data\] terminal full path=//p' "$err" | tail -n 1)"
  if [[ -z "$terminal_path" ]]; then
    echo "[data-real-gate] FAIL run=$run missing terminal audit path in stderr/log stream" >&2
    echo "[data-real-gate] stdout=$out stderr=$err" >&2
    exit 1
  fi
  if [[ ! -f "$terminal_path" ]]; then
    echo "[data-real-gate] FAIL run=$run terminal audit file missing: $terminal_path" >&2
    echo "[data-real-gate] stdout=$out stderr=$err" >&2
    exit 1
  fi
  for want in '"action_graph"' '"artifact_graph"' '"progress"' '"decision"' '"process_events"' '"resume"'; do
    if ! grep -aq "$want" "$terminal_path"; then
      echo "[data-real-gate] FAIL run=$run terminal audit missing $want" >&2
      echo "[data-real-gate] terminal=$terminal_path stdout=$out stderr=$err" >&2
      exit 1
    fi
  done

  if truthy "$REQUIRE_LEDGER" && ! grep -aq '\[cli/data\] data task result.*contributions=[1-9]' "$err"; then
    echo "[data-real-gate] FAIL run=$run missing non-empty contribution ledger signal" >&2
    echo "[data-real-gate] stdout=$out stderr=$err" >&2
    exit 1
  fi
  if truthy "$REQUIRE_RECONCILE" && ! grep -aq '\[cli/data\] data task result.*reconcile=pass' "$err"; then
    echo "[data-real-gate] FAIL run=$run missing reconcile=pass signal" >&2
    echo "[data-real-gate] stdout=$out stderr=$err" >&2
    exit 1
  fi

  printf '%s\t%s\t%s\t%s\t%s\n' "$run" "$(normalize_answer "$answer")" "$out" "$err" "$terminal_path" >>"$SUMMARY"
  echo "[data-real-gate] run=$run answer=$answer stdout=$out stderr=$err terminal=$terminal_path" >&2
  printf '%s' "$answer"
}

BASELINE_ANSWER=""
BASELINE_NORMALIZED=""
for ((run = 1; run <= RUNS; run++)); do
  ANSWER="$(run_once "$run")"
  NORMALIZED="$(normalize_answer "$ANSWER")"
  if [[ "$run" -eq 1 ]]; then
    BASELINE_ANSWER="$ANSWER"
    BASELINE_NORMALIZED="$NORMALIZED"
  elif [[ "$NORMALIZED" != "$BASELINE_NORMALIZED" ]]; then
    echo "[data-real-gate] FAIL answer volatility detected" >&2
    echo "[data-real-gate] baseline=$BASELINE_ANSWER" >&2
    echo "[data-real-gate] run_$run=$ANSWER" >&2
    echo "[data-real-gate] summary=$SUMMARY" >&2
    exit 1
  fi
done

echo "[data-real-gate] PASS runs=$RUNS answer=$BASELINE_ANSWER" >&2
echo "[data-real-gate] summary=$SUMMARY" >&2
printf '%s\n' "$BASELINE_ANSWER"
