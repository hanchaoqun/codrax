#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
tmp="$(mktemp -d "${TMPDIR:-/tmp}/codrax-log-files-runner.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
printf 'first source complete tail\n' >"$tmp/first log [1].txt"
printf 'second source complete tail\n' | gzip >"$tmp/second log.gz"
export LOG_FILES_TEST_CALLS="$tmp/calls"
export LOG_FILES_TEST_FIRST="$tmp/first log [1].txt"
export LOG_FILES_TEST_SECOND="$tmp/second log.gz"
cat >"$tmp/fake-codrax" <<'BIN'
#!/usr/bin/env bash
set -euo pipefail
files=()
inline=""
logdir=""
while (( $# )); do
  case "$1" in
    --log) files+=("$2"); shift ;;
    --log-text) inline="$2"; shift ;;
    --log-dir) logdir="$2"; shift ;;
    --htrace|--htrace-text) exit 21 ;;
  esac
  shift
done
if [[ -n "$inline" ]]; then
  [[ "$inline" == "legacy inline" && ${#files[@]} == 0 ]] || exit 22
else
  [[ ${#files[@]} == "$LOG_FILES_TEST_EXPECTED" ]] || exit 23
  [[ "${files[0]}" == "$LOG_FILES_TEST_FIRST" ]] || exit 24
  [[ "$(cat "${files[0]}")" == "first source complete tail" ]] || exit 25
  if [[ ${#files[@]} == 2 ]]; then
    [[ "${files[1]}" == "$LOG_FILES_TEST_SECOND" ]] || exit 26
    [[ "$(gzip -cd "${files[1]}")" == "second source complete tail" ]] || exit 27
  fi
fi
printf 'dispatch\n' >>"$LOG_FILES_TEST_CALLS"
mkdir -p "$logdir"
printf '%s\n' \
  '2026-10-09T00:00:00.000 DEBUG [diag orchestrator] DISPATCH stage=log_triage agent=log_triager' \
  '2026-10-09T00:00:00.001 DEBUG [diag log_triager] phase=toolcall tool=emit_log_triage' \
  '2026-10-09T00:00:00.002 DEBUG [diag explorer] phase=toolcall tool=log_query' \
  >"$logdir/codrax-fixture.log"
printf '%s\n' 'log-sources-exact: this transport fixture checks repeated log arguments, original source boundaries, filenames with spaces and glob characters, and intact gzip bytes. It is only a deterministic runner test; it does not stand in for a model answer or semantic analysis.'
BIN
chmod +x "$tmp/fake-codrax"
export LOG_FILES_TEST_EXPECTED=2
cat >"$tmp/array.case" <<CASE
ID="log_files_transport"
NAME="complete log source transport"
QUESTION="Inspect the attached logs."
LOG_FILES=("$LOG_FILES_TEST_FIRST" "$LOG_FILES_TEST_SECOND")
EXPECT_CONTAINS="log-sources-exact"
CASE
if ! CODRAX_BIN="$tmp/fake-codrax" EVAL_RESULTS_ROOT="$tmp/results" CODRAX_PROVIDER_ARGS_RAW="" \
  bash eval/run.sh "$tmp/array.case" 2 >"$tmp/runner.log" 2>&1; then
  cat "$tmp/runner.log"; exit 1
fi
source eval/runner_lib.sh
verdict_count=0
for verdict in "$tmp"/results/*/run-*.verdict; do
  [[ "$(head -1 "$verdict")" == PASS* ]] || { cat "$tmp/runner.log"; exit 1; }
  verdict_count=$((verdict_count + 1))
  metrics="${verdict%.verdict}.metrics.txt"
  [[ "$(eval_metric_field "$metrics" runtime_artifact_attached)" == log ]]
  [[ "$(eval_metric_field "$metrics" log_triage_dispatches)" == 1 ]]
  [[ "$(eval_metric_field "$metrics" emit_log_triage_calls)" == 1 ]]
  [[ "$(eval_metric_field "$metrics" tool_log_query)" == 1 ]]
  [[ "$(eval_metric_field "$metrics" runtime_authority_path)" == log_triage+log_query ]]
done
[[ "$verdict_count" == 2 && "$(wc -l <"$tmp/calls" | tr -d ' ')" == 2 ]]
[[ "$(eval_case_oracle_surface "$tmp/array.case")" == *log_attachment* ]]
printf '%s\n' '2026-10-09T00:00:00.000 DEBUG [diag explorer] phase=toolcall tool=log_query' >"$tmp/query-only.log"
[[ "$(eval_runtime_attachment_kind_from_log "$tmp/query-only.log")" == log ]]
[[ "$(eval_runtime_authority_path log "$tmp/query-only.log")" == log_query ]]
printf '%s\n' '2026-10-09T00:00:00.000 DEBUG [diag explorer] phase=toolcall tool=log_query_extra' >"$tmp/other.log"
[[ "$(eval_runtime_attachment_kind_from_log "$tmp/other.log")" == none ]]
[[ "$(eval_runtime_authority_path log "$tmp/other.log")" == missing_runtime_authority ]]

# Legacy scalar forms, including an explicitly empty array, remain valid.
export LOG_FILES_TEST_EXPECTED=1
for form in file inline; do
  printf 'ID="legacy_%s"\nNAME="legacy log"\nQUESTION="Inspect logs."\nEXPECT_CONTAINS="log-sources-exact"\nLOG_FILES=()\n' "$form" >"$tmp/legacy.case"
  if [[ "$form" == file ]]; then
    printf 'LOG_FILE="%s"\n' "$LOG_FILES_TEST_FIRST" >>"$tmp/legacy.case"
  else
    printf 'LOG="legacy inline"\n' >>"$tmp/legacy.case"
  fi
  if ! CODRAX_BIN="$tmp/fake-codrax" EVAL_RESULTS_ROOT="$tmp/legacy-results" \
    bash eval/run.sh "$tmp/legacy.case" 1 >"$tmp/legacy.log" 2>&1; then
    cat "$tmp/legacy.log"; exit 1
  fi
done

reject_case() {
  local label="$1" declaration="$2" expected="$3" rc=0
  printf 'ID="invalid_log_files"\nQUESTION="Must not dispatch."\n%s\n' "$declaration" >"$tmp/invalid.case"
  CODRAX_BIN="$tmp/fake-codrax" EVAL_RESULTS_ROOT="$tmp/invalid-results" \
    bash eval/run.sh "$tmp/invalid.case" 1 >"$tmp/invalid.log" 2>&1 || rc=$?
  [[ "$rc" == 2 ]] || { echo "$label returned $rc, expected 2"; cat "$tmp/invalid.log"; exit 1; }
  grep -Fq "$expected" "$tmp/invalid.log" || { cat "$tmp/invalid.log"; exit 1; }
}
array="LOG_FILES=(\"$LOG_FILES_TEST_FIRST\" \"$LOG_FILES_TEST_SECOND\")"
reject_case inline_conflict "$array
LOG='other'" 'LOG_FILES must not be combined'
reject_case file_conflict "$array
LOG_FILE='$LOG_FILES_TEST_FIRST'" 'LOG_FILES must not be combined'
reject_case trace_conflict "$array
HTRACE='# trace'" 'log and htrace attachments together'
reject_case trace_file_conflict "$array
HTRACE_FILE='$LOG_FILES_TEST_FIRST'" 'log and htrace attachments together'
reject_case trace_stdin_conflict "$array
HTRACE_STDIN_FILE='$LOG_FILES_TEST_FIRST'" 'HTRACE_STDIN_FILE must be the only attachment'
reject_case missing_member "LOG_FILES=(\"$LOG_FILES_TEST_FIRST\" \"$tmp/missing.log\")" 'LOG_FILES member not found'
reject_case empty_member 'LOG_FILES=("")' 'LOG_FILES member not found'
reject_case stdin_member 'LOG_FILES=("-")' 'LOG_FILES member not found'
reject_case scalar 'LOG_FILES="not an array"' 'LOG_FILES must be an indexed array'
[[ "$(wc -l <"$tmp/calls" | tr -d ' ')" == 4 ]] || { echo 'invalid case reached the binary'; exit 1; }
echo 'PASS LOG_FILES preserves each source, gzip bytes, metrics and attachment exclusions'
