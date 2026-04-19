#!/usr/bin/env bash
#
# Multi-sample evaluation runner.
#
# Usage:
#   eval/run.sh <case-file> [N]
#   eval/run.sh eval/cases/df1.case 5
#
# Runs the case N times (default 3), captures stdout/log per run, checks
# EXPECT_CONTAINS / EXPECT_NOT_CONTAINS substrings, and optionally
# EXPECT_MATCHES_REGEX (ERE, ALL must match — useful for numeric
# scalar answers like "at least 4 digits somewhere in the answer")
# and EXPECT_SECTIONS (space-sep tokens, ALL must appear as literal
# substrings — useful for comparison questions that require both
# sides of "A vs B" to be mentioned). Extracts mechanism trace
# metrics from each run's debug log, and prints a markdown summary.
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
EXPECT_MATCHES_REGEX="${EXPECT_MATCHES_REGEX:-}"
EXPECT_SECTIONS="${EXPECT_SECTIONS:-}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Always rebuild — a stale binary silently invalidates every metric in
# the summary, and we already learned that lesson once.
echo "building codrax..." >&2
make >/dev/null || { echo "build failed" >&2; exit 1; }

TS="$(date +%Y%m%d-%H%M%S)"
OUTDIR="eval/results/${ID}-${TS}"
mkdir -p "$OUTDIR"

echo "case: $ID  ($NAME)" >&2
echo "question: $QUESTION" >&2
echo "runs: $N" >&2
echo "outdir: $OUTDIR" >&2
echo >&2

count_pattern() {
  # Robust grep -c that returns "0" cleanly on no-match (grep exits 1).
  local pat="$1" file="$2" n
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  n=$(grep -c "$pat" "$file" 2>/dev/null) || n=0
  echo "${n:-0}"
}

run_one() {
  local i="$1"
  local out="$OUTDIR/run-$i.out"
  local metrics="$OUTDIR/run-$i.metrics.txt"
  local verdict="$OUTDIR/run-$i.verdict"
  # Per-run log dir so we don't accidentally pick up an unrelated log.
  local logdir="$OUTDIR/run-$i.logs"
  mkdir -p "$logdir"

  ./codrax --repo . --branch main --pipeline-max-steps 15 \
    --log-level debug \
    --log-dir "$logdir" \
    --request "$QUESTION" \
    >"$out" 2>&1
  local rc=$?

  # Pick the most recent log file in this run's dedicated logdir.
  local log
  log="$(ls -t "$logdir"/codrax-*.log 2>/dev/null | head -1)"

  {
    echo "exit_code=$rc"
    echo "log_file=$log"
    echo "tool_read_file=$(count_pattern 'tool=read_file' "$log")"
    echo "concrete_values=$(count_pattern 'concrete values' "$log")"
    echo "synthesis_runs=$(count_pattern 'SYNTHESIS prompt' "$log")"
    echo "function_boundary_push=$(count_pattern 'CRITICAL.*Incomplete' "$log")"
    echo "enumeration_push=$(count_pattern 'Enumeration completeness' "$log")"
    echo "focus_warning=$(count_pattern 'Potential Focus' "$log")"
    echo "t11_gate_skip=$(count_pattern 'T1.1 gate.*skipping' "$log")"
    echo "t11_gate_run=$(count_pattern 'T1.1 gate.*running' "$log")"
    echo "dataflow_intent_lookup=$(count_pattern 'dataflowIntent=lookup' "$log")"
    echo "dataflow_intent_propagate=$(count_pattern 'dataflowIntent=propagate' "$log")"
    echo "midloop_inject=$(count_pattern 'MIDLOOP inject' "$log")"
    echo "answer_chain_lines=$(count_pattern 'answer_chain' "$log")"
  } >"$metrics"

  # Verdict: check expectation substrings against the stdout. Strip ANSI
  # codes first since codrax may color terminal output.
  local cleaned
  cleaned="$(sed -r 's/\x1B\[[0-9;]*[A-Za-z]//g' "$out")"
  local pass=1 reasons=()

  # Global min-length sanity filter (2026-04-14 deferred #10): any
  # answer body shorter than 20 non-whitespace characters is a
  # fragment — "type" (Go keyword picked by bug #14), "3" (df1 count
  # hallucination), "• **type" (round-4 truncation), etc. These are
  # produced by the S3 correction loop when the slate is wrong AND
  # the case gate is empty. The threshold is low enough to allow a
  # legitimate short single-symbol answer like "explorer (foo.go:12)"
  # (~25 chars) to pass.
  local stripped
  stripped="$(tr -d '[:space:]' <<<"$cleaned")"
  if (( ${#stripped} < 20 )); then
    pass=0
    reasons+=("too_short:${#stripped}chars")
  fi

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

  # EXPECT_MATCHES_REGEX: newline-separated ERE patterns (IFS hack so
  # whitespace inside a pattern is preserved — regex alternation
  # typically has no spaces but user patterns might). ALL must match.
  if [[ -n "$EXPECT_MATCHES_REGEX" ]]; then
    local old_ifs="$IFS"
    IFS=$'\n'
    for rx in $EXPECT_MATCHES_REGEX; do
      [[ -z "$rx" ]] && continue
      if ! grep -Eq -- "$rx" <<<"$cleaned"; then
        pass=0
        reasons+=("no_regex_match:${rx}")
      fi
    done
    IFS="$old_ifs"
  fi

  # EXPECT_SECTIONS: space-separated literal tokens; ALL must appear.
  # Intended for comparison-shape questions where the answer must
  # symmetrically mention both sides (A AND B). Semantically identical
  # to EXPECT_CONTAINS but spelled separately so summary reasons
  # distinguish "missing symmetric section" from "missing load-bearing
  # identifier".
  if [[ -n "$EXPECT_SECTIONS" ]]; then
    for needle in $EXPECT_SECTIONS; do
      if ! grep -qF -- "$needle" <<<"$cleaned"; then
        pass=0
        reasons+=("missing_section:$needle")
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
  metric_keys="tool_read_file concrete_values synthesis_runs function_boundary_push enumeration_push focus_warning t11_gate_skip t11_gate_run dataflow_intent_lookup dataflow_intent_propagate midloop_inject answer_chain_lines"
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
