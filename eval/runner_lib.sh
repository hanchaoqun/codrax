#!/usr/bin/env bash

# Shared helpers for eval runners. Kept bash-3.2 compatible because
# macOS still ships that shell.

eval_run_with_timeout() {
  local seconds="$1"
  shift
  # Prefer one cross-platform implementation over platform-specific
  # timeout(1) variants. The Python path starts the command in a fresh
  # process group and always tears down the whole group on timeout, so
  # a timed-out eval cannot leave LLM/tool grandchildren occupying a
  # parallel slot after the worker shell exits.
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$seconds" "$@" <<'PY'
import os
import signal
import subprocess
import sys
import time

timeout = float(sys.argv[1])
cmd = sys.argv[2:]
p = subprocess.Popen(cmd, start_new_session=True)
deadline = time.time() + timeout

def kill_group(sig):
    try:
        os.killpg(p.pid, sig)
    except ProcessLookupError:
        pass

def terminate(signum, _frame):
    kill_group(signal.SIGTERM)
    grace_deadline = time.time() + 10
    while True:
        rc = p.poll()
        if rc is not None:
            sys.exit(128 + signum)
        if time.time() >= grace_deadline:
            break
        time.sleep(min(0.25, max(0.0, grace_deadline - time.time())))
    kill_group(signal.SIGKILL)
    p.wait()
    sys.exit(128 + signum)

signal.signal(signal.SIGTERM, terminate)
signal.signal(signal.SIGINT, terminate)

try:
    while True:
        rc = p.poll()
        if rc is not None:
            sys.exit(rc)
        if time.time() >= deadline:
            break
        time.sleep(min(0.5, max(0.0, deadline - time.time())))
    kill_group(signal.SIGTERM)
    grace_deadline = time.time() + 10
    while True:
        rc = p.poll()
        if rc is not None:
            sys.exit(124)
        if time.time() >= grace_deadline:
            break
        time.sleep(min(0.25, max(0.0, grace_deadline - time.time())))
    kill_group(signal.SIGKILL)
    p.wait()
    sys.exit(124)
except KeyboardInterrupt:
    kill_group(signal.SIGTERM)
    raise
PY
    return $?
  fi
  if command -v timeout >/dev/null 2>&1; then
    command timeout -k 10 "$seconds" "$@"
    return $?
  fi
  if command -v gtimeout >/dev/null 2>&1; then
    command gtimeout -k 10 "$seconds" "$@"
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
  local line
  line=$(grep -a -m1 "^${key}=" "$file" 2>/dev/null || true)
  if [[ -z "$line" ]]; then
    echo "-"
    return 0
  fi
  printf '%s\n' "${line#*=}" | tr -d '\000'
}

eval_is_uint() {
  case "$1" in
    ""|*[!0-9]*)
      return 1
      ;;
    *)
      return 0
      ;;
  esac
}

eval_metric_int_field() {
  local file="$1"
  local key="$2"
  local v
  v="$(eval_metric_field "$file" "$key")"
  if eval_is_uint "$v"; then
    echo "$v"
    return 0
  fi
  echo 0
}

eval_metric_exceeds() {
  local file="$1"
  local key="$2"
  local limit="$3"
  local v
  if ! eval_is_uint "$limit"; then
    return 1
  fi
  v="$(eval_metric_int_field "$file" "$key")"
  [[ "$v" -gt "$limit" ]]
}

eval_print_efficiency_advisory_row() {
  local file="$1"
  local run_id="$2"
  local advisory="$3"
  local key="$4"
  local limit="$5"
  local v
  if ! eval_metric_exceeds "$file" "$key" "$limit"; then
    return 1
  fi
  v="$(eval_metric_int_field "$file" "$key")"
  printf '| %s | %s | %s=%s limit=%s |\n' "$run_id" "$advisory" "$key" "$v" "$limit"
  return 0
}

