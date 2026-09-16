#!/usr/bin/env bash
# B1612: real napi symptom case + real runner; deterministic durable deliveries,
# not model calls or a claim that generated JavaScript has been executed.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1
source eval/runner_lib.sh
fail() { echo "FAIL: $*" >&2; exit 1; }
assert_eq() { [[ "$1" == "$2" ]] || fail "$3: got '$1', want '$2'"; }
mkdir -p "$ROOT/.codrax/tmp"
tmp="$(mktemp -d "$ROOT/.codrax/tmp/napi-post-apply-scope-test.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT
target="cli/src/api/templates/js-binding.ts"

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
target="cli/src/api/templates/js-binding.ts"
if [[ -n "$plan_out" ]]; then
  printf '{"id":"plan-napi-scope","status":"ready","changes":[{"path":"%s","kind":"patch"}]}\n' "$target" >"$plan_out"
  echo planned
  exit 0
fi
[[ -n "$plan_file" ]] || exit 2
plan_dir="$(dirname "$plan_file")"
# A correct explanatory example must not prove that the implementation changed.
printf "\nNAPI_RS_FORCE_WASI === 'true'\nNAPI_RS_FORCE_WASI === 'error'\n" >>"$repo/README.md"
case "$FAKE_NAPI_VARIANT" in
  decoy) ;;
  true_only)
    sed "s/=== 'error'/=== 'true'/g" "$repo/$target" >"$repo/$target.tmp"
    mv "$repo/$target.tmp" "$repo/$target"
    ;;
  fixed)
    sed "s/if (!nativeBinding || process.env.NAPI_RS_FORCE_WASI) {/if (!nativeBinding || process.env.NAPI_RS_FORCE_WASI === 'true' || process.env.NAPI_RS_FORCE_WASI === 'error') {/" "$repo/$target" >"$repo/$target.tmp"
    mv "$repo/$target.tmp" "$repo/$target"
    ;;
  missing) rm "$repo/$target" ;;
  *) exit 2 ;;
esac
git -C "$repo" add --all
git -C "$repo" -c user.email=eval@codrax -c user.name=eval commit -q -m "fake napi scope delivery"
sha="$(git -C "$repo" rev-parse HEAD)"
git -C "$repo" update-ref refs/codrax/applied/plan-napi-scope "$sha"
git -C "$repo" reset --hard -q HEAD~1
printf '{"id":"plan-napi-scope","status":"applied","applied_commit_sha":"%s","worktree_path":"%s/discarded","changes":[{"path":"%s","kind":"patch"}]}\n' "$sha" "$plan_dir" "$target" >"$plan_file"
printf '{\n  "plan_id": "plan-napi-scope",\n  "channel": "post_apply_verify",\n  "passed": true,\n  "executed_commands": []\n}\n' >"$plan_dir/plan-napi-scope.report.json"
printf '{"kind":"final_report","run_status":"complete","completion":{"verdict":"verified","reason_code":"all_batches_verified"},"plan":{"id":"plan-napi-scope"}}\n' >"$plan_dir/plan-napi-scope.final.json"
# README is an intentional changed decoy in these fixtures. Include it in the
# real checkpoint receipt without broadening the implementation-file oracle.
python3 - "$repo" "$sha" "$plan_file" <<'PY'
import json
from pathlib import Path
import subprocess
import sys

repo, sha, raw_plan = sys.argv[1:]
plan_path = Path(raw_plan)
plan = json.loads(plan_path.read_text())
raw_paths = subprocess.check_output(["git", "-C", repo, "diff-tree", "--no-commit-id", "--name-only", "--no-renames", "-r", "-z", sha])
paths = [path.decode() for path in raw_paths.split(b"\0") if path]
declared = {change["path"] for change in plan["changes"]}
plan["changes"].extend({"path": path, "kind": "patch"} for path in paths if path not in declared)
plan["applied_paths"] = paths
plan["apply_checkpoint"] = {"commit_sha": sha, "recovery_ref": "refs/codrax/applied/" + plan["id"], "committed_paths": paths}
plan_path.write_text(json.dumps(plan))
(plan_path.parent / (plan["id"] + ".json")).write_text(json.dumps(plan))
final_path = plan_path.parent / (plan["id"] + ".final.json")
final = json.loads(final_path.read_text())
final["run_id"] = "wf-napi-scope"
final["delivery"] = {"materialization": {
    "schema_version": 1, "status": "available", "reason_code": "applied_checkpoint_candidates",
    "run_id": final["run_id"], "final_plan_id": plan["id"], "retained_plan_ids": [plan["id"]],
    "owners": [{"plan_id": plan["id"], "commit_sha": sha, "paths": paths}],
}}
final_path.write_text(json.dumps(final))
PY
echo applied
FAKE
chmod +x "$fake"

run_napi() {
  local id="$1" variant="$2" override="${3:-}"
  local case_file="$tmp/$id.case"
  {
    printf 'source %q\nID=%q\n' "$ROOT/eval/cases/github_issue_napi_force_wasi_env_symptom.case" "$id"
    printf '%s\n' "$override"
  } >"$case_file"
  FAKE_NAPI_VARIANT="$variant" CODRAX_BIN="$fake" CODRAX_PROVIDER_ARGS_RAW="" \
    EVAL_RESULTS_ROOT="$tmp/results" bash eval/run.sh "$case_file" 1 \
    >"$tmp/$id.out" 2>"$tmp/$id.err"
  result_dir="$(find "$tmp/results" -maxdepth 1 -type d -name "$id-*" 2>/dev/null | LC_ALL=C sort | tail -1)"
  [[ -n "$result_dir" && -f "$result_dir/run-1.verdict" ]] || fail "runner did not produce $id verdict"
  verdict="$(cat "$result_dir/run-1.verdict")"
}

# No scope override here: this is a regression pin on the actual case wiring.
run_napi napi_impl_unfixed decoy
case "$verdict" in FAIL*no_regex_match*) ;; *) fail "README must not satisfy the napi implementation oracle: $verdict" ;; esac
grep -Fq "if (!nativeBinding || process.env.NAPI_RS_FORCE_WASI) {" "$result_dir/run-1.applied-tree/$target" || fail "red witness must retain the unfixed implementation"
assert_eq "$(eval_json_top_bool_field "$result_dir/run-1.write-apply.json" verify_authoritative)" true "file oracle failure must not rewrite the independent mock delivery receipt"

run_napi napi_error_missing true_only
case "$verdict" in FAIL*no_regex_match:*error*) ;; *) fail "the original error expectation must remain independently required: $verdict" ;; esac
run_napi napi_impl_fixed fixed
assert_eq "$verdict" PASS "both original expectations in the implementation must pass"
grep -Fq "$target" "$result_dir/summary.md" || fail "summary must identify the exact implementation file"
run_napi napi_impl_missing missing
case "$verdict" in FAIL*"post_apply_file_missing:$target"*) ;; *) fail "a missing implementation cannot pass via README: $verdict" ;; esac

run_napi napi_legacy_unscoped decoy 'POST_APPLY_FILE=""; POST_APPLY_FILES=""'
assert_eq "$verdict" PASS "this case migration must not alter legacy unscoped runner behavior"
run_napi napi_explicit_readme decoy 'POST_APPLY_FILE="README.md"; POST_APPLY_FILES=""'
assert_eq "$verdict" PASS "an explicitly requested documentation scope must remain legal"
echo "ok napi symptom implementation-scope contracts"
