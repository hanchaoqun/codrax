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

# eval_archive_output_artifacts <repo-root> — ARTIFACT-KEEP mechanical guard
# (PIN-1 B7, ledger §29.65 / §29.60.2 第三犯, 2026-07-13).
#
# Root cause: eval runs execute codrax with CWD at the repo root, so every run
# writes its report into <root>/.codrax/output/ — and the product's retention
# prune (outputdump.PruneDir, keeps the newest N) then deletes the OLDEST
# dumps there. A gold sweep floods that directory and silently destroys the
# operator's replay/witness artifacts (three incidents: gold sweep ate a
# witness dump; log rotation ate 6/7 morning witness logs; §29.55.5 F-item).
#
# Guard shape: BEFORE any run starts, copy (cp -pn: no-clobber, preserve
# mtimes) every existing dump into <root>/.codrax/output_archive/. Canonical
# dump names are timestamp+pid unique, so a flat no-clobber archive is
# idempotent and race-tolerant under parallel sweeps. The archive is
# APPEND-ONLY by design — nothing in the harness ever deletes from it; the
# operator prunes it manually. Never fails the run (guard, not gate).
eval_archive_output_artifacts() {
  local root="$1"
  local outdir="$root/.codrax/output"
  local archive="$root/.codrax/output_archive"
  [[ -d "$outdir" ]] || return 0
  local have=0 f
  for f in "$outdir"/*.md "$outdir"/*.html; do
    [[ -f "$f" ]] && { have=1; break; }
  done
  [[ "$have" -eq 1 ]] || return 0
  mkdir -p "$archive" 2>/dev/null || return 0
  for f in "$outdir"/*.md "$outdir"/*.html; do
    [[ -f "$f" ]] || continue
    cp -pn "$f" "$archive/" 2>/dev/null || true
  done
  return 0
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

# eval_dynamic_scalar_reasons <answer> <principal> <primary> <repo-root>
#                             <receipt-path> <mode>
#
# Eval-only typed oracle for values that must be recomputed from the exact
# checkout under test. A case declares EXPECT_DYNAMIC_SCALARS plus, per ID:
#
#   EXPECT_DYNAMIC_SCALAR_COMMAND_<ID>       command that prints one uint
#   EXPECT_DYNAMIC_SCALAR_DATA_SCOPE_<ID>    human/audit scope provenance
#   EXPECT_DYNAMIC_SCALAR_SURFACE_<ID>       primary[_text]|principal[_text]|
#                                             answer[_text] (primary)
#   EXPECT_DYNAMIC_SCALAR_BINDING_REGEX_<ID> ERE containing {{VALUE}}
#
# The command is trusted versioned eval input (case files are already sourced
# shell), never product/model input. Its result is written to an audit receipt
# and only influences the opt-in eval verdict; it does not enter Codrax
# routing, prompts, answer validation, or answer mutation.
eval_dynamic_scalar_reasons() {
  local answer="$1" principal="$2" primary="$3" repo_root="$4"
  local receipt="$5" mode="$6"
  local scalar scalar_key command_var command data_scope_var data_scope
  local surface_var surface binding_var binding output rc value checked regex
  : >"$receipt"
  [[ -n "${EXPECT_DYNAMIC_SCALARS:-}" ]] || return 0
  if [[ -n "$mode" && "$mode" != "read" ]]; then
    printf 'dynamic_scalar_oracle_requires_read_mode\n'
    return 0
  fi
  if [[ -z "$repo_root" || ! -d "$repo_root" ]]; then
    printf 'dynamic_scalar_repo_root_missing\n'
    return 0
  fi
  for scalar in $EXPECT_DYNAMIC_SCALARS; do
    scalar_key="$(eval_env_key "$scalar")"
    command_var="EXPECT_DYNAMIC_SCALAR_COMMAND_${scalar_key}"
    command="${!command_var:-}"
    data_scope_var="EXPECT_DYNAMIC_SCALAR_DATA_SCOPE_${scalar_key}"
    data_scope="${!data_scope_var:-}"
    surface_var="EXPECT_DYNAMIC_SCALAR_SURFACE_${scalar_key}"
    surface="${!surface_var:-primary}"
    binding_var="EXPECT_DYNAMIC_SCALAR_BINDING_REGEX_${scalar_key}"
    binding="${!binding_var:-}"
    if [[ -z "$command" ]]; then
      printf 'dynamic_scalar_command_missing:%s\n' "$scalar"
      continue
    fi
    if [[ -z "$data_scope" ]]; then
      printf 'dynamic_scalar_data_scope_missing:%s\n' "$scalar"
      continue
    fi
    if [[ -z "$binding" || "$binding" != *'{{VALUE}}'* ]]; then
      printf 'dynamic_scalar_binding_invalid:%s\n' "$scalar"
      continue
    fi
    case "$surface" in
      answer) checked="$answer" ;;
      answer_text) checked="$(LC_ALL=C tr '\n\r\t' '   ' <<<"$answer")" ;;
      principal) checked="$principal" ;;
      principal_text) checked="$(LC_ALL=C tr '\n\r\t' '   ' <<<"$principal")" ;;
      primary) checked="$primary" ;;
      primary_text) checked="$(LC_ALL=C tr '\n\r\t' '   ' <<<"$primary")" ;;
      *)
        printf 'dynamic_scalar_surface_invalid:%s:%s\n' "$scalar" "$surface"
        continue
        ;;
    esac
    output="$(cd "$repo_root" && /bin/bash -c "$command" 2>/dev/null)"
    rc=$?
    value="$(eval_trim "$output")"
    if [[ $rc -ne 0 ]]; then
      printf 'dynamic_scalar_command_failed:%s:%s\n' "$scalar" "$rc"
      continue
    fi
    if ! eval_is_uint "$value" || [[ "$output" == *$'\n'* ]]; then
      printf 'dynamic_scalar_output_invalid:%s\n' "$scalar"
      continue
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$scalar" "$value" "$surface" "$data_scope" "$repo_root" \
      "$(printf '%s' "$command" | tr '\t\r\n' '   ')" >>"$receipt"
    regex="${binding//'{{VALUE}}'/$value}"
    if ! LC_ALL=C grep -aEq -- "$regex" <<<"$checked"; then
      printf 'dynamic_scalar_binding_missing:%s:%s\n' "$scalar" "$value"
    fi
  done
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

# eval_total_repair_rounds <metrics>
#
# Selected/priority/all sweep tables have one compact `repair` column. Read
# pipeline repair plans and data-lane repair rounds are disjoint typed runtime
# counters, so reporting only repair_plan_lines makes a data run with real
# replanning look like repair=0. Sum the two control-plane counters; do not
# infer repairs from answer text or generic warning prose.
eval_total_repair_rounds() {
  local file="$1"
  local read_repairs data_repairs
  read_repairs="$(eval_metric_int_field "$file" repair_plan_lines)"
  data_repairs="$(eval_metric_int_field "$file" data_repair_rounds)"
  echo $((read_repairs + data_repairs))
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

# eval_count_degraded_read_answer_check_skips <log-file>
#
# Counts the orchestrator's stable control-plane event for a read answer that
# shipped with SkipAnswerChecks=true. The prefix is deliberately anchored to
# the orchestrator WARN record: model/user prose that quotes the same reason
# token cannot affect an eval verdict.
eval_count_degraded_read_answer_check_skips() {
  local file="$1"
  eval_count_pattern '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[^[:space:]]+[[:space:]]+WARN \[orchestrator\] finalizer returned degraded answer; skipping structured answer checks reason=[^[:space:]]+' "$file"
}

# eval_pipe_list_any_matches <regex> <pipe-delimited-values>
#
# Operation terminal events serialize typed repeated fields with the exact
# separator " | ".  Match each member independently so an anchored regex for
# one receipt/locator does not become an accidental whole-list regex.  This is
# an eval-side typed-field decoder; answer/user prose never reaches it.
eval_pipe_list_any_matches() {
  local pattern="$1"
  local remaining="$2"
  local member

  while true; do
    if [[ "$remaining" == *" | "* ]]; then
      member="${remaining%%" | "*}"
      remaining="${remaining#*" | "}"
    else
      member="$remaining"
      remaining=""
    fi
    if LC_ALL=C grep -aEq -- "$pattern" <<<"$member"; then
      return 0
    fi
    [[ -n "$remaining" ]] || return 1
  done
}

# EVAL-B73-OPEVAL1 / B74-OPREPAUTH1: inspect the last system-authored typed
# operation evaluation event. Intermediate rounds cannot green-light a later
# partial terminal, and answer prose never participates in this oracle.
eval_operation_terminal_reasons() {
  local file="$1"
  local receipt_file="$2"
  local expected_status="${EXPECT_OPERATION_TERMINAL_STATUS:-}"
  local expected_coverage="${EXPECT_OPERATION_MATERIAL_COVERAGE_STATUS:-}"
  local expected_ref_regex="${EXPECT_OPERATION_COVERAGE_REF_REGEX:-}"
  local expected_source_regex="${EXPECT_OPERATION_COVERAGE_SOURCE_REGEX:-}"
  local line actual_status actual_coverage actual_refs actual_source_refs actual_source_identities actual_source_locators

  if [[ -z "$expected_status$expected_coverage$expected_ref_regex$expected_source_regex" ]]; then
    return 0
  fi
  if [[ -z "$expected_status" ]]; then
    echo "operation_terminal_expected_status_missing"
    return 0
  fi
  case "$expected_status" in
    complete|blocked|budget_exhausted|partial_answer_possible|needs_approval|needs_clarification|continue_command|continue_provider) ;;
    *)
      echo "operation_terminal_expected_status_invalid:$(eval_reason_slug "$expected_status")"
      return 0
      ;;
  esac
  if [[ -n "$expected_coverage" ]]; then
    case "$expected_coverage" in
      complete|partial|not_applicable|not_evaluated) ;;
      *)
        echo "operation_material_coverage_expected_status_invalid:$(eval_reason_slug "$expected_coverage")"
        return 0
        ;;
    esac
  fi
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo "operation_terminal_log_missing"
    return 0
  fi

  line="$(LC_ALL=C grep -aE '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[^[:space:]]+[[:space:]]+INFO \[(cli|repl)/operation\] command evaluation status=' "$file" | tail -n 1 || true)"
  if [[ -z "$line" ]]; then
    echo "operation_terminal_event_missing"
    return 0
  fi
  actual_status="$(LC_ALL=C sed -E 's/.* command evaluation status=([^[:space:]]+).*/\1/' <<<"$line")"
  actual_coverage="$(LC_ALL=C sed -E 's/.* material_coverage_status=([^[:space:]]+).*/\1/' <<<"$line")"
  if [[ "$actual_coverage" == "$line" ]]; then
    actual_coverage=""
  fi
  actual_refs="$(LC_ALL=C sed -E 's/.* coverage_material_refs="([^"]*)".*/\1/' <<<"$line")"
  if [[ "$actual_refs" == "$line" ]]; then
    actual_refs=""
  fi
  actual_source_refs="$(LC_ALL=C sed -E 's/.* coverage_source_refs="([^"]*)".*/\1/' <<<"$line")"
  [[ "$actual_source_refs" == "$line" ]] && actual_source_refs=""
  actual_source_identities="$(LC_ALL=C sed -E 's/.* coverage_source_identities="([^"]*)".*/\1/' <<<"$line")"
  [[ "$actual_source_identities" == "$line" ]] && actual_source_identities=""
  actual_source_locators="$(LC_ALL=C sed -E 's/.* coverage_source_locators="([^"]*)".*/\1/' <<<"$line")"
  [[ "$actual_source_locators" == "$line" ]] && actual_source_locators=""

  {
	printf 'expected_status\tactual_status\texpected_material_coverage\tactual_material_coverage\tcoverage_material_refs\tcoverage_source_refs\tcoverage_source_identities\tcoverage_source_locators\n'
	printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$expected_status" "$actual_status" "$expected_coverage" "$actual_coverage" "$actual_refs" "$actual_source_refs" "$actual_source_identities" "$actual_source_locators"
  } >"$receipt_file"

  if [[ "$actual_status" != "$expected_status" ]]; then
    echo "operation_terminal_status:$actual_status:expected:$expected_status"
  fi
  if [[ -n "$expected_coverage" && "$actual_coverage" != "$expected_coverage" ]]; then
    echo "operation_material_coverage_status:${actual_coverage:-missing}:expected:$expected_coverage"
  fi
  if [[ -n "$expected_ref_regex" ]] && ! eval_pipe_list_any_matches "$expected_ref_regex" "$actual_refs"; then
    echo "operation_coverage_ref_missing:$(eval_reason_slug "$expected_ref_regex")"
  fi
  if [[ -n "$expected_source_regex" ]] && ! eval_pipe_list_any_matches "$expected_source_regex" "$actual_source_locators"; then
    echo "operation_coverage_source_missing:$(eval_reason_slug "$expected_source_regex")"
  fi
}

eval_case_oracle_surface() {
  local file="$1"
  local surfaces=""

  eval_case_oracle_surface_add() {
    local item="$1"
    if [[ -z "$surfaces" ]]; then
      surfaces="$item"
    else
      surfaces="${surfaces},${item}"
    fi
  }

  if [[ -z "$file" || ! -f "$file" ]]; then
    echo "unknown"
    return 0
  fi

  if LC_ALL=C grep -aEq '^[[:space:]]*EXPECT_INVENTORY_ROWSETS=' "$file"; then
    eval_case_oracle_surface_add "typed_inventory_rowset"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*EXPECT_DIMENSIONS=' "$file"; then
    eval_case_oracle_surface_add "dimension_substring"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*EXPECT_LOG_MATCHES_REGEX=' "$file"; then
    eval_case_oracle_surface_add "log_regex"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*EXPECT_OPERATION_TERMINAL_STATUS=' "$file"; then
    eval_case_oracle_surface_add "typed_operation_terminal"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*(HTRACE|HTRACE_FILE)=' "$file"; then
    eval_case_oracle_surface_add "trace_attachment"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*(LOG|LOG_FILE)=' "$file"; then
    eval_case_oracle_surface_add "log_attachment"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*MODE=["'\'']?apply' "$file"; then
    eval_case_oracle_surface_add "write_apply"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*MODE=["'\'']?plan' "$file"; then
    eval_case_oracle_surface_add "write_plan"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*(PLAN_EXPECT_REGEX|POST_APPLY_FILE)=' "$file"; then
    eval_case_oracle_surface_add "write_patch_oracle"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*EXPECT_REGEX=' "$file" || {
    LC_ALL=C grep -aEq '^[[:space:]]*EXPECT_MATCHES_REGEX=' "$file" &&
      ! LC_ALL=C grep -aEq '^[[:space:]]*POST_APPLY_FILE=' "$file"
  }; then
    eval_case_oracle_surface_add "answer_regex"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*EXPECT_CONTAINS=' "$file"; then
    eval_case_oracle_surface_add "answer_contains"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*EXPECT_PRINCIPAL_(CONTAINS|NOT_CONTAINS|MATCHES_REGEX|MATCHES_TEXT_REGEX)=' "$file"; then
    eval_case_oracle_surface_add "principal_answer"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*EXPECT_PRIMARY_(CONTAINS|NOT_CONTAINS|MATCHES_REGEX|MATCHES_TEXT_REGEX)=' "$file"; then
    eval_case_oracle_surface_add "primary_answer"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*MAX_[A-Z0-9_]+=' "$file"; then
    eval_case_oracle_surface_add "metric_hard_budget"
  fi
  if LC_ALL=C grep -aEq '^[[:space:]]*ADVISORY_MAX_[A-Z0-9_]+=' "$file"; then
    eval_case_oracle_surface_add "metric_advisory_budget"
  fi

  echo "${surfaces:-basic_output}"
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

# eval_expect_token_regex <token> — prints the ERE used to match one literal
# EXPECT_CONTAINS / EXPECT_NOT_CONTAINS / EXPECT_SECTIONS / dimension /
# inventory token, with digit-boundary guards on digit edges.
#
# EVALFIX h5 (§29.64 / §29.67): plain grep -F substring matching let the
# banned count face `×3` bite the innocent `×39` (and `×2`/`×4` bite
# `×28`/`×40`) — an oracle-precision false FAIL on a structurally fine
# answer. `\b` is not the fix: these tokens sit in CJK/symbol context where
# byte-locale word boundaries do not exist. Instead the boundary is spelled
# as explicit context classes, exactly for the numeric-edge family:
#   - a token that ENDS with a digit must not be followed by another digit
#     (`×3` → `×3([^0-9]|$)` — end-of-line counts as a boundary);
#   - a token that STARTS with a digit must not be preceded by one
#     (`132.041` must not bite `5132.041`).
# Non-digit edges keep plain substring semantics — the token's own edge
# character (×, GHz, 次, %, letters…) is already the boundary marker, and
# tightening those would change long-standing concept-containment matching.
# ERE metacharacters inside the token are escaped so the literal semantics
# of the historical -F channel are preserved.
#
# Hash-prefix carve-out: a token that is a pure hex string of >=7 chars with
# at least one hex LETTER (git short-hash shape, e.g. `aa27be48`) keeps plain
# substring semantics even on digit edges — it names an identity PREFIX of a
# longer hash, so a following hex digit is the same object, not a different
# value (archived u7g PASS answer carries `aa27be488e9e030afc88`; a digit
# guard there would be a false miss).
eval_expect_token_regex() {
  local token="$1" escaped prefix="" suffix="" hash_like=0
  escaped="$(printf '%s' "$token" | LC_ALL=C sed -E 's/[][\.|$(){}?+*^]/\\&/g')"
  case "$token" in
    *[!0-9a-fA-F]*) ;;
    *[a-fA-F]*)
      if [[ "${#token}" -ge 7 ]]; then
        hash_like=1
      fi
      ;;
  esac
  if [[ "$hash_like" -eq 0 ]]; then
    case "$token" in
      [0-9]*) prefix='(^|[^0-9])' ;;
    esac
    case "$token" in
      *[0-9]) suffix='([^0-9]|$)' ;;
    esac
  fi
  printf '%s%s%s' "$prefix" "$escaped" "$suffix"
}

# eval_expect_token_present <token> <text> — case-insensitive containment
# test for one literal expect/banned token over the checked bytes, with the
# digit-boundary semantics of eval_expect_token_regex. Replaces the raw
# `grep -aqiF` sites in run.sh write_verdict and the inventory row matcher.
eval_expect_token_present() {
  local token="$1" text="$2"
  LC_ALL=C grep -aqiE -- "$(eval_expect_token_regex "$token")" <<<"$text"
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
    if ! eval_expect_token_present "$token" "$text"; then
      return 1
    fi
  done
  [[ "$seen" -eq 1 ]]
}

eval_inventory_rowset_label_regex() {
  local rowset="$1"
  local words
  words="$(printf '%s' "$rowset" | LC_ALL=C sed -E 's/[_-]+/ /g; s/^[[:space:]]+//; s/[[:space:]]+$//')"
  printf '%s' "$words" | LC_ALL=C sed -E 's/[[:space:]]+/[[:space:]]+/g'
}

eval_inventory_rowset_section_text() {
  local cleaned="$1"
  local rowset="$2"
  local explicit_label="${3:-}"
  local label_regex match_mode
  label_regex="$(eval_inventory_rowset_label_regex "$rowset")"
  match_mode="regex"
  if [[ -n "$explicit_label" ]]; then
    match_mode="literal"
    label_regex="$explicit_label"
  fi
  awk -v label="$label_regex" -v match_mode="$match_mode" '
    function heading_level(line, trimmed, marks) {
      trimmed = line
      sub(/^[[:space:]]*/, "", trimmed)
      if (trimmed ~ /^#{1,6}[[:space:]]+/) {
        marks = trimmed
        sub(/[[:space:]].*$/, "", marks)
        return length(marks)
      }
      if (trimmed ~ /^(>[[:space:]]*)?\*\*[^*]+\*\*[：:]?[[:space:]]*$/) {
        return 7
      }
      return 0
    }
    BEGIN { in_section = 0; found = 0; selected_level = 0 }
    {
      level = heading_level($0)
      if (in_section && level > 0 && (level <= selected_level || selected_level == 7)) {
        exit
      }
      if (!in_section && level > 0) {
        lower = tolower($0)
        label_lower = tolower(label)
        matches = (match_mode == "literal" ? index(lower, label_lower) > 0 : lower ~ label_lower)
      }
      if (!in_section && level > 0 && matches) {
        in_section = 1
        found = 1
        selected_level = level
        print
        next
      }
    }
    in_section { print }
    END { if (!found) exit 1 }
  ' <<<"$cleaned"
}

eval_inventory_visible_row_count() {
  local section_text="$1"
  awk '
    BEGIN { count = 0; table = 0; table_header_seen = 0 }
    /^[[:space:]]*\|/ {
      line = $0
      compact = line
      gsub(/[[:space:]|:-]/, "", compact)
      if (compact == "") {
        next
      }
      if (!table) {
        table = 1
        table_header_seen = 1
        next
      }
      count++
      next
    }
    {
      table = 0
      table_header_seen = 0
    }
    /^[[:space:]]*[-*+][[:space:]]+/ {
      count++
    }
    /^[[:space:]]*[0-9]+[.)][[:space:]]+/ {
      count++
    }
    END {
      if (count == 0) {
        exit 1
      }
      print count
    }
  ' <<<"$section_text"
}

# eval_inventory_marker_rows <text> <marker> — selects only visible inventory
# presentation rows that carry both a case-declared group marker and an exact
# source location. This is the format-neutral fallback when a correct answer
# groups rows inline instead of emitting a dedicated markdown heading. The
# caller supplies the terminal primary answer, so renderer citations and
# deterministic supplements cannot manufacture membership.
eval_inventory_marker_rows() {
  local text="$1"
  local marker="$2"
  awk -v marker="$marker" '
    BEGIN { found = 0; marker_lower = tolower(marker) }
    {
      if ($0 == "**引用**：" || $0 == "**Citations:**" ||
          $0 == "**关键代码**：" || $0 == "**Key snippets:**") {
        exit
      }
      lower = tolower($0)
      presentation = ($0 ~ /^[[:space:]]*\|/ ||
                      $0 ~ /^[[:space:]]*[-*+][[:space:]]+/ ||
                      $0 ~ /^[[:space:]]*[0-9]+[.)][[:space:]]+/)
      location = ($0 ~ /[^[:space:]|()`]+\.[[:alnum:]_+-]+:[0-9]+/)
      if (presentation && location && index(lower, marker_lower) > 0) {
        print
        found = 1
      }
    }
    END { if (!found) exit 1 }
  ' <<<"$text"
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
  local banned_var banned_rows scope_var row_scope section_var section_label marker_var row_marker rowset_text rowset_scoped marker_scoped old_ifs row matched total reason_row visible_count
  for rowset in $rowsets; do
    rowset_key="$(eval_env_key "$rowset")"
    rows_var="EXPECT_INVENTORY_ROWS_${rowset_key}"
    rows="${!rows_var:-}"
    count_var="EXPECT_INVENTORY_COUNT_${rowset_key}"
    expected_count="${!count_var:-}"
    scope_var="EXPECT_INVENTORY_ROW_SCOPE_${rowset_key}"
    row_scope="${!scope_var:-document}"
    section_var="EXPECT_INVENTORY_SECTION_LABEL_${rowset_key}"
    section_label="${!section_var:-}"
    marker_var="EXPECT_INVENTORY_ROW_MARKER_${rowset_key}"
    row_marker="${!marker_var:-}"
    rowset_scoped=0
    marker_scoped=0
    if rowset_text="$(eval_inventory_rowset_section_text "$cleaned" "$rowset" "$section_label")"; then
      rowset_scoped=1
    # A case-declared row marker is the stable group discriminator; the full
    # section label is presentation copy and may be shortened/localized by a
    # correct answer. Prefer a marker-bearing section before falling back to
    # inline marker rows so sibling inventories cannot satisfy each other.
    elif [[ -n "$row_marker" ]] && rowset_text="$(eval_inventory_rowset_section_text "$cleaned" "$rowset" "$row_marker")"; then
      rowset_scoped=1
    elif [[ -n "$row_marker" ]] && rowset_text="$(eval_inventory_marker_rows "$cleaned" "$row_marker")"; then
      rowset_scoped=1
      marker_scoped=1
    elif [[ -n "$section_label" && -z "$row_marker" ]]; then
      printf 'missing_inventory_section:%s:%s\n' "$rowset" "$(eval_reason_slug "$section_label")"
      continue
    elif [[ -n "$section_label$row_marker" ]]; then
      printf 'missing_inventory_group:%s:%s\n' "$rowset" "$(eval_reason_slug "${row_marker:-$section_label}")"
      continue
    else
      rowset_text="$cleaned"
    fi

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
        if eval_inventory_row_visible "$rowset_text" "$row" "$row_scope"; then
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
    else
      visible_count=""
      if [[ "$rowset_scoped" -eq 1 ]]; then
        if [[ "$marker_scoped" -eq 1 ]]; then
          visible_count="$(LC_ALL=C awk 'NF { count++ } END { if (count > 0) print count }' <<<"$rowset_text")"
        else
          visible_count="$(eval_inventory_visible_row_count "$rowset_text" || true)"
        fi
      fi
      if [[ -n "$visible_count" ]] && [[ "$visible_count" -ne "$expected_count" ]]; then
        printf 'inventory_count_mismatch:%s:got%s:want%s\n' "$rowset" "$visible_count" "$expected_count"
      elif [[ -z "$visible_count" ]] && [[ "$matched" -ne "$expected_count" ]]; then
        printf 'inventory_count_mismatch:%s:got%s:want%s\n' "$rowset" "$matched" "$expected_count"
      fi
    fi

    banned_var="EXPECT_INVENTORY_BANNED_ROWS_${rowset_key}"
    banned_rows="${!banned_var:-}"
    if [[ -n "$banned_rows" ]]; then
      old_ifs="$IFS"
      IFS=$'\n'
      for row in $banned_rows; do
        row="$(eval_trim "$row")"
        [[ -z "$row" ]] && continue
        if eval_inventory_row_visible "$rowset_text" "$row" "$row_scope"; then
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
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$file" "$field" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    value = json.load(handle).get(sys.argv[2])
if not isinstance(value, str):
    raise SystemExit(1)
sys.stdout.write(value)
PY
    return
  fi
  LC_ALL=C sed -nE 's/^  "'"$field"'"[[:space:]]*:[[:space:]]*"([^"]*)",?$/\1/p' "$file" | head -1
}

eval_data_terminal_action_failed_count() {
  local file="$1"
  [[ -f "$file" ]] || { echo 0; return; }
  LC_ALL=C awk '
    /"action_events"[[:space:]]*:[[:space:]]*\[/ { in_actions=1; next }
    in_actions && /"action_graph"[[:space:]]*:/ { in_actions=0 }
    in_actions && /"(status|Status)"[[:space:]]*:[[:space:]]*"failed"/ { count++ }
    END { print count + 0 }
  ' "$file"
}

eval_json_top_bool_field() {
  local file="$1"
  local field="$2"
  [[ -f "$file" ]] || return 1
  LC_ALL=C sed -nE 's/^  "'"$field"'"[[:space:]]*:[[:space:]]*(true|false),?$/\1/p' "$file" | head -1
}

eval_json_nested_string_field() {
  local file="$1"
  shift
  [[ -f "$file" && "$#" -gt 0 ]] || return 1
  python3 - "$file" "$@" <<'PY' 2>/dev/null
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    value = json.load(handle)
for key in sys.argv[2:]:
    if not isinstance(value, dict):
        raise SystemExit(1)
    value = value.get(key)
if not isinstance(value, str):
    raise SystemExit(1)
sys.stdout.write(value)
PY
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

eval_find_write_final_report_path() {
  local plan_path="$1"
  local outdir="$2"
  local scratch="$3"
  local worktree="$4"
  local plan_id="$5"
  local plan_dir candidate
  [[ -n "$plan_id" ]] || return 1
  plan_dir="$(dirname "$plan_path")"
  for candidate in \
    "$outdir/${plan_id}.final.json" \
    "$plan_dir/${plan_id}.final.json" \
    "$scratch/.codrax/plans/${plan_id}.final.json" \
    "$worktree/.codrax/plans/${plan_id}.final.json"
  do
    if [[ -n "$candidate" && -f "$candidate" ]]; then
      printf '%s' "$candidate"
      return 0
    fi
  done
  candidate="$(find "$outdir" -maxdepth 1 -type f -name "${plan_id}.final.json" 2>/dev/null | sort | tail -1)"
  if [[ -n "$candidate" && -f "$candidate" ]]; then
    printf '%s' "$candidate"
    return 0
  fi
  return 1
}

eval_materialize_write_apply_source() {
  # Durable-delivery-first (eval-audit 20260719 GAP-2): EXPECT must judge
  # the bytes a /merge-by-ref or cherry-pick would actually land — the
  # applied_commit_sha / refs/codrax/applied/<id> chain — NOT the live
  # worktree. A live worktree can carry uncommitted applied bytes that
  # mask a broken durable chain (zod run-1: the worktree had the
  # implementation fix, the ref did not, and EXPECT green-lit a broken
  # delivery). The worktree is only a fallback when no durable commit
  # resolves; run.sh flags that fallback as a verdict reason.
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
  if [[ -n "$scratch" && -d "$scratch/.git" && -n "$plan_id" ]]; then
    # Multi-plan sessions pin ONE ref per applied plan. A cold-discard
    # session (worktree gone, checkpoints NOT chained onto each other)
    # delivers the UNION of every refs/codrax/applied/* in apply order —
    # the exact byte set an in-order `git cherry-pick refA..refN` (the
    # G3 landing guidance) reproduces. Materializing only the newest ref
    # would judge a complete delivery as red (rework P2-1). Exactly one
    # ref keeps the original single-commit lane below unchanged.
    local union_refs=() r
    while IFS= read -r r; do
      [[ -n "$r" ]] && union_refs+=("$r")
    done < <(git -C "$scratch" for-each-ref --sort=refname --sort=committerdate --format='%(refname)' 'refs/codrax/applied' 2>/dev/null)
    if [[ "${#union_refs[@]}" -ge 2 ]]; then
      # The final plan's applied_commit_sha may exist without its ref
      # (ref tag failed after the commit landed): layer it last, at its
      # apply position, when no enumerated ref covers it.
      if [[ -n "$applied_sha" ]] && git -C "$scratch" cat-file -e "${applied_sha}^{commit}" >/dev/null 2>&1; then
        local sha_full sha_covered=0
        sha_full="$(git -C "$scratch" rev-parse "${applied_sha}^{commit}" 2>/dev/null || true)"
        for r in "${union_refs[@]}"; do
          if [[ "$(git -C "$scratch" rev-parse "${r}^{commit}" 2>/dev/null || true)" == "$sha_full" ]]; then
            sha_covered=1
            break
          fi
        done
        [[ "$sha_covered" == 1 ]] || union_refs+=("$applied_sha")
      fi
      dest="$outdir/run-${run_id}.applied-tree"
      rm -rf "$dest"
      mkdir -p "$dest"
      # Cherry-pick semantics: full tree of the FIRST checkpoint, then
      # per-ref overlay of only the paths THAT commit touched (vs its
      # parent). A full-tree overlay of a sibling (non-chained) commit
      # would revert an earlier plan's fix back to base bytes. Deleted
      # paths are skipped (archive cannot carry a deletion) — a minor
      # infidelity the eval oracles do not depend on.
      local union_ok=1 union_first=1 changed p
      local -a changed_paths=()
      for r in "${union_refs[@]}"; do
        if [[ "$union_first" == 1 ]]; then
          union_first=0
          if ! git -C "$scratch" archive "$r" | tar -x -C "$dest"; then
            union_ok=0
            break
          fi
          continue
        fi
        changed="$(git -C "$scratch" diff-tree --no-commit-id --name-only --diff-filter=d -r "$r" 2>/dev/null || true)"
        [[ -n "$changed" ]] || continue
        changed_paths=()
        while IFS= read -r p; do
          [[ -n "$p" ]] && changed_paths+=("$p")
        done <<<"$changed"
        if ! git -C "$scratch" archive "$r" -- "${changed_paths[@]}" | tar -x -C "$dest"; then
          union_ok=0
          break
        fi
      done
      if [[ "$union_ok" == 1 ]]; then
        printf '%s\n' "$dest"
        return 0
      fi
      rm -rf "$dest"
    fi
    if [[ -n "$applied_sha" ]] && git -C "$scratch" cat-file -e "${applied_sha}^{commit}" >/dev/null 2>&1; then
      commit="$applied_sha"
    elif git -C "$scratch" cat-file -e "refs/codrax/applied/${plan_id}^{commit}" >/dev/null 2>&1; then
      commit="refs/codrax/applied/${plan_id}"
    fi
    if [[ -n "$commit" ]]; then
      dest="$outdir/run-${run_id}.applied-tree"
      rm -rf "$dest"
      mkdir -p "$dest"
      if git -C "$scratch" archive "$commit" | tar -x -C "$dest"; then
        printf '%s\n' "$dest"
        return 0
      fi
      rm -rf "$dest"
    fi
  fi
  if [[ -n "$worktree" && -d "$worktree" ]]; then
    printf '%s\n' "$worktree"
    return 0
  fi
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
  local final_path="" final_exists=false final_plan_id="" final_run_status="" final_verdict="" final_reason_code=""

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
  final_path="$(eval_find_write_final_report_path "$plan_path" "$outdir" "$scratch" "$worktree" "$plan_id" || true)"
  if [[ -n "$final_path" && -f "$final_path" ]]; then
    final_exists=true
    final_plan_id="$(eval_json_nested_string_field "$final_path" plan id || true)"
    final_run_status="$(eval_json_top_string_field "$final_path" run_status || true)"
    final_verdict="$(eval_json_nested_string_field "$final_path" completion verdict || true)"
    final_reason_code="$(eval_json_nested_string_field "$final_path" completion reason_code || true)"
  fi
  if [[ "$report_exists" == true && "$report_plan_id" == "$plan_id" && "$report_channel" == "post_apply_verify" && "$report_passed" == "true" && "$final_exists" == true && "$final_plan_id" == "$plan_id" && "$final_run_status" == "complete" && "$final_verdict" == "verified" ]]; then
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
    printf '  "final_report_path": "%s",\n' "$(eval_json_escape "$final_path")"
    printf '  "final_report_exists": %s,\n' "$final_exists"
    printf '  "final_plan_id": "%s",\n' "$(eval_json_escape "$final_plan_id")"
    printf '  "final_run_status": "%s",\n' "$(eval_json_escape "$final_run_status")"
    printf '  "final_verdict": "%s",\n' "$(eval_json_escape "$final_verdict")"
    printf '  "final_reason_code": "%s",\n' "$(eval_json_escape "$final_reason_code")"
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
  # The tool-result line and the user-facing render line are two mirrors of
  # the SAME reject. Use the larger control-plane census so a build that omits
  # either mirror remains observable without double-counting ordinary runs.
  if [[ "$tool" -ge "$render" ]]; then
    n="$tool"
  else
    n="$render"
  fi
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

eval_count_stage_dispatches() {
  local file="$1"
  local stage="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$stage" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag [^]]+\\] DISPATCH stage=${stage}( |$)" "$file"
}

eval_runtime_attachment_kind_from_log() {
  local file="$1"
  local log_triage perf_triage trace_query emit_log emit_perf
  log_triage="$(eval_count_stage_dispatches "$file" log_triage)"
  perf_triage="$(eval_count_stage_dispatches "$file" perf_triage)"
  trace_query="$(eval_count_tool_calls "$file" trace_query)"
  emit_log="$(eval_count_tool_calls "$file" emit_log_triage)"
  emit_perf="$(eval_count_tool_calls "$file" emit_perf_trace)"
  if [[ "${perf_triage:-0}" -gt 0 || "${emit_perf:-0}" -gt 0 || "${trace_query:-0}" -gt 0 ]]; then
    echo trace
    return
  fi
  if [[ "${log_triage:-0}" -gt 0 || "${emit_log:-0}" -gt 0 ]]; then
    echo log
    return
  fi
  echo none
}

eval_runtime_authority_path() {
  local attachment="$1"
  local file="$2"
  local log_triage perf_triage trace_query emit_log emit_perf
  attachment="${attachment:-none}"
  log_triage="$(eval_count_stage_dispatches "$file" log_triage)"
  perf_triage="$(eval_count_stage_dispatches "$file" perf_triage)"
  trace_query="$(eval_count_tool_calls "$file" trace_query)"
  emit_log="$(eval_count_tool_calls "$file" emit_log_triage)"
  emit_perf="$(eval_count_tool_calls "$file" emit_perf_trace)"
  case "$attachment" in
    log)
      if [[ "${log_triage:-0}" -gt 0 || "${emit_log:-0}" -gt 0 ]]; then
        echo log_triage
      elif [[ "${trace_query:-0}" -gt 0 ]]; then
        echo trace_query
      else
        echo missing_runtime_authority
      fi
      ;;
    trace)
      if [[ "${trace_query:-0}" -gt 0 && ( "${perf_triage:-0}" -gt 0 || "${emit_perf:-0}" -gt 0 ) ]]; then
        echo perf_triage+trace_query
      elif [[ "${trace_query:-0}" -gt 0 ]]; then
        echo trace_query
      elif [[ "${perf_triage:-0}" -gt 0 || "${emit_perf:-0}" -gt 0 ]]; then
        echo perf_triage
      else
        echo missing_runtime_authority
      fi
      ;;
    *)
      echo none
      ;;
  esac
}

eval_count_trace_query_view_family() {
  local file="$1"
  local views="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$views" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag [^]]+\\][^:]*phase=toolcall [^:]*tool=trace_query params=.*\"view\"[[:space:]]*:[[:space:]]*\"(${views})\"" "$file"
}

eval_count_trace_query_windowed_calls() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=toolcall [^:]*tool=trace_query params=.*"(time_start|timeStart|time_end|timeEnd|line_start|lineStart|line_end|lineEnd)"[[:space:]]*:' "$file"
}

eval_count_trace_query_pid_filtered_calls() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=toolcall [^:]*tool=trace_query params=.*"pid"[[:space:]]*:' "$file"
}

eval_count_trace_query_thread_filtered_calls() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=toolcall [^:]*tool=trace_query params=.*"thread"[[:space:]]*:' "$file"
}

eval_count_trace_query_target_inherited() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag [^]]+\].*trace_query_target_inherited=true' "$file"
}

eval_count_trace_query_final_projection_blocks() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Count the deterministic answer block, not every mention of its title.
  # Operation/data answers can legitimately quote manuals, HTML titles, or
  # model progress containing the same words; treating those as a published
  # trace projection contaminates cross-mode audits.
  awk '
    /^##[[:space:]]+(Trace 因果投影|Trace Causal Projection)([[:space:]]+—.*)?[[:space:]]*$/ { n++ }
    END { print n + 0 }
  ' "$file"
}

eval_count_trace_query_dimension_families() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  local n=0
  if [[ "$(eval_count_trace_query_view_family "$file" 'root_cause_rank|frame_root_cause_bundle|frame_bundle')" -gt 0 ]]; then
    n=$((n + 1))
  fi
  if [[ "$(eval_count_trace_query_view_family "$file" 'wakeup_chain|causal_impact|frame_root_cause_bundle|frame_bundle')" -gt 0 ]]; then
    n=$((n + 1))
  fi
  if [[ "$(eval_count_trace_query_view_family "$file" 'critical_blocking_calls|ipc_graph|frame_root_cause_bundle|frame_bundle')" -gt 0 ]]; then
    n=$((n + 1))
  fi
  if [[ "$(eval_count_trace_query_view_family "$file" 'window_stats|thread_timeline|scheduler_latency_stats|frame_root_cause_bundle|frame_bundle')" -gt 0 ]]; then
    n=$((n + 1))
  fi
  if [[ "$(eval_count_trace_query_view_family "$file" 'window_stats|frame_root_cause_bundle|frame_bundle')" -gt 0 ]]; then
    n=$((n + 1))
  fi
  echo "$n"
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

# STYLE-1 (§29.96.3) observation-only column: sums the ai_style_hits=N
# values from the orchestrator's answer-style advisory WARN lines
# (Chinese AI-register filler phrases counted in the final answer).
# Pure human-read observation — MUST NOT feed any verdict, gate, or
# pass/fail decision (precise-signals red line: noisy signal, soft
# surface only).
eval_count_ai_style_hits() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  grep -Eo 'ai_style_hits=[0-9]+' "$file" 2>/dev/null | cut -d= -f2 | awk '{s+=$1} END {print s+0}'
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
      echo "trace_query_dimension_families=$(eval_count_trace_query_dimension_families "$log")"
      echo "trace_query_root_cause_views=$(eval_count_trace_query_view_family "$log" 'root_cause_rank|frame_root_cause_bundle|frame_bundle')"
      echo "trace_query_wakeup_views=$(eval_count_trace_query_view_family "$log" 'wakeup_chain|causal_impact|frame_root_cause_bundle|frame_bundle')"
      echo "trace_query_blocking_views=$(eval_count_trace_query_view_family "$log" 'critical_blocking_calls|ipc_graph|frame_root_cause_bundle|frame_bundle')"
      echo "trace_query_timeline_views=$(eval_count_trace_query_view_family "$log" 'window_stats|thread_timeline|scheduler_latency_stats|frame_root_cause_bundle|frame_bundle')"
      echo "trace_query_resource_views=$(eval_count_trace_query_view_family "$log" 'window_stats|frame_root_cause_bundle|frame_bundle')"
      echo "trace_query_windowed_calls=$(eval_count_trace_query_windowed_calls "$log")"
      echo "trace_query_pid_filtered_calls=$(eval_count_trace_query_pid_filtered_calls "$log")"
      echo "trace_query_thread_filtered_calls=$(eval_count_trace_query_thread_filtered_calls "$log")"
      echo "trace_query_target_inherited=$(eval_count_trace_query_target_inherited "$log")"
      echo "trace_query_final_projection_blocks=0"
      runtime_attachment_kind="$(eval_runtime_attachment_kind_from_log "$log")"
      log_triage_dispatches="$(eval_count_stage_dispatches "$log" log_triage)"
      perf_triage_dispatches="$(eval_count_stage_dispatches "$log" perf_triage)"
      echo "runtime_artifact_attached=$runtime_attachment_kind"
      echo "runtime_authority_path=$(eval_runtime_authority_path "$runtime_attachment_kind" "$log")"
      echo "runtime_prestage_dispatches=$(( ${log_triage_dispatches:-0} + ${perf_triage_dispatches:-0} ))"
      echo "log_triage_dispatches=$log_triage_dispatches"
      echo "perf_triage_dispatches=$perf_triage_dispatches"
      echo "emit_log_triage_calls=$(eval_count_tool_calls "$log" emit_log_triage)"
      echo "emit_perf_trace_calls=$(eval_count_tool_calls "$log" emit_perf_trace)"
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
      echo "ai_style_hits=$(eval_count_ai_style_hits "$log")"
      echo "strict_decode_remap_events=$(eval_count_pattern 'strict_decode_remap.*misplaced field' "$log")"
      echo "strict_decode_carrier_events=$(eval_count_pattern 'strict_decode_remap. string-carrier field' "$log")"
      echo "strict_decode_element_shape_events=$(eval_count_pattern 'strict_decode_remap. array-element shape field' "$log")"
      echo "answer_document_blocks_string_recovery_events=$(eval_count_control_pattern 'WARN \[emit_answer_document\] blocks\[\] arrived as JSON-encoded string; re-parsed via flat-mode tolerance path' "$log")"
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
