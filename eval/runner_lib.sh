#!/usr/bin/env bash

# Shared helpers for eval runners. Kept bash-3.2 compatible because
# macOS still ships that shell.

eval_run_with_timeout() {
  local seconds="$1"
  shift
  if command -v timeout >/dev/null 2>&1; then
    command timeout "$seconds" "$@"
    return $?
  fi
  if command -v gtimeout >/dev/null 2>&1; then
    command gtimeout "$seconds" "$@"
    return $?
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$seconds" "$@" <<'PY'
import os
import signal
import subprocess
import sys

timeout = float(sys.argv[1])
cmd = sys.argv[2:]
p = subprocess.Popen(cmd, start_new_session=True)
try:
    sys.exit(p.wait(timeout=timeout))
except subprocess.TimeoutExpired:
    os.killpg(p.pid, signal.SIGTERM)
    try:
        p.wait(timeout=10)
    except subprocess.TimeoutExpired:
        os.killpg(p.pid, signal.SIGKILL)
        p.wait()
    sys.exit(124)
PY
    return $?
  fi
  "$@"
}

eval_latest_result_dir() {
  local results_root="$1"
  local case_id="$2"
  local sweep_start="$3"
  local d ts
  for d in $(ls -dt "${results_root}/${case_id}"-* 2>/dev/null); do
    ts=$(basename "$d" | sed "s/${case_id}-//")
    if [[ "$ts" > "$sweep_start" ]] || [[ "$ts" == "$sweep_start"* ]]; then
      echo "$d"
      return 0
    fi
  done
  return 1
}

eval_metric_field() {
  local file="$1"
  local key="$2"
  local v
  v=$(grep -oE "^${key}=[0-9]+" "$file" 2>/dev/null | head -1 | cut -d= -f2)
  echo "${v:--}"
}

eval_count_pattern() {
  local pattern="$1"
  local file="$2"
  local n
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  n=$(grep -aE -c "$pattern" "$file" 2>/dev/null) || n=0
  echo "${n:-0}"
}

eval_running_jobs() {
  jobs -rp | wc -l | tr -d ' '
}

eval_wait_for_slot() {
  local parallel="$1"
  while [[ "$(eval_running_jobs)" -ge "$parallel" ]]; do
    sleep 1
  done
}