eval_metric_budget_reasons() {
  local file="$1"
  shift
  local key limit v
  while [[ $# -ge 2 ]]; do
    key="$1"
    limit="$2"
    shift 2
    if eval_metric_exceeds "$file" "$key" "$limit"; then
      v="$(eval_metric_int_field "$file" "$key")"
      printf 'perf_budget:%s:%s>%s\n' "$key" "$v" "$limit"
    fi
  done
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

eval_json_escape() {
  printf '%s' "$1" | LC_ALL=C sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

eval_env_key() {
  LC_ALL=C tr '[:lower:]' '[:upper:]' <<<"$1" | LC_ALL=C sed -E 's/[^A-Z0-9]+/_/g; s/^_+//; s/_+$//'
}

eval_trim() {
  LC_ALL=C sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//' <<<"$1"
}

eval_reason_slug() {
  printf '%s' "$1" |
    LC_ALL=C tr '\n\r\t ' '____' |
    LC_ALL=C sed -E 's/[^A-Za-z0-9_.:@+\/-]+/_/g; s/_+/_/g; s/^_+//; s/_+$//' |
    LC_ALL=C cut -c1-120
}

eval_inventory_row_tokens_visible() {
  local text="$1"
  local row="$2"
  local old_ifs token seen
  local -a row_tokens
  old_ifs="$IFS"
  IFS='|'
  read -r -a row_tokens <<<"$row"
  IFS="$old_ifs"
  seen=0
  for token in "${row_tokens[@]}"; do
    token="$(eval_trim "$token")"
    [[ -z "$token" ]] && continue
    seen=1
    if ! LC_ALL=C grep -aqiF -- "$token" <<<"$text"; then
      return 1
    fi
  done
  [[ "$seen" -eq 1 ]]
}

eval_inventory_row_visible() {
  local cleaned="$1"
  local row="$2"
  local scope="${3:-document}"
  local old_ifs line
  case "$scope" in
    line|row)
      old_ifs="$IFS"
      IFS=$'\n'
      for line in $cleaned; do
        if eval_inventory_row_tokens_visible "$line" "$row"; then
          IFS="$old_ifs"
          return 0
        fi
      done
      IFS="$old_ifs"
      return 1
      ;;
    *)
      eval_inventory_row_tokens_visible "$cleaned" "$row"
      ;;
  esac
}

eval_inventory_rowset_reasons() {
  local cleaned="$1"
  local rowsets="${EXPECT_INVENTORY_ROWSETS:-}"
  [[ -n "$rowsets" ]] || return 0

  local rowset rowset_key rows_var rows count_var expected_count
  local banned_var banned_rows scope_var row_scope old_ifs row matched total reason_row
  for rowset in $rowsets; do
    rowset_key="$(eval_env_key "$rowset")"
    rows_var="EXPECT_INVENTORY_ROWS_${rowset_key}"
    rows="${!rows_var:-}"
    count_var="EXPECT_INVENTORY_COUNT_${rowset_key}"
    expected_count="${!count_var:-}"
    scope_var="EXPECT_INVENTORY_ROW_SCOPE_${rowset_key}"
    row_scope="${!scope_var:-document}"

    matched=0
    total=0
    if [[ -n "$rows" ]]; then
      old_ifs="$IFS"
      IFS=$'\n'
      for row in $rows; do
        row="$(eval_trim "$row")"
        [[ -z "$row" ]] && continue
        total=$((total + 1))
        reason_row="$(eval_reason_slug "$row")"
        if eval_inventory_row_visible "$cleaned" "$row" "$row_scope"; then
          matched=$((matched + 1))
        else
          printf 'missing_inventory_row:%s:%s\n' "$rowset" "$reason_row"
        fi
      done
      IFS="$old_ifs"
    fi

    if [[ -z "$expected_count" ]]; then
      expected_count="$total"
    fi
    if ! eval_is_uint "$expected_count"; then
      printf 'invalid_inventory_count:%s:%s\n' "$rowset" "$(eval_reason_slug "$expected_count")"
    elif [[ "$matched" -ne "$expected_count" ]]; then
      printf 'inventory_count_mismatch:%s:got%s:want%s\n' "$rowset" "$matched" "$expected_count"
    fi

    banned_var="EXPECT_INVENTORY_BANNED_ROWS_${rowset_key}"
    banned_rows="${!banned_var:-}"
    if [[ -n "$banned_rows" ]]; then
      old_ifs="$IFS"
      IFS=$'\n'
      for row in $banned_rows; do
        row="$(eval_trim "$row")"
        [[ -z "$row" ]] && continue
        if eval_inventory_row_visible "$cleaned" "$row" "$row_scope"; then
          printf 'banned_inventory_row:%s:%s\n' "$rowset" "$(eval_reason_slug "$row")"
        fi
      done
      IFS="$old_ifs"
    fi
  done
}

eval_json_top_string_field() {
  local file="$1"
  local field="$2"
  [[ -f "$file" ]] || return 1
  LC_ALL=C sed -nE 's/^  "'"$field"'"[[:space:]]*:[[:space:]]*"([^"]*)",?$/\1/p' "$file" | head -1
}

eval_json_top_bool_field() {
  local file="$1"
  local field="$2"
  [[ -f "$file" ]] || return 1
  LC_ALL=C sed -nE 's/^  "'"$field"'"[[:space:]]*:[[:space:]]*(true|false),?$/\1/p' "$file" | head -1
}

eval_bool_literal() {
  case "$1" in
    1|true|TRUE|yes|YES)
      echo true
      ;;
    *)
      echo false
      ;;
  esac
}

eval_find_write_report_path() {
  local plan_path="$1"
  local outdir="$2"
  local scratch="$3"
  local worktree="$4"
  local plan_id="$5"
  local plan_dir candidate
  [[ -n "$plan_id" ]] || return 1
  plan_dir="$(dirname "$plan_path")"
  for candidate in \
    "$outdir/${plan_id}.report.json" \
    "$plan_dir/${plan_id}.report.json" \
    "$scratch/.codrax/plans/${plan_id}.report.json" \
    "$worktree/.codrax/plans/${plan_id}.report.json"
  do
    if [[ -n "$candidate" && -f "$candidate" ]]; then
      printf '%s' "$candidate"
      return 0
    fi
  done
  candidate="$(find "$outdir" -maxdepth 1 -type f -name "${plan_id}.report.json" 2>/dev/null | sort | tail -1)"
  if [[ -n "$candidate" && -f "$candidate" ]]; then
    printf '%s' "$candidate"
    return 0
  fi
  return 1
}

eval_materialize_write_apply_source() {
  local plan_path="$1"
  local outdir="$2"
  local scratch="$3"
  local run_id="$4"
  local worktree="" plan_id="" applied_sha="" commit="" dest=""
  if [[ -f "$plan_path" ]]; then
    worktree="$(eval_json_top_string_field "$plan_path" worktree_path || true)"
    plan_id="$(eval_json_top_string_field "$plan_path" id || true)"
    applied_sha="$(eval_json_top_string_field "$plan_path" applied_commit_sha || true)"
  fi
  if [[ -n "$worktree" && -d "$worktree" ]]; then
    printf '%s\n' "$worktree"
    return 0
  fi
  if [[ -z "$scratch" || ! -d "$scratch/.git" || -z "$plan_id" ]]; then
    return 1
  fi
  if [[ -n "$applied_sha" ]] && git -C "$scratch" cat-file -e "${applied_sha}^{commit}" >/dev/null 2>&1; then
    commit="$applied_sha"
  elif git -C "$scratch" cat-file -e "refs/codrax/applied/${plan_id}^{commit}" >/dev/null 2>&1; then
    commit="refs/codrax/applied/${plan_id}"
  else
    return 1
  fi
  dest="$outdir/run-${run_id}.applied-tree"
  rm -rf "$dest"
  mkdir -p "$dest"
  if git -C "$scratch" archive "$commit" | tar -x -C "$dest"; then
    printf '%s\n' "$dest"
    return 0
  fi
  rm -rf "$dest"
  return 1
}

eval_find_latest_change_plan() {
  # Finds the newest persisted ChangePlan JSON under one or more roots.
  # Used by commandless write-mode evals where the product persists the plan
  # internally instead of receiving --plan-out from the harness.
  local roots=("$@")
  local root file ts
  for root in "${roots[@]}"; do
    [[ -n "$root" && -d "$root" ]] || continue
    find "$root" -type f -name '*.json' ! -name '*.report.json' ! -path '*/workflows/*' -print 2>/dev/null
  done | while IFS= read -r file; do
    [[ -f "$file" ]] || continue
    if LC_ALL=C grep -aq '"changes"[[:space:]]*:' "$file" &&
      LC_ALL=C grep -aq '"id"[[:space:]]*:' "$file" &&
      LC_ALL=C grep -aq '"summary"[[:space:]]*:' "$file"; then
      ts="$(stat -f %m "$file" 2>/dev/null || stat -c %Y "$file" 2>/dev/null || echo 0)"
      printf '%s\t%s\n' "$ts" "$file"
    fi
  done | LC_ALL=C sort -n | tail -1 | cut -f2-
}

