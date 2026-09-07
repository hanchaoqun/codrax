#!/usr/bin/env bash
# B1610: exercise the real runner against durable fake-CLI deliveries, never LLMs.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1
source eval/runner_lib.sh

fail() { echo "FAIL: $*" >&2; exit 1; }
assert_eq() {
  [[ "$1" == "$2" ]] || fail "$3: got '$1', want '$2'"
}
mkdir -p "$ROOT/.codrax/tmp"
tmp="$(mktemp -d "$ROOT/.codrax/tmp/post-apply-scope-test.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT
first="include/nlohmann/detail/output/serializer.hpp"
second="single_include/nlohmann/json.hpp"
both="$first"$'\n'"$second"

fake="$tmp/fake-codrax"
cat >"$fake" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
repo="" plan_out="" plan_file=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo) repo="$2"; shift 2 ;;
    --plan-out) plan_out="$2"; shift 2 ;;
    --plan-file) plan_file="$2"; shift 2 ;;
    *) shift ;;
  esac
done
first="include/nlohmann/detail/output/serializer.hpp"
second="single_include/nlohmann/json.hpp"
if [[ -n "$plan_out" ]]; then
  printf '{"id":"plan-scope","status":"ready","changes":[{"path":"%s","kind":"patch"},{"path":"%s","kind":"patch"}]}\n' "$first" "$second" >"$plan_out"
  echo planned
  exit 0
fi
[[ -n "$plan_file" ]] || exit 2
plan_dir="$(dirname "$plan_file")"
for header in "$first" "$second"; do
  format="$FAKE_FIRST_FORMAT"
  [[ "$header" != "$second" ]] || format="$FAKE_SECOND_FORMAT"
  sed "s/%\.\*lg/%.*$format/" "$repo/$header" >"$repo/$header.tmp"
  mv "$repo/$header.tmp" "$repo/$header"
done
printf '\n// first_marker\n' >>"$repo/$first"
case "${FAKE_VARIANT:-}" in
  missing) rm "$repo/$second" ;;
  banned) printf '\n// forbidden_marker\n' >>"$repo/$second" ;;
  space) cp "$repo/$first" "$repo/space name.hpp" ;;
  escape)
    printf 'outside source includes %%.*Lg and enough text\n' >"$plan_dir/outside.hpp"
    ln -s "$plan_dir/outside.hpp" "$repo/escape.hpp"
    ;;
esac
git -C "$repo" add --all
git -C "$repo" -c user.email=eval@codrax -c user.name=eval commit -q -m "fake scoped delivery"
sha="$(git -C "$repo" rev-parse HEAD)"
git -C "$repo" update-ref refs/codrax/applied/plan-scope "$sha"
git -C "$repo" reset --hard -q HEAD~1
printf '{"id":"plan-scope","status":"applied","applied_commit_sha":"%s","worktree_path":"%s/discarded","changes":[{"path":"%s","kind":"patch"},{"path":"%s","kind":"patch"}]}\n' "$sha" "$plan_dir" "$first" "$second" >"$plan_file"
printf '{\n  "plan_id": "plan-scope",\n  "channel": "post_apply_verify",\n  "passed": true,\n  "executed_commands": []\n}\n' >"$plan_dir/plan-scope.report.json"
printf '{"kind":"final_report","run_status":"complete","completion":{"verdict":"verified","reason_code":"all_batches_verified"},"plan":{"id":"plan-scope"}}\n' >"$plan_dir/plan-scope.final.json"
echo applied
FAKE
chmod +x "$fake"

# Use the real r1034 symptom case, preserving its fixture and expected regex.
# Only scope/independent test assertions and deterministic fake delivery vary.
run_scope() {
  local id="$1" single="$2" plural="$3" format1="$4" format2="$5"
  local variant="${6:-}" extra="${7:-}"
  case_file="$tmp/$id.case"
  {
    printf 'source %q\n' "$ROOT/eval/cases/github_issue_nlohmann_long_double_symptom.case"
    printf 'ID=%q\nPOST_APPLY_FILE=%q\nPOST_APPLY_FILES=%q\n' "$id" "$single" "$plural"
    printf '%s\n' "$extra"
  } >"$case_file"
  FAKE_FIRST_FORMAT="$format1" FAKE_SECOND_FORMAT="$format2" FAKE_VARIANT="$variant" \
    CODRAX_BIN="$fake" CODRAX_PROVIDER_ARGS_RAW="" EVAL_RESULTS_ROOT="$tmp/results" \
    bash eval/run.sh "$case_file" 1 >"$tmp/$id.out" 2>"$tmp/$id.err"
  run_rc=$?
  result_dir="$(find "$tmp/results" -maxdepth 1 -type d -name "$id-*" 2>/dev/null | LC_ALL=C sort | tail -1)"
  verdict=""
  [[ -z "$result_dir" ]] || verdict="$(cat "$result_dir/run-1.verdict")"
}
assert_fail_for() {
  case "$verdict" in
    FAIL*"post_apply_file:$1:"*"$2"*) ;;
    *) fail "$3: $verdict" ;;
  esac
}
summary_scope_verdict() {
  LC_ALL=C awk -v header="## Post-apply file — \`$1\`" '
    /^## Post-apply file/ { selected = index($0, header) == 1 }
    selected && /^\*\*Scoped oracle verdict:\*\* / {
      sub(/^\*\*Scoped oracle verdict:\*\* /, "")
      print
      exit
    }
  ' "$result_dir/summary.md"
}

