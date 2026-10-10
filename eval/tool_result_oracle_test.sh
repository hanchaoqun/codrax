#!/usr/bin/env bash
# A call attempt (including an unavailable tool) must not satisfy execution.
set -euo pipefail
cd "$(dirname "$0")/.."
source eval/runner_lib.sh
task_tmp="$(mktemp -d /tmp/codrax-tool-result-oracle.XXXXXX)"
trap 'rm -rf "$task_tmp"' EXIT
assert_eq() { [[ "$1" == "$2" ]] || { printf 'FAIL: %s: got %s, want %s\n' "$3" "$1" "$2"; exit 1; }; }
cat >"$task_tmp/rejected.log" <<'LOG'
2026-10-10T01:00:00.000 DEBUG [diag explorer] iter=0 phase=toolcall call[0] tool=trace_query params={}
2026-10-10T01:00:00.001 DEBUG [diag explorer] iter=0 phase=toolresult TOOLRESULT trace_query ok=false len=10:
unavailable
2026-10-10T01:00:00.002 DEBUG [diag explorer] iter=1 ASSISTANT content: quoted phase=toolresult TOOLRESULT trace_query ok=true len=1:
2026-10-10T01:00:00.003 DEBUG [diag explorer] iter=1 phase=toolresult TOOLRESULT other_tool ok=true len=10:
quoted phase=toolresult TOOLRESULT trace_query ok=true len=1:
LOG
assert_eq "$(eval_count_successful_tool_results "$task_tmp/rejected.log" trace_query)" 0 'attempt/rejection/quoted success'
assert_eq "$(eval_successful_tool_result_reasons "$task_tmp/rejected.log" trace_query)" 'no_successful_tool_result:trace_query' 'failed execution reason'
cp "$task_tmp/rejected.log" "$task_tmp/accepted.log"
printf '%s\n' '2026-10-10T01:00:00.004 DEBUG [diag explorer] iter=2 phase=toolresult TOOLRESULT trace_query ok=true len=10:' >>"$task_tmp/accepted.log"
assert_eq "$(eval_count_successful_tool_results "$task_tmp/accepted.log" trace_query)" 1 'actual result'
assert_eq "$(eval_successful_tool_result_reasons "$task_tmp/accepted.log" 'trace_query other_tool')" '' 'all declared tools executed'
assert_eq "$(eval_successful_tool_result_reasons '' trace_query)" 'no_successful_tool_result:trace_query' 'missing log'
assert_eq "$(eval_successful_tool_result_reasons "$task_tmp/accepted.log" 'trace.*')" 'invalid_successful_tool_result_name:trace.*' 'names are not regexes'

cat >"$task_tmp/fake-codrax" <<'SH'
#!/usr/bin/env bash
set -eu
while [[ $# -gt 0 ]]; do
  if [[ "$1" == --log-dir ]]; then logdir="$2"; shift 2; else shift; fi
done
mkdir -p "$logdir"
cp "$ORACLE_LOG" "$logdir/codrax-oracle.log"
printf 'sample answer\n'
SH
chmod +x "$task_tmp/fake-codrax"
for outcome in rejected accepted; do
  printf 'ID=tool_result_%s\nNAME="tool result oracle"\nQUESTION="natural user question"\nMIN_OUTPUT_CHARS=1\nEXPECT_SUCCESSFUL_TOOL_RESULTS="trace_query"\n' "$outcome" >"$task_tmp/$outcome.case"
  ORACLE_LOG="$task_tmp/$outcome.log" CODRAX_BIN="$task_tmp/fake-codrax" EVAL_RESULTS_ROOT="$task_tmp/results" CODRAX_PROVIDER_ARGS_RAW="" bash eval/run.sh "$task_tmp/$outcome.case" 1 >"$task_tmp/$outcome.out" 2>&1 || { cat "$task_tmp/$outcome.out"; exit 1; }
  result_dir="$(eval_latest_result_dir "$task_tmp/results" "tool_result_$outcome" 00000000-000000)"
  expected=PASS
  [[ "$outcome" != rejected ]] || expected='FAIL no_successful_tool_result:trace_query'
  assert_eq "$(<"$result_dir/run-1.verdict")" "$expected" 'public runner wiring'
  assert_eq "$(eval_case_oracle_surface "$task_tmp/$outcome.case")" 'successful_tool_result' 'surface declaration'
done
printf 'PASS: tool result oracle\n'
