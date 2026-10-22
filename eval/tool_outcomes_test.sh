#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source eval/runner_lib.sh
test_dir="$(mktemp -d "${TMPDIR:-/tmp}/codrax-tool-outcomes.XXXXXX")"
trap 'rm -r "$test_dir"' EXIT
cat >"$test_dir/results.log" <<'LOG'
2026-10-22T00:00:00.000 DEBUG [diag explorer] iter=0 phase=toolresult TOOLRESULT emit_investigation_complete ok=false len=90:
emit_investigation_complete rejected: same failure, not a second rejection
---
2026-10-22T00:00:00.001 DEBUG [diag explorer] iter=1 phase=toolresult TOOLRESULT emit_investigation_complete ok=true len=90:
emit_investigation_complete rejected: legacy semantic rejection
---
2026-10-22T00:00:00.002 DEBUG [diag explorer] iter=2 phase=toolresult TOOLRESULT emit_investigation_complete ok=false len=30:
transport failed
---
2026-10-22T00:00:00.003 DEBUG [diag explorer] iter=3 phase=toolresult TOOLRESULT read_file ok=true len=100:
emit_investigation_complete rejected: unrelated file contents
---
2026-10-22T00:00:00.004 DEBUG [diag explorer] iter=4 ASSISTANT content:
emit_investigation_complete rejected: unprefixed quoted payload
2026-10-22T00:00:00.005 DEBUG [diag explorer] iter=5 phase=toolresult TOOLRESULT emit_investigation_complete ok=true len=100:
Investigation marked complete
emit_investigation_complete rejected: quoted later in a successful result
---
2026-10-22T00:00:00.006 INFO [explorer] emit_investigation_complete DOWNGRADED at iter=6 — loop continues
2026-10-22T00:00:00.007 INFO [explorer] emit_investigation_complete observed at iter=7
2026-10-22T00:00:00.008 DEBUG [diag explorer] iter=8 ASSISTANT content: emit_investigation_complete observed at iter=7
2026-10-22T00:00:00.009 DEBUG [diag explorer] iter=9 phase=toolresult TOOLRESULT emit_hypothesis_verdict ok=false len=90:
emit_hypothesis_verdict rejected: one rejection
LOG
assert_count() {
  local got="$1" want="$2" label="$3"
  [[ "$got" == "$want" ]] || { echo "FAIL $label: got $got, want $want" >&2; exit 1; }
}
assert_count "$(eval_count_tool_rejects "$test_dir/results.log" emit_investigation_complete)" 3 'one outcome per result'
assert_count "$(eval_count_tool_rejects "$test_dir/results.log" emit_hypothesis_verdict)" 1 'shared emit counter'
assert_count "$(eval_count_tool_rejects "$test_dir/results.log" '.*')" 0 'invalid tool name'
assert_count "$(eval_count_investigation_outcomes "$test_dir/results.log" accepted)" 1 'accepted observation'
assert_count "$(eval_count_investigation_outcomes "$test_dir/results.log" downgraded)" 1 'soft downgrade observation'
assert_count "$(eval_count_investigation_outcomes "$test_dir/results.log" unknown)" 0 'unknown outcome'
assert_count "$(eval_count_tool_rejects "$test_dir/missing.log" emit_investigation_complete)" 0 'missing log'
assert_count "$(eval_successful_tool_result_reasons "$test_dir/results.log" '')" '' 'absent successful-result requirement'
assert_count "$(eval_successful_tool_result_reasons "$test_dir/results.log" '   ')" '' 'blank successful-result requirement'
echo 'PASS tool outcome counters'
