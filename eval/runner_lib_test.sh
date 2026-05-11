#!/usr/bin/env bash

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1

# shellcheck source=eval/runner_lib.sh
source eval/runner_lib.sh

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local got="$1"
  local want="$2"
  local label="$3"
  if [[ "$got" != "$want" ]]; then
    fail "$label: got '$got', want '$want'"
  fi
}

tmp="$(mktemp -d "${TMPDIR:-/tmp}/codrax-eval-runner-test.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/results/sample-20260511-000000"
mkdir -p "$tmp/results/sample-20260511-010000"
latest="$(eval_latest_result_dir "$tmp/results" sample 20260511-005959)" || fail "latest result dir not found"
assert_eq "$latest" "$tmp/results/sample-20260511-010000" "fresh result selection"

printf 'analyzer_dispatches=2\nfinalizer_dispatches=5\n' >"$tmp/metrics.txt"
assert_eq "$(eval_metric_field "$tmp/metrics.txt" analyzer_dispatches)" "2" "metric field parse"
assert_eq "$(eval_metric_field "$tmp/metrics.txt" missing_key)" "-" "missing metric field"

printf 'one\nreject\nreject\n' >"$tmp/log.txt"
assert_eq "$(eval_count_pattern 'reject' "$tmp/log.txt")" "2" "pattern count"
assert_eq "$(eval_count_pattern 'reject' "$tmp/missing.log")" "0" "missing log pattern count"

if command -v timeout >/dev/null 2>&1 ||
  command -v gtimeout >/dev/null 2>&1 ||
  command -v python3 >/dev/null 2>&1; then
  start="$(date +%s)"
  eval_run_with_timeout 1 bash -c 'sleep 5' >/dev/null 2>&1
  rc=$?
  elapsed=$(($(date +%s) - start))
  assert_eq "$rc" "124" "timeout return code"
  if [[ "$elapsed" -gt 12 ]]; then
    fail "timeout fallback took too long: ${elapsed}s"
  fi
fi

true_bin="$(command -v true || true)"
if [[ -n "$true_bin" && -x "$true_bin" ]]; then
  case_file="$tmp/runner_contract.case"
  cat >"$case_file" <<'CASE'
ID=runner_contract
QUESTION="runner harness smoke"
EXPECT_CONTAINS="this-will-not-appear"
CASE
  out="$(CODRAX_BIN="$true_bin" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$case_file" 1 2>&1)"
  if [[ "$out" != *"using codrax snapshot: $true_bin"* ]]; then
    fail "eval/run.sh did not honor CODRAX_BIN snapshot; output: $out"
  fi
  result_count="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_contract-*' | wc -l | tr -d ' ')"
  if [[ "$result_count" -lt 1 ]]; then
    fail "eval/run.sh did not write under EVAL_RESULTS_ROOT"
  fi
fi

echo "ok eval runner contracts"