eval_collect_apply_source_text() {
  local source="$1"
  local source_real="" git_top="" git_top_real=""
  if [[ -z "$source" || ! -d "$source" ]]; then
    return 1
  fi
  source_real="$(cd "$source" && pwd -P)" || return 1
  git_top="$(git -C "$source" rev-parse --show-toplevel 2>/dev/null || true)"
  if [[ -n "$git_top" && -d "$git_top" ]]; then
    git_top_real="$(cd "$git_top" && pwd -P)" || git_top_real=""
  fi
  if [[ -n "$git_top_real" && "$git_top_real" == "$source_real" ]]; then
    git -C "$source" ls-files -z 2>/dev/null | (cd "$source" && xargs -0 cat 2>/dev/null)
    return 0
  fi
  find "$source" -type f ! -path '*/.git/*' -print 2>/dev/null | LC_ALL=C sort | while IFS= read -r file; do
    cat "$file" 2>/dev/null || true
    printf '\n'
  done
}

eval_post_apply_source_file() {
  local plan_path="$1"
  local outdir="$2"
  local scratch="$3"
  local run_id="$4"
  local post_apply_file="$5"
  local source=""
  if [[ -z "$post_apply_file" ]]; then
    return 1
  fi
  if [[ -f "$plan_path" ]]; then
    source="$(eval_materialize_write_apply_source "$plan_path" "$outdir" "$scratch" "$run_id" || true)"
    if [[ -n "$source" && -f "$source/$post_apply_file" ]]; then
      printf '%s\n' "$source/$post_apply_file"
      return 0
    fi
  fi
  if [[ -f "$scratch/$post_apply_file" ]]; then
    printf '%s\n' "$scratch/$post_apply_file"
    return 0
  fi
  return 1
}

eval_print_regex_matching_lines() {
  local file="$1"
  local regex_text="$2"
  local limit="${3:-12}"
  local old_ifs="$IFS"
  local rx line count=0
  if [[ -z "$file" || ! -f "$file" || -z "$regex_text" ]]; then
    return 1
  fi
  IFS='
'
  for rx in $regex_text; do
    [[ -n "$rx" ]] || continue
    while IFS= read -r line; do
      printf '%s\n' "$line"
      count=$((count + 1))
      if [[ "$count" -ge "$limit" ]]; then
        IFS="$old_ifs"
        return 0
      fi
    done < <(LC_ALL=C grep -nE "$rx" "$file" 2>/dev/null | head -20)
  done
  IFS="$old_ifs"
  [[ "$count" -gt 0 ]]
}

eval_print_applied_diff_hunk() {
  local plan_path="$1"
  local scratch="$2"
  local post_apply_file="$3"
  local limit="${4:-80}"
  local plan_id="" applied_sha="" commit=""
  if [[ -z "$plan_path" || ! -f "$plan_path" || -z "$scratch" || ! -d "$scratch/.git" ]]; then
    return 1
  fi
  plan_id="$(eval_json_top_string_field "$plan_path" id || true)"
  applied_sha="$(eval_json_top_string_field "$plan_path" applied_commit_sha || true)"
  if [[ -n "$applied_sha" ]] && git -C "$scratch" cat-file -e "${applied_sha}^{commit}" >/dev/null 2>&1; then
    commit="$applied_sha"
  elif [[ -n "$plan_id" ]] && git -C "$scratch" cat-file -e "refs/codrax/applied/${plan_id}^{commit}" >/dev/null 2>&1; then
    commit="refs/codrax/applied/${plan_id}"
  else
    return 1
  fi
  if [[ -n "$post_apply_file" ]]; then
    git -C "$scratch" show --format= --unified=3 "$commit" -- "$post_apply_file" 2>/dev/null | head -"$limit"
  else
    git -C "$scratch" show --format= --unified=3 "$commit" 2>/dev/null | head -"$limit"
  fi
}

eval_print_write_report_commands() {
  local plan_path="$1"
  local outdir="$2"
  local scratch="$3"
  local plan_id="" worktree="" report_path=""
  if [[ -z "$plan_path" || ! -f "$plan_path" ]]; then
    return 1
  fi
  plan_id="$(eval_json_top_string_field "$plan_path" id || true)"
  worktree="$(eval_json_top_string_field "$plan_path" worktree_path || true)"
  report_path="$(eval_find_write_report_path "$plan_path" "$outdir" "$scratch" "$worktree" "$plan_id" || true)"
  if [[ -z "$report_path" || ! -f "$report_path" ]]; then
    return 1
  fi
  python3 - "$report_path" <<'PY' 2>/dev/null
import json
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as f:
        report = json.load(f)
except Exception:
    raise SystemExit(1)

commands = report.get("executed_commands") or []
if not commands:
    raise SystemExit(1)
for idx, cmd in enumerate(commands[:8], 1):
    if not isinstance(cmd, dict):
        continue
    runner = str(cmd.get("runner") or "?")
    cwd = str(cmd.get("working_dir") or ".")
    command = str(cmd.get("command") or "").replace("\n", " ")
    outcome = str(cmd.get("outcome") or "?")
    source = str(cmd.get("source") or "?")
    exit_code = cmd.get("exit_code")
    if exit_code is None:
        exit_code = "?"
    print(f"{idx}. runner={runner} cwd={cwd} exit={exit_code} outcome={outcome} source={source} cmd={command}")
PY
}

eval_write_apply_result_record() {
  local result_file="$1"
  local plan_path="$2"
  local outdir="$3"
  local scratch="$4"
  local plan_written="$5"
  local apply_attempted="$6"
  local allow_unverified="$7"
  local plan_id="" worktree="" worktree_exists=false report_path="" report_exists=false
  local report_plan_id="" report_channel="" report_passed="" authoritative=false

  if [[ -f "$plan_path" ]]; then
    plan_id="$(eval_json_top_string_field "$plan_path" id || true)"
    worktree="$(eval_json_top_string_field "$plan_path" worktree_path || true)"
  fi
  if [[ -n "$worktree" && -d "$worktree" ]]; then
    worktree_exists=true
  fi
  report_path="$(eval_find_write_report_path "$plan_path" "$outdir" "$scratch" "$worktree" "$plan_id" || true)"
  if [[ -n "$report_path" && -f "$report_path" ]]; then
    report_exists=true
    report_plan_id="$(eval_json_top_string_field "$report_path" plan_id || true)"
    report_channel="$(eval_json_top_string_field "$report_path" channel || true)"
    report_passed="$(eval_json_top_bool_field "$report_path" passed || true)"
  fi
  if [[ "$report_exists" == true && "$report_plan_id" == "$plan_id" && "$report_channel" == "post_apply_verify" && "$report_passed" == "true" ]]; then
    authoritative=true
  fi

  {
    echo "{"
    printf '  "plan_id": "%s",\n' "$(eval_json_escape "$plan_id")"
    printf '  "plan_written": %s,\n' "$(eval_bool_literal "$plan_written")"
    printf '  "apply_attempted": %s,\n' "$(eval_bool_literal "$apply_attempted")"
    printf '  "worktree_path": "%s",\n' "$(eval_json_escape "$worktree")"
    printf '  "worktree_exists": %s,\n' "$worktree_exists"
    printf '  "report_path": "%s",\n' "$(eval_json_escape "$report_path")"
    printf '  "report_exists": %s,\n' "$report_exists"
    printf '  "report_plan_id": "%s",\n' "$(eval_json_escape "$report_plan_id")"
    printf '  "report_channel": "%s",\n' "$(eval_json_escape "$report_channel")"
    if [[ "$report_passed" == "true" || "$report_passed" == "false" ]]; then
      printf '  "report_passed": %s,\n' "$report_passed"
    else
      echo '  "report_passed": false,'
    fi
    printf '  "verify_authoritative": %s,\n' "$authoritative"
    printf '  "allow_unverified": %s\n' "$(eval_bool_literal "$allow_unverified")"
    echo "}"
  } >"$result_file"
}