run_scope readme_mask "" "$both" Lf Lf
assert_fail_for "$first" no_regex_match "README must not satisfy a header oracle"
assert_fail_for "$second" no_regex_match "each bad publication header must fail independently"
assert_eq "$(eval_json_top_bool_field "$result_dir/run-1.write-apply.json" verify_authoritative)" true "scope failure must not falsify the independent verified delivery receipt"
grep -Fq '%.*Lg' "$result_dir/run-1.applied-tree/README.md" || fail "mask reproduction requires the unchanged correct README example"

run_scope second_bad "" "$both" Lg Lf
assert_fail_for "$second" no_regex_match "one correct target must not satisfy the other"
assert_eq "$(summary_scope_verdict "$first")" "$(cat "$result_dir/run-1.post-apply-1.verdict")" "first summary scope must show its persisted PASS"
assert_eq "$(summary_scope_verdict "$second")" "$(cat "$result_dir/run-1.post-apply-2.verdict")" "second summary scope must show its persisted FAIL, not the first target result"
run_scope first_bad "" "$both" Lf Lg
assert_fail_for "$first" no_regex_match "target ordering must not select the winning target"
run_scope both_good "" "$both" Lg Lg
assert_eq "$verdict" PASS "both targets independently satisfy the unchanged oracle"
assert_eq "$(cat "$result_dir/run-1.post-apply-1.verdict")" PASS "first scoped receipt"
assert_eq "$(cat "$result_dir/run-1.post-apply-2.verdict")" PASS "second scoped receipt"
grep -Fq "$first" "$result_dir/run-1.post-apply-scopes.tsv" || fail "receipt must name first target"
grep -Fq "$second" "$result_dir/run-1.post-apply-scopes.tsv" || fail "receipt must name second target"
grep -Fq "$first" "$result_dir/summary.md" || fail "summary must show first target"
grep -Fq "$second" "$result_dir/summary.md" || fail "summary must show second target"

run_scope missing_target "" "$both" Lg Lg missing
assert_fail_for "$second" missing "missing target cannot pass via README or sibling"
run_scope document_task "" README.md Lf Lf
assert_eq "$verdict" PASS "an explicitly scoped documentation task remains valid"
run_scope legacy_single_good "$first" "" Lg Lf
assert_eq "$verdict" PASS "legacy single file does not acquire a second-file obligation"
run_scope legacy_single_bad "$first" "" Lf Lg
case "$verdict" in FAIL*no_regex_match*) ;; *) fail "legacy single failure changed: $verdict" ;; esac
run_scope legacy_unscoped "" "" Lf Lf
assert_eq "$verdict" PASS "unscoped legacy behavior must not be silently changed in this batch"

# Every existing text matcher goes through the shared checker, not a new regex-only lane.
run_scope per_file_contains "" "$both" Lg Lg "" 'EXPECT_CONTAINS="first_marker"'
assert_fail_for "$second" missing:first_marker "literal positive must hold in every target"
run_scope per_file_banned "" "$both" Lg Lg banned 'EXPECT_NOT_CONTAINS="forbidden_marker"'
assert_fail_for "$second" banned:forbidden_marker "literal negative must hold in every target"
run_scope per_file_sections "" "$both" Lg Lg "" 'EXPECT_SECTIONS="first_marker"'
assert_fail_for "$second" missing_section:first_marker "section oracle must hold in every target"
run_scope per_file_folded "" "$both" Lg Lg "" 'EXPECT_MATCHES_TEXT_REGEX="namespace.*first_marker"'
assert_fail_for "$second" no_text_regex_match "folded regex must hold in every target"
run_scope global_plan_requirement "" "$both" Lg Lg "" 'PLAN_EXPECT_REGEX="missing_plan_contract"'
case "$verdict" in FAIL*no_plan_regex:missing_plan_contract*) ;; *) fail "global preconditions lost: $verdict" ;; esac

run_scope spaces "" 'space name.hpp' Lg Lg space
assert_eq "$verdict" PASS "space-containing path must remain one exact path"
run_scope literal_glob "" '*.hpp' Lg Lg space
assert_fail_for '*.hpp' missing "wildcard syntax must not expand into existing targets"
run_scope external_link "" escape.hpp Lg Lg escape
assert_fail_for escape.hpp outside_source "symlink must not leave the delivered source tree"
run_scope conflicting_scopes "$first" "$both" Lg Lg
assert_eq "$run_rc" 2 "two nonempty scope declarations must fail before dispatch"
[[ -z "$result_dir" ]] || fail "conflicting scopes dispatched a case"
run_scope parent_scope "" '../outside.hpp' Lg Lg
assert_eq "$run_rc" 2 "parent traversal must fail before dispatch"
run_scope absolute_scope "" '/tmp/outside.hpp' Lg Lg
assert_eq "$run_rc" 2 "absolute target must fail before dispatch"
run_scope empty_lines "" $'\n\n' Lg Lg
assert_eq "$run_rc" 2 "declared plural scope without any path must not fall back to whole-tree"

cat >"$tmp/oracle.case" <<'CASE'
MODE="apply"
POST_APPLY_FILES="one.hpp
two.hpp"
EXPECT_MATCHES_REGEX="format"
CASE
assert_eq "$(eval_case_oracle_surface "$tmp/oracle.case")" "write_apply,write_patch_oracle" "plural scope must be classified as write patch oracle, not answer regex"
echo "ok post-apply per-file scope contracts"
