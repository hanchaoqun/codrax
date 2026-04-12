#!/usr/bin/env bash
#
# Multi-sample evaluation runner.
#
# Usage:
#   eval/run.sh <case-file> [N]
#   eval/run.sh eval/cases/df1.case 5
#
# Runs the case N times (default 3), captures stdout/log per run, checks
# EXPECT_CONTAINS / EXPECT_NOT_CONTAINS substrings, extracts mechanism
# trace metrics from each run's debug log, and prints a markdown summary.
#
# Output layout:
#   eval/results/<case-id>-<timestamp>/
#     run-1.out          # codrax stdout
#     run-1.metrics.txt  # mechanism trace counters
#     run-1.verdict      # PASS/FAIL + reason
#     summary.md         # aggregated table

set -uo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <case-file> [N]" >&2
  exit 2
fi

CASE_FILE="$1"
N="${2:-3}"

if [[ ! -f "$CASE_FILE" ]]; then
  echo "case file not found: $CASE_FILE" >&2
  exit 2
fi

# shellcheck source=/dev/null
source "$CASE_FILE"

: "${ID:?case must define ID}"
: "${QUESTION:?case must define QUESTION}"
EXPECT_CONTAINS="${EXPECT_CONTAINS:-}"
EXPECT_NOT_CONTAINS="${EXPECT_NOT_CONTAINS:-}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -x ./codrax ]]; then
  echo "building codrax..." >&2
  make >/dev/null || { echo "build failed" >&2; exit 1; }
fi

TS="$(date +%Y%m%d-%H%M%S)"
OUTDIR="eval/results/${ID}-${TS}"
mkdir -p "$OUTDIR"

echo "case: $ID  ($NAME)" >&2
echo "question: $QUESTION" >&2
echo "runs: $N" >&2
echo "outdir: $OUTDIR" >&2
echo >&2

run_one() {
  local i="$1"
  local out="$OUTDIR/run-$i.out"
  local metrics="$OUTDIR/run-$i.metrics.txt"
  local verdict="$OUTDIR/run-$i.verdict"

  ./codrax -repo . -branch main -pipeline-max-steps 15 \
    -log-level debug \
    -request "$QUESTION" \
    >"$out" 2>&1
  local rc=$?

  # Find the most recent log file (per-PID, freshly created by this run).
  local log
  log="$(ls -t logs/codrax-*.log 2>/dev/null | head -1)"

  {
    echo "exit_code=$rc"
    echo "log_file=$log"
    if [[ -n "$log" && -f "$log" ]]; then
      echo "tool_read_file=$(grep -c 'tool=read_file' "$log" 2>/dev/null || echo 0)"
      echo "concrete_values=$(grep -c 'concrete values' "$log" 2>/dev/null || echo 0)"
      echo "synthesis_runs=$(grep -c 'SYNTHESIS prompt' "$log" 2>/dev/null || echo 0)"
      echo "function_boundary_push=$(grep -c 'CRITICAL.*Incomplete' "$log" 2>/dev/null || echo 0)"
      echo "enumeration_push=$(grep -c 'Enumeration completeness' "$log" 2>/dev/null || echo 0)"
      echo "focus_warning=$(grep -c 'Potential Focus' "$log" 2>/dev/null || echo 0)"
      echo "t11_gate_skip=$(grep -c 'T1.1 gate.*skipping' "$log" 2>/dev/null || echo 0)"
      echo "t11_gate_run=$(grep -c 'T1.1 gate.*running' "$log" 2>/dev/null || echo 0)"
      echo "dataflow_intent_lookup=$(grep -c 'dataflowIntent=lookup' "$log" 2>/dev/null || echo 0)"
      echo "dataflow_intent_propagate=$(grep -c 'dataflowIntent=propagate' "$log" 2>/dev/null || echo 0)"
      echo "answer_chain_lines=$(grep -c 'answer_chain' "$log" 2>/dev/null || echo 0)"
    fi
  } >"$metrics"

  # Verdict: check expectation substrings against the stdout. Strip ANSI
  # codes first since codrax may color terminal output.
  local cleaned
  cleaned="$(sed -r 's/\x1B\[[0-9;]*[A-Za-z]//g' "$out")"
  local pass=1 reasons=()

  if [[ -n "$EXPECT_CONTAINS" ]]; then
    for needle in $EXPECT_CONTAINS; do
      if ! grep -qF -- "$needle" <<<"$cleaned"; then
        pass=0
        reasons+=("missing:$needle")
      fi
    done
  fi
  if [[ -n "$EXPECT_NOT_CONTAINS" ]]; then
    for needle in $EXPECT_NOT_CONTAINS; do
      if grep -qF -- "$needle" <<<"$cleaned"; then
        pass=0
        reasons+=("banned:$needle")
      fi
    done
  fi

  if [[ $pass -eq 1 ]]; then
    echo "PASS" >"$verdict"
  else
    printf 'FAIL %s\n' "${reasons[*]}" >"$verdict"
  fi
  echo "run $i: $(cat "$verdict")" >&2
}

for i in $(seq 1 "$N"); do
  run_one "$i"
done

# Aggregate.
SUMMARY="$OUTDIR/summary.md"
{
  echo "# Eval results — $ID"
  echo
  echo "- case: \`$NAME\`"
  echo "- question: \`$QUESTION\`"
  echo "- runs: $N"
  echo "- timestamp: $TS"
  echo
  echo "## Verdicts"
  echo
  echo "| run | result | reasons |"
  echo "|----:|--------|---------|"
  pass_count=0
  for i in $(seq 1 "$N"); do
    v="$(cat "$OUTDIR/run-$i.verdict")"
    if [[ "$v" == "PASS" ]]; then
      pass_count=$((pass_count + 1))
      echo "| $i | PASS | — |"
    else
      reason="${v#FAIL }"
      echo "| $i | FAIL | $reason |"
    fi
  done
  echo
  echo "**pass rate: $pass_count / $N**"
  echo
  echo "## Mechanism trace metrics"
  echo
  echo "| metric | $(seq 1 "$N" | sed 's|^|run |' | tr '\n' '|' | sed 's/|$//') | median |"
  echo "|--------|$(printf '%.0s---|' $(seq 1 "$N"))------|"
  metric_keys="tool_read_file concrete_values synthesis_runs function_boundary_push enumeration_push focus_warning t11_gate_skip t11_gate_run dataflow_intent_lookup dataflow_intent_propagate answer_chain_lines"
  for key in $metric_keys; do
    row="| $key |"
    vals=()
    for i in $(seq 1 "$N"); do
      v=$(grep "^$key=" "$OUTDIR/run-$i.metrics.txt" 2>/dev/null | cut -d= -f2 || echo "")
      v="${v:-0}"
      vals+=("$v")
      row+=" $v |"
    done
    # Median (sorted middle).
    median=$(printf '%s\n' "${vals[@]}" | sort -n | awk -v n="$N" 'NR==int((n+1)/2){print; exit}')
    row+=" $median |"
    echo "$row"
  done
  echo
} >"$SUMMARY"

echo >&2
echo "summary written: $SUMMARY" >&2
cat "$SUMMARY"
