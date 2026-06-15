#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP_ROOT="${TMPDIR:-/tmp}"
WORK="$(mktemp -d "${TMP_ROOT%/}/codrax-swebench-smoke.XXXXXX")"
KEEP="${KEEP:-0}"

cleanup() {
  if [[ "$KEEP" != "1" ]]; then
    rm -rf "$WORK"
  else
    echo "kept smoke workspace: $WORK" >&2
  fi
}
trap cleanup EXIT

SRC="$WORK/source"
mkdir -p "$SRC"
git -C "$SRC" init -q -b main
git -C "$SRC" config user.email "swebench-smoke@codrax.local"
git -C "$SRC" config user.name "Codrax SWE-bench Smoke"

python3 - "$SRC/bug.py" "$SRC/helper.py" <<'PY'
from pathlib import Path
import sys

Path(sys.argv[1]).write_text('def answer():\n    return "bad"\n', encoding='utf-8')
Path(sys.argv[2]).write_text('def helper():\n    return "context-only"\n', encoding='utf-8')
PY
git -C "$SRC" add bug.py helper.py
git -C "$SRC" commit -q -m seed
BASE_COMMIT="$(git -C "$SRC" rev-parse HEAD)"

FAKE_CODRAX="$WORK/fake-codrax"
python3 - "$FAKE_CODRAX" <<'PY'
from pathlib import Path
import sys

Path(sys.argv[1]).write_text("""#!/usr/bin/env bash
set -euo pipefail

repo=""
log_dir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      repo="$2"
      shift 2
      ;;
    --repo=*)
      repo="${1#*=}"
      shift
      ;;
    --log-dir)
      log_dir="$2"
      shift 2
      ;;
    --log-dir=*)
      log_dir="${1#*=}"
      shift
      ;;
    *)
      shift
      ;;
  esac
done

if [[ -z "$repo" ]]; then
  echo "fake-codrax: --repo is required" >&2
  exit 2
fi

mkdir -p "$repo/.codrax/plans"
mkdir -p "$repo/.codrax/plans/workflows"
if [[ -n "$log_dir" ]]; then
  mkdir -p "$log_dir"
fi

python3 - "$repo/bug.py" <<'PY_INNER'
from pathlib import Path
import sys

path = Path(sys.argv[1])
path.write_text(path.read_text(encoding="utf-8").replace('"bad"', '"good"'), encoding="utf-8")
PY_INNER

git -C "$repo" add bug.py
git -C "$repo" -c user.email=swebench-smoke@codrax.local -c user.name="Codrax SWE-bench Smoke" commit -q -m "fake codrax fix"
sha="$(git -C "$repo" rev-parse HEAD)"
git -C "$repo" update-ref refs/codrax/applied/plan-fake "$sha"

python3 - "$repo/.codrax/plans/plan-fake.json" "$sha" <<'PY_INNER'
from pathlib import Path
import json
import sys

plan_path = Path(sys.argv[1])
sha = sys.argv[2]
plan = {
    "id": "plan-fake",
    "summary": "fake SWE-bench adapter smoke fix",
    "status": "applied",
    "target_paths": ["bug.py"],
    "changes": [{"path": "bug.py", "kind": "patch", "rationale": "local smoke"}],
    "applied_commit_sha": sha,
}
plan_path.write_text(json.dumps(plan, indent=2, sort_keys=True), encoding="utf-8")
PY_INNER

python3 - "$repo/.codrax/plans/workflows/wf-fake.json" <<'PY_INNER'
from pathlib import Path
import json
import sys

workflow = {
    "run_id": "wf-fake",
    "status": "complete",
    "active_batch_id": "batch-1",
    "context_packs": [{
        "pack_id": "pack-fake",
        "batch_id": "batch-1",
        "items": [
            {
                "priority": "p1",
                "kind": "target_file",
                "text": "bug.py",
                "source_stage": "explore",
                "consumers": ["planner"],
            },
            {
                "priority": "p1",
                "kind": "target_file",
                "text": "helper.py",
                "source_stage": "explore",
                "consumers": ["planner"],
            },
            {
                "priority": "p1",
                "kind": "target_file",
                "text": "tests/test_bug.py",
                "source_stage": "explore",
                "consumers": ["planner"],
            },
        ],
    }],
}
Path(sys.argv[1]).write_text(json.dumps(workflow, indent=2, sort_keys=True), encoding="utf-8")
PY_INNER
""", encoding="utf-8")
PY
chmod +x "$FAKE_CODRAX"