eval_max_context_tokens_estimate() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    {
      if ($0 !~ /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\][^:]*phase=llm_request/) {
        next
      }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^context_tokens_est=[0-9]+$/) {
          split($i, a, "=")
          if (a[2] + 0 > max) {
            max = a[2] + 0
          }
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_max_context_window_tokens() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    {
      if ($0 !~ /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\][^:]*phase=llm_request/) {
        next
      }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^context_window=[0-9]+$/) {
          split($i, a, "=")
          if (a[2] + 0 > max) {
            max = a[2] + 0
          }
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_max_context_window_pct() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    {
      if ($0 !~ /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\][^:]*phase=llm_request/) {
        next
      }
      tok = 0
      win = 0
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^context_tokens_est=[0-9]+$/) {
          split($i, a, "=")
          tok = a[2] + 0
        } else if ($i ~ /^context_window=[0-9]+$/) {
          split($i, b, "=")
          win = b[2] + 0
        }
      }
      if (tok > 0 && win > 0) {
        pct = int((tok * 100 / win) + 0.5)
        if (pct > max) {
          max = pct
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_count_control_pattern() {
  local pattern="$1"
  local file="$2"
  # Control-plane metrics must not match source snippets, model answers, or
  # customer-log payloads. Codrax debug logs are timestamped; requiring that
  # prefix keeps quoted control-looking text out of retry/rewrite counters.
  eval_count_pattern "^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]* ${pattern}" "$file"
}

eval_count_finalizer_rejects() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  local tool render n
  # Count control-plane events only. Whole-log grep of these strings is unsafe:
  # source snippets and model answers may discuss `finalizer_rejects` or quote
  # customer logs that contain "成文校验未通过".
  tool=$(eval_count_control_pattern 'DEBUG \[diag finalizer\][^:]*phase=toolresult TOOLRESULT emit_answer_document(_patch)? ok=false' "$file")
  # Multibyte bracket expressions ([校交]) are locale-dependent: in a
  # non-UTF-8 grep locale the class decomposes into single BYTES and the
  # pattern can never match, silently zeroing the counter. Literal
  # alternation is byte-exact in every locale.
  render=$(eval_count_control_pattern 'INFO \[render\][[:space:]]+•[[:space:]]+(成文校验未通过|成文交验未通过)' "$file")
  n=$((tool + render))
  echo "$n"
}

eval_count_finalizer_rewrites() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'INFO \[render\][[:space:]]+⟳ 4/4 .*(答案待完善|正在重写答案|检测到 .*前后不一致)' "$file"
}

eval_convergence_flags() {
  local metrics="$1"
  local verdict="${2:-UNKNOWN}"
  local read_calls repo_map_calls list_files_calls exp exp_disp sem
  local fin fin_reject fin_rewrite mermaid_repair answer_contract answer_contract_strict_raw answer_contract_final_strict_raw answer_contract_strict
  local history_prunes origin_block analyze_refine_dispatches read_loop_add_proof_consumed flags

  read_calls="$(eval_metric_int_field "$metrics" tool_read_file)"
  repo_map_calls="$(eval_metric_int_field "$metrics" tool_repo_map)"
  list_files_calls="$(eval_metric_int_field "$metrics" tool_list_files)"
  exp="$(eval_metric_int_field "$metrics" explorer_iters)"
  exp_disp="$(eval_metric_int_field "$metrics" explorer_dispatches)"
  sem="$(eval_metric_int_field "$metrics" semantic_quality_concerns)"
  fin="$(eval_metric_int_field "$metrics" finalizer_iters)"
  fin_reject="$(eval_metric_int_field "$metrics" finalizer_rejects)"
  fin_rewrite="$(eval_metric_int_field "$metrics" finalizer_rewrites)"
  mermaid_repair="$(eval_metric_int_field "$metrics" mermaid_source_repair_applied)"
  answer_contract="$(eval_metric_int_field "$metrics" answer_contract_violations)"
  answer_contract_final_strict_raw="$(eval_metric_field "$metrics" answer_contract_final_strict_violations)"
  answer_contract_strict_raw="$(eval_metric_field "$metrics" answer_contract_strict_violations)"
  if [[ -n "$answer_contract_final_strict_raw" && "$answer_contract_final_strict_raw" != "-" ]]; then
    answer_contract_strict="$(eval_metric_int_field "$metrics" answer_contract_final_strict_violations)"
  elif [[ -n "$answer_contract_strict_raw" && "$answer_contract_strict_raw" != "-" ]]; then
    answer_contract_strict="$(eval_metric_int_field "$metrics" answer_contract_strict_violations)"
  else
    answer_contract_strict="$answer_contract"
  fi
  history_prunes="$(eval_metric_int_field "$metrics" tool_history_prunes)"
  origin_block="$(eval_metric_int_field "$metrics" mixed_origin_autocomplete_blocks)"
  analyze_refine_dispatches="$(eval_metric_int_field "$metrics" analyze_refine_dispatches)"
  read_loop_add_proof_consumed="$(eval_metric_int_field "$metrics" read_loop_add_proof_consumed)"

  flags=""
  if [[ "$verdict" != "PASS" ]]; then
    flags="${flags} verdict"
  fi
  if [[ "$fin" -gt 1 || "$fin_reject" -gt 0 || "$fin_rewrite" -gt 0 ]]; then
    flags="${flags} finalizer"
  fi
  if [[ "$mermaid_repair" -gt 0 && ( "$fin" -gt 1 || "$fin_reject" -gt 0 || "$fin_rewrite" -gt 0 ) ]]; then
    flags="${flags} repair_churn"
  fi
  if [[ "$exp_disp" -gt 1 || "$exp" -gt 25 ]]; then
    flags="${flags} explorer_long"
  fi
  if [[ "$read_calls" -gt 30 || "$repo_map_calls" -gt 8 || "$list_files_calls" -gt 12 ]]; then
    flags="${flags} wide_search"
  fi
  if [[ "$sem" -gt 0 ]]; then
    flags="${flags} semantic"
  fi
  if [[ "$answer_contract_strict" -gt 0 ]]; then
    flags="${flags} contract_warning"
  fi
  if [[ "$history_prunes" -gt 0 ]]; then
    flags="${flags} context_prune"
  fi
  if [[ "$origin_block" -gt 0 ]]; then
    flags="${flags} lane_wait"
  fi
  if [[ "$analyze_refine_dispatches" -gt 1 || "$read_loop_add_proof_consumed" -gt 1 ]]; then
    flags="${flags} adaptive_loop"
  fi
  flags="${flags# }"
  printf '%s\n' "${flags:-—}"
}

