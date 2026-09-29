#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
tmp="$(mktemp -d "${TMPDIR:-/tmp}/codrax-stdin-runner.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
printf 'a\000b\377c' >"$tmp/input.bin"
cat >"$tmp/fake-codrax" <<'BIN'
#!/usr/bin/env bash
set -eu
found=0
while (( $# )); do
  if [[ "$1" == --htrace ]]; then [[ "$2" == - ]] || exit 19; found=1; shift; fi
  shift
done
[[ "$found" == 1 ]] || exit 20
hex="$(od -An -tx1 | tr -d ' \n')"
[[ "$hex" == 610062ff63 ]] || exit 21
echo 'stream-bytes-exact'
printf '%s\n' 'The fixture verifies complete binary input, including NUL and non-UTF8 bytes. It also verifies that each evaluation invocation reopens the input independently. This is a transport test, not a model answer or a semantic correctness evaluation.'
BIN
chmod +x "$tmp/fake-codrax"
cat >"$tmp/stream.case" <<CASE
ID="stdin_binary_transport"
NAME="binary stdin transport"
QUESTION="Inspect the attached trace."
HTRACE_STDIN_FILE="$tmp/input.bin"
EXPECT_CONTAINS="stream-bytes-exact"
CASE
if ! CODRAX_BIN="$tmp/fake-codrax" EVAL_RESULTS_ROOT="$tmp/results" CODRAX_PROVIDER_ARGS_RAW="" \
  bash eval/run.sh "$tmp/stream.case" 2 >"$tmp/runner.log" 2>&1; then
  cat "$tmp/runner.log"; exit 1
fi
for verdict in "$tmp"/results/*/run-*.verdict; do
  [[ "$(head -1 "$verdict")" == PASS* ]] || { cat "$tmp/runner.log"; exit 1; }
done
source eval/runner_lib.sh
[[ "$(eval_case_oracle_surface "$tmp/stream.case")" == *trace_attachment* ]]
# Reject conflicting attachment declarations before dispatch.
printf '\nHTRACE="other"\n' >>"$tmp/stream.case"
if CODRAX_BIN="$tmp/fake-codrax" EVAL_RESULTS_ROOT="$tmp/conflicts" bash eval/run.sh "$tmp/stream.case" 1 >"$tmp/conflict.log" 2>&1; then
  echo 'conflicting stdin/inline was accepted' >&2; exit 1
fi
echo 'PASS binary stdin runner preserves bytes and reopens each run'