INSTANCES="$WORK/instances.jsonl"
python3 - "$INSTANCES" "$BASE_COMMIT" "$SRC" <<'PY'
import json
import sys

row = {
    "instance_id": "local__bug-1",
    "repo": "local/bug",
    "repo_url": sys.argv[3],
    "base_commit": sys.argv[2],
    "problem_statement": "answer() returns the wrong string. Fix the implementation so it returns good.",
}
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    handle.write(json.dumps(row, sort_keys=True) + "\n")
PY

PREDICTIONS="$WORK/predictions.jsonl"
RESULTS="$WORK/results.jsonl"
python3 "$ROOT/eval/swebench/run_codrax_swebench.py" \
  --instances-jsonl "$INSTANCES" \
  --repo-cache "$WORK/repo-cache" \
  --workdir "$WORK/run" \
  --predictions-path "$PREDICTIONS" \
  --results-path "$RESULTS" \
  --codrax-bin "$FAKE_CODRAX" \
  --limit 1 \
  --codrax-timeout 60 \
  --git-timeout 60

python3 "$ROOT/eval/swebench/validate_predictions.py" "$PREDICTIONS" \
  --instances-jsonl "$INSTANCES" \
  --require-nonempty-patch

python3 - "$PREDICTIONS" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    rows = [json.loads(line) for line in handle if line.strip()]
patch = rows[0]["model_patch"]
if 'return "bad"' not in patch or 'return "good"' not in patch:
    raise SystemExit("prediction patch does not contain the expected before/after lines")
PY

python3 - "$RESULTS" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    rows = [json.loads(line) for line in handle if line.strip()]
row = rows[0]
env = row.get("env_prepare") or {}
if row.get("env_prepare_status") != "skipped" or env.get("status") != "skipped":
    raise SystemExit(f"env prepare status not exported as skipped: row={row.get('env_prepare_status')!r} nested={env.get('status')!r}")
if row.get("env_prepare_success") is not False or env.get("success") is not False:
    raise SystemExit("env prepare success should be false for skipped local smoke")
if row.get("env_prepare_env_available") is not False or env.get("env_available") is not False:
    raise SystemExit("env prepare env_available should be false for skipped local smoke")
if row.get("env_prepare_failed_step_names") != [] or env.get("failed_step_names") != []:
    raise SystemExit("env prepare failed_step_names should be an empty list for skipped local smoke")
if env.get("hard_gate") is not False:
    raise SystemExit("env prepare telemetry must be observational, not a hard gate")
if row.get("workflow_run_id") != "wf-fake" or row.get("workflow_status") != "complete":
    raise SystemExit("workflow telemetry was not exported")
if row.get("plan_context_paths") != ["bug.py", "helper.py"]:
    raise SystemExit(f"unexpected context paths: {row.get('plan_context_paths')!r}")
if row.get("plan_context_covered_paths") != ["bug.py"]:
    raise SystemExit(f"unexpected covered context paths: {row.get('plan_context_covered_paths')!r}")
if row.get("plan_context_uncovered_paths") != ["helper.py"]:
    raise SystemExit(f"unexpected uncovered context paths: {row.get('plan_context_uncovered_paths')!r}")
if row.get("plan_context_coverage_ratio") != 0.5:
    raise SystemExit(f"unexpected context coverage ratio: {row.get('plan_context_coverage_ratio')!r}")
PY

DRY_RUN=1 PREDICTIONS_PATH="$PREDICTIONS" "$ROOT/eval/swebench/run_official_harness.sh" >/dev/null

echo "local SWE-bench adapter smoke passed"
echo "predictions: $PREDICTIONS"
echo "results: $RESULTS"