eval_count_answer_document_patch_calls() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag finalizer\][^:]*phase=toolcall [^:]*tool=emit_answer_document_patch( |$)' "$file"
}

eval_count_midloop_injects() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=midloop_inject' "$file"
}

eval_count_analyze_refine_dispatches() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag orchestrator\][^:]*phase=read_dag_dispatch .*analyze_refine=true' "$file"
}

eval_count_read_loop_add_proof_selected() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag orchestrator\][^:]*phase=read_loop_next_action_selected .*action=add_proof' "$file"
}

eval_count_read_loop_add_proof_consumed() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag orchestrator\][^:]*phase=read_loop_next_action_consumed .*action=add_proof' "$file"
}

eval_count_tool_calls() {
  local file="$1"
  local tool="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$tool" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag [^]]+\\][^:]*phase=toolcall [^:]*tool=${tool}( |$)" "$file"
}

eval_count_repeated_mcp_resource_reads() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\]/ &&
    $0 !~ /ASSISTANT content/ &&
    $0 ~ /phase=toolcall .*tool=mcp_read_resource( |$)/ {
      idx = index($0, " params=")
      key = $0
      if (idx > 0) {
        key = substr($0, idx + 8)
      }
      count[key]++
    }
    END {
      dup = 0
      for (k in count) {
        if (count[k] > 1) {
          dup += count[k] - 1
        }
      }
      print dup + 0
    }
  ' "$file"
}

eval_count_source_inventory_tool_calls() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Count actual repo_map tool calls only. Prompt text, model answers, and
  # customer logs can mention source_inventory without the model using it.
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=toolcall [^:]*tool=repo_map params=.*"view"[[:space:]]*:[[:space:]]*"source_inventory"' "$file"
}

eval_count_repo_lens_discovery_hints() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[repo_lens\] discovery_hint ' "$file"
}

eval_count_transient_retry_checkpoints() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Counts only the orchestrator control-plane event emitted when a preserved
  # explore-state checkpoint is installed after a stream-level retry. Prompt
  # text and quoted customer logs can mention the same phrase and must not
  # inflate this metric.
  eval_count_control_pattern 'DEBUG \[diag orchestrator\][^:]*phase=transient_retry_checkpoint [^:]*installed=true' "$file"
}

eval_count_unavailable_tool_attempts() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  local global restricted
  global=$(eval_count_control_pattern 'WARN \[agent\] tool "[^"]+" rejected before execution: not in current tool schema' "$file")
  restricted=$(eval_count_control_pattern 'WARN \[explorer\] tool "[^"]+" rejected: tool "[^"]+" is not available in the current explorer repair state' "$file")
  echo $((global + restricted))
}

eval_count_checkpoint_continuation_broad_hint() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[orchestrator\] window hint applied .*Checkpoint summary.*DAG-scheduled investigation window' "$file"
}

eval_count_closure_only_repeated() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Counts the old anti-pattern where closure-only hints carried an
  # iteration suffix, allowing the same closure instruction to look
  # like a fresh hint every round. Stable per-window keys should keep
  # this at zero.
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=midloop_signal .*key="explorer\.mid-loop\.[^"]*closure-only\.[0-9]+"' "$file"
}

eval_count_mermaid_source_repairs() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[mermaidcompat\] source repair applied/ {
      hash = ""
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^repair_hash=/) {
          split($i, kv, "=")
          hash = kv[2]
          break
        }
      }
      if (hash != "") {
        seen[hash] = 1
      } else {
        legacy++
      }
    }
    END {
      total = legacy
      for (h in seen) {
        total++
      }
      print total + 0
    }
  ' "$file"
}

eval_count_repair_debt_checkpoints() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[repair_debt\] checkpoint ' "$file"
}

eval_count_repair_debt_close_ready_filters() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[repair_debt\] close_ready filtered_advisory=' "$file"
}

