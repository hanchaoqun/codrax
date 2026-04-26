#!/usr/bin/env bash
# Batch L: re-run 18-task multi-language eval after the 4 retry/recovery fixes
# (commit 22dbfe4). Tag: r4 to keep prior batches' artifacts intact.
set -uo pipefail
cd "$(dirname "$0")"

ROOT=/tmp/codrax-eval/polyglot-benchmark
declare -A TASKS=(
  [accumulate-rust]="$ROOT/rust/exercises/practice/accumulate"
  [acronym-rust]="$ROOT/rust/exercises/practice/acronym"
  [alphametics-go]="$ROOT/go/exercises/practice/alphametics"
  [binary-js]="$ROOT/javascript/exercises/practice/binary"
  [counter-go]="$ROOT/go/exercises/practice/counter"
  [dominoes-go]="$ROOT/go/exercises/practice/dominoes"
  [food-chain]="$ROOT/python/exercises/practice/food-chain"
  [forth-py]="$ROOT/python/exercises/practice/forth"
  [gigasecond-rust]="$ROOT/rust/exercises/practice/gigasecond"
  [grade-school-rust]="$ROOT/rust/exercises/practice/grade-school"
  [house-js]="$ROOT/javascript/exercises/practice/house"
  [list-ops]="$ROOT/python/exercises/practice/list-ops"
  [meetup-js]="$ROOT/javascript/exercises/practice/meetup"
  [phone-number]="$ROOT/python/exercises/practice/phone-number"
  [react-go]="$ROOT/go/exercises/practice/react"
  [robot-sim-go]="$ROOT/go/exercises/practice/robot-simulator"
  [sgf-parsing]="$ROOT/python/exercises/practice/sgf-parsing"
  [space-age-js]="$ROOT/javascript/exercises/practice/space-age"
)

for name in "${!TASKS[@]}"; do
  src="${TASKS[$name]}"
  log=/tmp/codrax-eval/runs/$name-r4.driver.log
  ./run-task.sh "$src" "$name" 4 >"$log" 2>&1 &
done
wait
echo "===== BATCH L COMPLETE ====="
grep -h "^VERDICT:" /tmp/codrax-eval/runs/*-r4.driver.log 2>/dev/null | sort
