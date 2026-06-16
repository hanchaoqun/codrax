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
python3 - "$SRC/helper.py" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
path.write_text('def helper():\n    return "future-fix-hidden-from-fair-eval"\n', encoding='utf-8')
PY
git -C "$SRC" add helper.py
git -C "$SRC" commit -q -m "future fix that fair eval must not expose"

FAKE_CODRAX="$WORK/fake-codrax"
python3 - "$FAKE_CODRAX" <<'PY'
from pathlib import Path
import sys

Path(sys.argv[1]).write_text("""#!/usr/bin/env bash
set -euo pipefail

orig_args=("$@")
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
printf '%s\n' "${orig_args[@]}" > "$repo/.codrax/fake-codrax-argv.txt"
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
if row.get("exported_patch_paths") != ["bug.py"]:
    raise SystemExit(f"unexpected exported patch paths: {row.get('exported_patch_paths')!r}")
if row.get("exported_patch_source_paths") != ["bug.py"]:
    raise SystemExit(f"unexpected exported source paths: {row.get('exported_patch_source_paths')!r}")
if row.get("exported_patch_test_paths") != []:
    raise SystemExit(f"unexpected exported test paths: {row.get('exported_patch_test_paths')!r}")
if row.get("final_plan_source_paths") != ["bug.py"]:
    raise SystemExit(f"unexpected final plan source paths: {row.get('final_plan_source_paths')!r}")
if row.get("final_plan_test_only") is not False:
    raise SystemExit(f"unexpected final_plan_test_only: {row.get('final_plan_test_only')!r}")
if row.get("final_plan_covers_exported_source_patch") is not True:
    raise SystemExit(
        "final plan should structurally cover the exported source patch: "
        f"{row.get('final_plan_covers_exported_source_patch')!r}"
    )
if row.get("prediction_verdict") != "predicted_unchecked":
    raise SystemExit(f"unexpected prediction_verdict for fake run without report: {row.get('prediction_verdict')!r}")
if row.get("prediction_local_confidence") != "unknown":
    raise SystemExit(f"unexpected prediction_local_confidence: {row.get('prediction_local_confidence')!r}")
if row.get("prediction_blocks_local_acceptance") is not False:
    raise SystemExit(
        "fake run has no failed verify report, so it should not block local acceptance: "
        f"{row.get('prediction_blocks_local_acceptance')!r}"
    )
if row.get("prediction_audit_block_reason") not in ("", None):
    raise SystemExit(f"unexpected prediction_audit_block_reason: {row.get('prediction_audit_block_reason')!r}")
if row.get("git_history_isolated") is not False:
    raise SystemExit(f"default adapter run must not isolate git history: {row.get('git_history_isolated')!r}")
if (row.get("git_history_isolation") or {}).get("enabled") is not False:
    raise SystemExit(f"default adapter run isolation telemetry should be disabled: {row.get('git_history_isolation')!r}")
argv_path = row.get("instance_dir", "") + "/repo/.codrax/fake-codrax-argv.txt"
with open(argv_path, encoding="utf-8") as handle:
    argv = handle.read().splitlines()
if "--eval-disable-git-history" in argv:
    raise SystemExit("default adapter run unexpectedly passed --eval-disable-git-history to Codrax")
PY

python3 - "$ROOT/eval/swebench/run_codrax_swebench.py" <<'PY'
import importlib.util
import sys
import tempfile
from pathlib import Path

path = sys.argv[1]
spec = importlib.util.spec_from_file_location("run_codrax_swebench", path)
mod = importlib.util.module_from_spec(spec)
assert spec.loader is not None
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)

with tempfile.TemporaryDirectory() as tmp:
    repo = Path(tmp)
    (repo / "setup.cfg").write_text(
        "[options]\n"
        "install_requires =\n"
        "    pyerfa>=2.0\n"
        "    numpy>=1.21  # runtime floor\n"
        "tests_require = pytest-astropy\n"
        "setup_requires =\n"
        "    setuptools_scm\n"
        "[options.extras_require]\n"
        "test =\n"
        "    pytest-xdist\n"
        "    pytest-astropy-header!=0.2.0\n"
        "docs =\n"
        "    sphinx\n",
        encoding="utf-8",
    )
    (repo / "requirements.txt").write_text("hypothesis\n", encoding="utf-8")
    setup_requires = mod.setup_declared_requires(repo)
    if setup_requires != ["pyerfa>=2.0", "numpy>=1.21", "setuptools_scm"]:
        raise SystemExit(f"unexpected setup.cfg declared requires: {setup_requires!r}")
    declared = mod.declared_python_requirements(repo, [{"path": "requirements.txt", "kind": "requirements"}])
    if declared != ["pyerfa>=2.0", "numpy>=1.21", "setuptools_scm", "hypothesis"]:
        raise SystemExit(f"unexpected combined declared requires: {declared!r}")
    test_requires = mod.setup_test_requires(repo)
    if test_requires != ["pytest-astropy", "pytest-xdist", "pytest-astropy-header!=0.2.0"]:
        raise SystemExit(f"unexpected setup.cfg test requires: {test_requires!r}")

reason = mod.prediction_audit_block_reason(
    exported_source_paths=["src/pkg.py"],
    final_plan_source_paths=[],
    final_plan_test_only=True,
    final_plan_covers_exported_source_patch=False,
)
if reason != "final_plan_test_only_exported_source_patch":
    raise SystemExit(f"unexpected audit block reason: {reason!r}")
verdict = mod.prediction_verdict("diff --git a/src/pkg.py b/src/pkg.py\n", "passed", "applied", reason)
if verdict != ("predicted_audit_blocked", "failed", True):
    raise SystemExit(f"unexpected audit-blocked verdict: {verdict!r}")

weak_plan = {
    "behavior_contracts": [
        {"id": "c1", "required": True, "source": "write_analyzer"},
    ],
    "verification_probes": [
        {"id": "probe", "language": "python", "code": "print('ok')"},
    ],
}
probe_report = {
    "verification_status": "passed",
    "passed": True,
    "executed_commands": [
        {"runner": "verification_probe", "outcome": "executed", "exit_code": 0},
    ],
    "test_results": [
        {"suite": "verification_probe/python", "assertion_id": "probe", "passed": True},
    ],
}
downgrade = mod.prediction_confidence_downgrade_reason(
    plan=weak_plan,
    report=probe_report,
    verify_status="passed",
)
if downgrade != "verification_probe_missing_required_contract_ref":
    raise SystemExit(f"unexpected weak-probe downgrade reason: {downgrade!r}")
verdict = mod.prediction_verdict("diff --git a/src/pkg.py b/src/pkg.py\n", "passed", "applied", "", downgrade)
if verdict != ("predicted_passed_low_confidence", "unknown", False):
    raise SystemExit(f"unexpected downgraded verdict: {verdict!r}")

fallback_plan = {
    "behavior_contracts": [
        {"id": "outcome-1", "required": True, "source": "expected_outcome_fallback"},
    ],
    "verification_probes": [
        {"id": "probe", "language": "python", "code": "assert ', in mm' in repr(obj)", "changed_symbol_refs": ["pkg.repr"]},
    ],
}
fallback_downgrade = mod.prediction_confidence_downgrade_reason(
    plan=fallback_plan,
    report=probe_report,
    verify_status="passed",
)
if fallback_downgrade != "verification_probe_missing_required_contract_ref":
    raise SystemExit(f"unexpected fallback-contract downgrade reason: {fallback_downgrade!r}")

report_confidence_downgrade = mod.prediction_confidence_downgrade_reason(
    plan=fallback_plan,
    report={
        "executed_commands": [{"runner": "verification_probe", "outcome": "executed"}],
        "test_results": [{"suite": "verification_probe/python"}],
        "verification_confidence": [
            {
                "category": "probe_contract_refs",
                "status": "missing",
                "reason_code": "verification_probe_missing_required_contract_ref",
            }
        ],
    },
    verify_status="passed",
)
if report_confidence_downgrade != "verification_probe_missing_required_contract_ref":
    raise SystemExit(f"unexpected report-confidence downgrade reason: {report_confidence_downgrade!r}")
confidence_reasons = mod.verification_confidence_reason_codes({
    "verification_confidence": [
        {"reason_code": "source_compile_ok"},
        {"reason_code": "source_compile_ok"},
        {"reason_code": "verification_probe_missing_required_contract_ref"},
    ]
})
if confidence_reasons != ["source_compile_ok", "verification_probe_missing_required_contract_ref"]:
    raise SystemExit(f"unexpected confidence reason export: {confidence_reasons!r}")

strong_plan = {
    "behavior_contracts": [
        {"id": "c1", "required": True, "source": "write_analyzer"},
    ],
    "verification_probes": [
        {
            "id": "probe",
            "language": "python",
            "code": "import pkg\nassert pkg.ok()",
            "contract_refs": ["c1"],
            "changed_symbol_refs": ["pkg.ok"],
        },
    ],
}
if mod.prediction_confidence_downgrade_reason(plan=strong_plan, report=probe_report, verify_status="passed") != "":
    raise SystemExit("strong contract-coupled probe should not downgrade local confidence")
PY

FAIR_PREDICTIONS="$WORK/predictions-fair.jsonl"
FAIR_RESULTS="$WORK/results-fair.jsonl"
python3 "$ROOT/eval/swebench/run_codrax_swebench.py" \
  --instances-jsonl "$INSTANCES" \
  --repo-cache "$WORK/repo-cache" \
  --workdir "$WORK/run-fair" \
  --predictions-path "$FAIR_PREDICTIONS" \
  --results-path "$FAIR_RESULTS" \
  --codrax-bin "$FAKE_CODRAX" \
  --limit 1 \
  --codrax-timeout 60 \
  --git-timeout 60 \
  --isolate-git-history

python3 "$ROOT/eval/swebench/validate_predictions.py" "$FAIR_PREDICTIONS" \
  --instances-jsonl "$INSTANCES" \
  --require-nonempty-patch

python3 - "$FAIR_RESULTS" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    rows = [json.loads(line) for line in handle if line.strip()]
row = rows[0]
isolation = row.get("git_history_isolation") or {}
if row.get("git_history_isolated") is not True or isolation.get("enabled") is not True:
    raise SystemExit(f"fair adapter run did not enable git history isolation: {isolation!r}")
if isolation.get("head") != row.get("base_commit_resolved"):
    raise SystemExit(f"git history isolation changed the evaluated HEAD: {isolation!r}")
if int(isolation.get("deleted_ref_count") or 0) < 1:
    raise SystemExit(f"fair adapter run did not prune any refs: {isolation!r}")
argv_path = row.get("instance_dir", "") + "/repo/.codrax/fake-codrax-argv.txt"
with open(argv_path, encoding="utf-8") as handle:
    argv = handle.read().splitlines()
if "--eval-disable-git-history" not in argv:
    raise SystemExit("fair adapter run did not pass --eval-disable-git-history to Codrax")
PY

DRY_RUN=1 PREDICTIONS_PATH="$PREDICTIONS" "$ROOT/eval/swebench/run_official_harness.sh" >/dev/null

echo "local SWE-bench adapter smoke passed"
echo "predictions: $PREDICTIONS"
echo "results: $RESULTS"