eval_max_repair_debt_checkpoint_class() {
  local file="$1" class="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$class" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk -v cls="$class" '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[repair_debt\] checkpoint / {
      n = split($0, fields, /[[:space:]]+/)
      for (i = 1; i <= n; i++) {
        if (fields[i] ~ ("^" cls "=[0-9]+$")) {
          split(fields[i], kv, "=")
          if ((kv[2] + 0) > max) {
            max = kv[2] + 0
          }
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_sum_answer_contract_violations() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      strict = ""
      total = ""
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^strict_violations=[0-9]+$/) {
          split($i, a, "=")
          strict = a[2] + 0
        } else if ($i ~ /^violations=[0-9]+$/) {
          split($i, a, "=")
          total = a[2] + 0
        }
      }
      if (strict != "") {
        sum += strict
      } else if (total != "") {
        # Legacy logs did not split strict/advisory, so preserve
        # the old hard-violation interpretation for historical runs.
        sum += total
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_sum_answer_contract_violations_for_section() {
  local file="$1"
  local section="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$section" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk -v section="$section" '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      matched = 0
      for (i = 1; i <= NF; i++) {
        if ($i == "section=" section) {
          matched = 1
        }
      }
      if (!matched) {
        next
      }
      strict = ""
      total = ""
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^strict_violations=[0-9]+$/) {
          split($i, a, "=")
          strict = a[2] + 0
        } else if ($i ~ /^violations=[0-9]+$/) {
          split($i, a, "=")
          total = a[2] + 0
        }
      }
      if (strict != "") {
        sum += strict
      } else if (total != "") {
        sum += total
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_sum_answer_contract_advisories() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^soft_violations=[0-9]+$/) {
          split($i, a, "=")
          sum += a[2] + 0
        }
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_sum_answer_contract_advisories_for_section() {
  local file="$1"
  local section="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$section" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk -v section="$section" '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      matched = 0
      for (i = 1; i <= NF; i++) {
        if ($i == "section=" section) {
          matched = 1
        }
      }
      if (!matched) {
        next
      }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^soft_violations=[0-9]+$/) {
          split($i, a, "=")
          sum += a[2] + 0
        }
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_sum_answer_contract_strict_violations() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      strict = ""
      total = ""
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^strict_violations=[0-9]+$/) {
          split($i, a, "=")
          strict = a[2] + 0
        } else if ($i ~ /^violations=[0-9]+$/) {
          split($i, b, "=")
          total = b[2] + 0
        }
      }
      if (strict != "") {
        sum += strict
      } else if (total != "") {
        # Legacy logs did not split strict/soft, so keep old behavior
        # when auditing pre-D1-F10g.83 results.
        sum += total
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_sum_answer_contract_strict_violations_for_section() {
  local file="$1"
  local section="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$section" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk -v section="$section" '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      matched = 0
      for (i = 1; i <= NF; i++) {
        if ($i == "section=" section) {
          matched = 1
        }
      }
      if (!matched) {
        next
      }
      strict = ""
      total = ""
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^strict_violations=[0-9]+$/) {
          split($i, a, "=")
          strict = a[2] + 0
        } else if ($i ~ /^violations=[0-9]+$/) {
          split($i, b, "=")
          total = b[2] + 0
        }
      }
      if (strict != "") {
        sum += strict
      } else if (total != "") {
        sum += total
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_answer_contract_phase_strict_violations() {
  local file="$1"
  local phase="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$phase" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk -v phase="$phase" '
    function field_value(name,    i, pat, a) {
      pat = "^" name "=[0-9]+$"
      for (i = 1; i <= NF; i++) {
        if ($i ~ pat) {
          split($i, a, "=")
          return a[2] + 0
        }
      }
      return -1
    }
    function section_value(    i, a) {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^section=[^[:space:]]+$/) {
          split($i, a, "=")
          return a[2]
        }
      }
      return "unknown"
    }
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      section = section_value()
      strict = field_value("strict_violations")
      if (strict < 0) {
        strict = field_value("violations")
      }
      if (strict < 0) {
        strict = 0
      }
      if (!(section in seen)) {
        first[section] = strict
        seen[section] = 1
      }
      final[section] = strict
    }
    END {
      sum = 0
      for (section in seen) {
        if (phase == "first") {
          sum += first[section]
        } else if (phase == "final") {
          sum += final[section]
        } else if (phase == "auto_repaired") {
          delta = first[section] - final[section]
          if (delta > 0) {
            sum += delta
          }
        }
      }
      print sum + 0
    }
  ' "$file"
}

eval_answer_contract_phase_strict_violations_for_section() {
  local file="$1"
  local section_filter="$2"
  local phase="$3"
  if [[ -z "$file" || ! -f "$file" || -z "$section_filter" || -z "$phase" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk -v section_filter="$section_filter" -v phase="$phase" '
    function field_value(name,    i, pat, a) {
      pat = "^" name "=[0-9]+$"
      for (i = 1; i <= NF; i++) {
        if ($i ~ pat) {
          split($i, a, "=")
          return a[2] + 0
        }
      }
      return -1
    }
    function section_value(    i, a) {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^section=[^[:space:]]+$/) {
          split($i, a, "=")
          return a[2]
        }
      }
      return "unknown"
    }
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      section = section_value()
      if (section != section_filter) {
        next
      }
      strict = field_value("strict_violations")
      if (strict < 0) {
        strict = field_value("violations")
      }
      if (strict < 0) {
        strict = 0
      }
      if (!seen) {
        first = strict
        seen = 1
      }
      final = strict
    }
    END {
      if (!seen) {
        print 0
      } else if (phase == "first") {
        print first + 0
      } else if (phase == "final") {
        print final + 0
      } else if (phase == "auto_repaired") {
        delta = first - final
        if (delta < 0) {
          delta = 0
        }
        print delta + 0
      } else {
        print 0
      }
    }
  ' "$file"
}

eval_first_pass_answer_contract_strict_violations() {
  eval_answer_contract_phase_strict_violations "$1" first
}

eval_final_answer_contract_strict_violations() {
  eval_answer_contract_phase_strict_violations "$1" final
}

eval_auto_repaired_answer_contract_strict_violations() {
  eval_answer_contract_phase_strict_violations "$1" auto_repaired
}

eval_first_pass_answer_contract_strict_violations_for_section() {
  eval_answer_contract_phase_strict_violations_for_section "$1" "$2" first
}

eval_final_answer_contract_strict_violations_for_section() {
  eval_answer_contract_phase_strict_violations_for_section "$1" "$2" final
}

eval_auto_repaired_answer_contract_strict_violations_for_section() {
  eval_answer_contract_phase_strict_violations_for_section "$1" "$2" auto_repaired
}

eval_count_agent_iterations() {
  local file="$1"
  local agent="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$agent" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag ${agent}\\][^:]*ASSISTANT content_len=" "$file"
}

eval_count_agent_dispatches() {
  local file="$1"
  local agent="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$agent" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag ${agent}\\][^:]*DISPATCH stage=" "$file"
}

eval_count_semantic_quality_dispatches() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'INFO \[semantic_quality_reviewer\].*verdict' "$file"
}

eval_count_semantic_quality_concerns() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'INFO \[semantic_quality_reviewer\].*(emitted [1-9]|verdict sufficient=false)' "$file"
}

eval_count_self_consistency_concerns() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  local reviewer orchestrator n
  reviewer=$(eval_count_control_pattern 'INFO \[self_consistency_reviewer\].*(emitted|consistent=false)' "$file")
  orchestrator=$(eval_count_control_pattern 'DEBUG \[orchestrator\].*self_contradiction' "$file")
  n=$((reviewer + orchestrator))
  echo "$n"
}

eval_detect_provider_blocked() {
  local reasons="" file
  for file in "$@"; do
    if [[ -z "$file" || ! -f "$file" ]]; then
      continue
    fi
    # Provider-blocked is an eval classification, not product logic. Match only
    # timestamped control-plane log lines so customer/source text that quotes an
    # LLM error does not turn a product regression into an external outage.
    if LC_ALL=C awk '
      /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ / &&
      $0 !~ / DEBUG \[diag [^]]+\].*ASSISTANT content/ &&
      $0 ~ /(LLM API error \(status 402\)|insufficient_balance_error)/ {
        found = 1
      }
      END { exit found ? 0 : 1 }
    ' "$file"; then
      case ",$reasons," in
        *,insufficient_balance,*) ;;
        *) reasons="${reasons:+$reasons,}insufficient_balance" ;;
      esac
    fi
    if LC_ALL=C awk '
      /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ / &&
      $0 !~ / DEBUG \[diag [^]]+\].*ASSISTANT content/ &&
      $0 ~ /(LLM provider is not configured|没有可用的模型 provider 配置|providers\.yaml: llm\.default\.provider is required)/ {
        found = 1
      }
      END { exit found ? 0 : 1 }
    ' "$file"; then
      case ",$reasons," in
        *,provider_unconfigured,*) ;;
        *) reasons="${reasons:+$reasons,}provider_unconfigured" ;;
      esac
    fi
  done
  echo "$reasons"
}

eval_materialize_partial_run_result() {
  local dir="$1"
  local run_id="${2:-1}"
  local rc="${3:-1}"
  local elapsed="${4:-0}"
  local reason="${5:-worker_exit}"
  if [[ -z "$dir" || ! -d "$dir" ]]; then
    return 0
  fi
  case "$elapsed" in
    ''|*[!0-9]*) elapsed=0 ;;
  esac
  local logdir="$dir/run-${run_id}.logs"
  local all_log="$dir/run-${run_id}.logs.all.log"
  local log=""
  if [[ -d "$logdir" ]]; then
    if [[ ! -s "$all_log" ]]; then
      : >"$all_log"
      local lf found=0
      for lf in "$logdir"/codrax-*.log; do
        [[ -f "$lf" ]] || continue
        found=1
        {
          echo
          echo "===== $lf ====="
          cat "$lf"
        } >>"$all_log"
      done
      if [[ "$found" -ne 1 ]]; then
        rm -f "$all_log"
      fi
    fi
  fi
  if [[ -f "$all_log" ]]; then
    log="$all_log"
  else
    log="$(ls -t "$logdir"/codrax-*.log 2>/dev/null | head -1)"
  fi
  if [[ ! -f "$dir/run-${run_id}.wall" ]]; then
    echo "$elapsed" >"$dir/run-${run_id}.wall"
  fi
  if [[ ! -f "$dir/run-${run_id}.metrics.txt" ]]; then
    {
      echo "exit_code=$rc"
      echo "log_file=$log"
      echo "partial_result=1"
      echo "partial_reason=$reason"
      echo "data_terminal_path="
      echo "data_terminal_status="
      echo "data_rounds=0"
      echo "data_repair_rounds=0"
      echo "data_record_count=0"
      echo "data_action_failed=0"
      echo "data_answer_len=0"
      echo "data_result_summary="
      echo "tool_read_file=$(eval_count_tool_calls "$log" read_file)"
      echo "tool_repo_map=$(eval_count_tool_calls "$log" repo_map)"
      echo "tool_list_files=$(eval_count_tool_calls "$log" list_files)"
      echo "tool_trace_query=$(eval_count_tool_calls "$log" trace_query)"
      echo "tool_mcp_read_resource=$(eval_count_tool_calls "$log" mcp_read_resource)"
      echo "repeated_mcp_resource_reads=$(eval_count_repeated_mcp_resource_reads "$log")"
      echo "mcp_tool_calls=$(eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=toolcall [^:]*tool=[A-Za-z0-9_-]+__[A-Za-z0-9_-]+' "$log")"
      echo "source_inventory_lens=$(eval_count_source_inventory_tool_calls "$log")"
      echo "repo_lens_discovery_hints=$(eval_count_repo_lens_discovery_hints "$log")"
      echo "transient_retry_checkpoints=$(eval_count_transient_retry_checkpoints "$log")"
      echo "unavailable_tool_attempts=$(eval_count_unavailable_tool_attempts "$log")"
      echo "checkpoint_continuation_broad_hint=$(eval_count_checkpoint_continuation_broad_hint "$log")"
      echo "closure_only_repeated=$(eval_count_closure_only_repeated "$log")"
      echo "mermaid_source_repair_applied=$(eval_count_mermaid_source_repairs "$log")"
      echo "repair_debt_checkpoints=$(eval_count_repair_debt_checkpoints "$log")"
      echo "repair_debt_close_ready_filters=$(eval_count_repair_debt_close_ready_filters "$log")"
      echo "repair_debt_principal_blocking_max=$(eval_max_repair_debt_checkpoint_class "$log" principal_blocking)"
      echo "repair_debt_surgical_grounding_max=$(eval_max_repair_debt_checkpoint_class "$log" surgical_grounding)"
      echo "repair_debt_advisory_max=$(eval_max_repair_debt_checkpoint_class "$log" advisory)"
      echo "tool_history_prunes=$(eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=prune TOOL HISTORY PRUNED' "$log")"
      echo "max_context_tokens_est=$(eval_max_context_tokens_estimate "$log")"
      echo "max_context_window=$(eval_max_context_window_tokens "$log")"
      echo "max_context_window_pct=$(eval_max_context_window_pct "$log")"
      echo "answer_contract_violations=$(eval_sum_answer_contract_violations "$log")"
      echo "answer_contract_strict_violations=$(eval_sum_answer_contract_strict_violations "$log")"
      echo "answer_contract_advisories=$(eval_sum_answer_contract_advisories "$log")"
      echo "answer_contract_first_pass_strict_violations=$(eval_first_pass_answer_contract_strict_violations "$log")"
      echo "answer_contract_final_strict_violations=$(eval_final_answer_contract_strict_violations "$log")"
      echo "answer_contract_auto_repaired_strict_violations=$(eval_auto_repaired_answer_contract_strict_violations "$log")"
      echo "answer_contract_lane_block_kind_violations=$(eval_sum_answer_contract_violations_for_section "$log" lane_block_kind)"
      echo "answer_contract_lane_block_kind_strict_violations=$(eval_sum_answer_contract_strict_violations_for_section "$log" lane_block_kind)"
      echo "answer_contract_lane_block_kind_advisories=$(eval_sum_answer_contract_advisories_for_section "$log" lane_block_kind)"
      echo "answer_contract_lane_block_kind_first_pass_strict_violations=$(eval_first_pass_answer_contract_strict_violations_for_section "$log" lane_block_kind)"
      echo "answer_contract_lane_block_kind_final_strict_violations=$(eval_final_answer_contract_strict_violations_for_section "$log" lane_block_kind)"
      echo "answer_contract_lane_block_kind_auto_repaired_strict_violations=$(eval_auto_repaired_answer_contract_strict_violations_for_section "$log" lane_block_kind)"
      echo "midloop_inject=$(eval_count_midloop_injects "$log")"
      echo "analyze_refine_dispatches=$(eval_count_analyze_refine_dispatches "$log")"
      echo "read_loop_add_proof_selected=$(eval_count_read_loop_add_proof_selected "$log")"
      echo "read_loop_add_proof_consumed=$(eval_count_read_loop_add_proof_consumed "$log")"
      echo "parallel_sibling_skips=$(eval_count_pattern 'skipping non-winning parallel explore sibling' "$log")"
      echo "mixed_origin_autocomplete_blocks=$(eval_count_pattern 'accepted investigation closure cannot auto-complete mixed-origin explore window' "$log")"
      echo "finalizer_rejects=$(eval_count_finalizer_rejects "$log")"
      echo "wall_seconds=$(cat "$dir/run-${run_id}.wall" 2>/dev/null || echo 0)"
      echo "pipeline_dispatches=$(eval_count_control_pattern 'DEBUG \[diag [^]]+\] DISPATCH stage=' "$log")"
      echo "investigation_complete_calls=$(eval_count_tool_calls "$log" emit_investigation_complete)"
      echo "investigation_complete_rejects=$(eval_count_tool_rejects "$log" emit_investigation_complete)"
      echo "hypothesis_verdict_rejects=$(eval_count_tool_rejects "$log" emit_hypothesis_verdict)"
      echo "finalizer_rewrites=$(eval_count_finalizer_rewrites "$log")"
      echo "analyzer_iters=$(eval_count_agent_iterations "$log" analyzer)"
      echo "explorer_iters=$(eval_count_agent_iterations "$log" explorer)"
      echo "extractor_iters=$(eval_count_agent_iterations "$log" extractor)"
      echo "finalizer_iters=$(eval_count_agent_iterations "$log" finalizer)"
      echo "analyzer_dispatches=$(eval_count_agent_dispatches "$log" analyzer)"
      echo "explorer_dispatches=$(eval_count_agent_dispatches "$log" explorer)"
      echo "extractor_dispatches=$(eval_count_agent_dispatches "$log" extractor)"
      echo "finalizer_dispatches=$(eval_count_agent_dispatches "$log" finalizer)"
      echo "repair_plan_lines=$(eval_count_pattern 'repair_plan: ' "$log")"
      echo "repair_exec_lines=$(eval_count_pattern 'repair_exec: ' "$log")"
      echo "repair_exec_promote=$(eval_count_pattern 'repair_exec: .*remaining=[1-9].*budget_downgrade=true' "$log")"
      echo "repair_exec_failloud=$(eval_count_pattern 'repair_exec: .*fail_loud=true' "$log")"
      echo "semantic_quality_dispatches=$(eval_count_semantic_quality_dispatches "$log")"
      echo "semantic_quality_concerns=$(eval_count_semantic_quality_concerns "$log")"
      echo "self_consistency_concerns=$(eval_count_self_consistency_concerns "$log")"
      echo "strict_decode_remap_events=$(eval_count_pattern 'strict_decode_remap.*misplaced field' "$log")"
      echo "strict_decode_carrier_events=$(eval_count_pattern 'strict_decode_remap. string-carrier field' "$log")"
      echo "strict_decode_element_shape_events=$(eval_count_pattern 'strict_decode_remap. array-element shape field' "$log")"
    } >"$dir/run-${run_id}.metrics.txt"
  fi
  if [[ ! -f "$dir/run-${run_id}.verdict" ]]; then
    if [[ "$rc" -eq 124 ]]; then
      printf 'TIMEOUT %s\n' "$reason" >"$dir/run-${run_id}.verdict"
    else
      printf 'FAIL %s rc=%s\n' "$reason" "$rc" >"$dir/run-${run_id}.verdict"
    fi
  fi
}

eval_provider_preflight() {
  local codrax_bin="$1"
  local repo_root="$2"
  local outdir="$3"
  local timeout_seconds="${4:-120}"
  if [[ -z "$codrax_bin" || ! -x "$codrax_bin" ]]; then
    echo "provider_unconfigured"
    return 0
  fi
  mkdir -p "$outdir/logs"
  local providers=()
  if [[ -f "$repo_root/providers.yaml" ]]; then
    providers=(--providers "$repo_root/providers.yaml")
  fi
  local rc=0
  eval_run_with_timeout "$timeout_seconds" "$codrax_bin" \
    "${providers[@]}" \
    --repo "$repo_root" \
    --multi-repo=false \
    --chitchat-classifier=true \
    --log-dir "$outdir/logs" \
    --log-level debug \
    --request "你好" \
    >"$outdir/preflight.out" 2>"$outdir/preflight.err" || rc=$?
  local logs=()
  while IFS= read -r f; do
    logs+=("$f")
  done < <(ls "$outdir"/logs/codrax-*.log 2>/dev/null || true)
  local blocked
  blocked="$(eval_detect_provider_blocked "${logs[@]}")"
  if [[ -n "$blocked" ]]; then
    echo "$blocked"
    return 0
  fi
  if LC_ALL=C grep -aqE '(insufficient_balance_error|LLM API error \(status 402\))' "$outdir/preflight.err" "$outdir/preflight.out" 2>/dev/null; then
    echo "insufficient_balance"
    return 0
  fi
  if LC_ALL=C grep -aqE '(LLM provider is not configured|没有可用的模型 provider 配置|providers\.yaml: llm\.default\.provider is required)' "$outdir/preflight.err" "$outdir/preflight.out" 2>/dev/null; then
    echo "provider_unconfigured"
    return 0
  fi
  if [[ "$rc" -eq 124 ]]; then
    echo "provider_preflight_timeout"
    return 0
  fi
  echo ""
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

# eval_count_tool_rejects <log> <tool> — control-plane count of
# explicit tool-level rejections (phase=toolresult ok=false) for one
# emit tool. Loop-churn diagnosis: each reject costs one agent round.
eval_count_tool_rejects() {
  local file="$1" tool="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$tool" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag [^]]+\\][^:]*phase=toolresult TOOLRESULT ${tool} ok=false" "$file"
}
