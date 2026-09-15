#!/usr/bin/env python3
"""Real eval/run.sh regression with a deterministic fake CLI, never an LLM.

The fake writes git checkpoints and formal plan/report/final artifacts. It does
not claim that native tests or product write gates ran. The original Go fixture
and case oracles are used unchanged; all generated state is temporary.
"""

import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent.parent


def dump(path, value):
    Path(path).write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


def git(repo, *args, env=None):
    return subprocess.check_output(["git", "-C", str(repo), *args], stderr=subprocess.PIPE, env=env).decode().strip()


def fake_cli():
    args = sys.argv[1:]
    arg = lambda key: args[args.index(key) + 1] if key in args else ""
    repo = Path(arg("--repo"))
    plan_out, plan_file = arg("--plan-out"), arg("--plan-file")
    variant = os.environ.get("B1634C_VARIANT", "proof")
    kind = "modify" if variant == "unrelated_patch" else "patch"
    source = {"id": "plan-source", "summary": "source edit", "status": "ready", "changes": [{"path": "main.go", "kind": kind}]}
    if plan_out:
        dump(plan_out, source)
        return
    if not plan_file:
        raise RuntimeError("fake only supports the plan/apply runner entry")
    out = Path(plan_file).parent
    source_file = repo / "main.go"
    source_file.write_text(source_file.read_text().replace("retrun fmt", "return fmt"))
    git(repo, "add", "main.go")
    git(repo, "-c", "user.email=eval@codrax", "-c", "user.name=eval", "commit", "-qm", "source checkpoint")
    sha = git(repo, "rev-parse", "HEAD")
    # The pre-existing tree materializer orders checkpoints by whole-second
    # committer time. Keep this fixture's chronology unambiguous; same-second
    # ref-name ordering is a separate materializer issue, not owner selection.
    next_commit_env = {**os.environ, "GIT_COMMITTER_DATE": str(int(git(repo, "show", "-s", "--format=%ct", sha)) + 1) + " +0000"}
    git(repo, "update-ref", "refs/codrax/applied/plan-source", sha)
    source.update(status="applied", applied_commit_sha=sha, applied_paths=["main.go"], worktree_path=str(repo),
                  apply_checkpoint={"commit_sha": sha, "recovery_ref": "refs/codrax/applied/plan-source", "committed_paths": ["main.go"]})
    owners, paths = ["plan-source"], ["main.go"]
    if variant.startswith("multi"):
        (repo / "helper.go").write_text("package main\nconst helper = 1\n")
        git(repo, "add", "helper.go")
        git(repo, "-c", "user.email=eval@codrax", "-c", "user.name=eval", "commit", "-qm", "second source checkpoint", env=next_commit_env)
        second_sha = git(repo, "rev-parse", "HEAD")
        git(repo, "update-ref", "refs/codrax/applied/plan-second", second_sha)
        second = {"id": "plan-second", "summary": "second source", "status": "applied", "changes": [{"path": "helper.go", "kind": "create"}],
                  "applied_commit_sha": second_sha, "applied_paths": ["helper.go"],
                  "apply_checkpoint": {"commit_sha": second_sha, "recovery_ref": "refs/codrax/applied/plan-second", "committed_paths": ["helper.go"]}}
        dump(out / "plan-second.json", second)
        owners.append("plan-second")
        paths.append("helper.go")
        if variant == "multi_reverse":
            owners.reverse()
        if variant == "multi_missing_owner":
            owners.remove("plan-second")
    if variant.startswith("overlap"):
        source_file.write_text(source_file.read_text() + "\n// second source owner\n")
        git(repo, "add", "main.go")
        second_env = next_commit_env
        if variant == "overlap_same_second":
            second_env = {**os.environ, "GIT_COMMITTER_DATE": git(repo, "show", "-s", "--format=%ct", sha) + " +0000"}
        git(repo, "-c", "user.email=eval@codrax", "-c", "user.name=eval", "commit", "-qm", "overlapping source checkpoint", env=second_env)
        second_sha = git(repo, "rev-parse", "HEAD")
        if variant == "overlap_fork":
            # Same bytes, but not a descendant of the first owner's commit.
            # Neither owner is a unique causal successor for this path.
            second_sha = git(repo, "-c", "user.email=eval@codrax", "-c", "user.name=eval", "commit-tree", git(repo, "rev-parse", "HEAD^{tree}"), "-p", git(repo, "rev-parse", sha + "~1"), "-m", "sibling owner", env=next_commit_env)
        git(repo, "update-ref", "refs/codrax/applied/plan-second", second_sha)
        dump(out / "plan-second.json", {
            "id": "plan-second", "summary": "overlapping source edit", "status": "applied", "changes": [{"path": "main.go", "kind": "modify"}],
            "applied_commit_sha": second_sha, "applied_paths": ["main.go"],
            "apply_checkpoint": {"commit_sha": second_sha, "recovery_ref": "refs/codrax/applied/plan-second", "committed_paths": ["main.go"]},
        })
        owners.append("plan-second")
        if variant == "overlap_reverse":
            owners.reverse()
    if variant == "wrong_checkpoint":
        source["apply_checkpoint"]["commit_sha"] = git(repo, "rev-parse", "HEAD~1")
    if variant == "wrong_ref":
        git(repo, "update-ref", "refs/codrax/applied/plan-source", git(repo, "rev-parse", "HEAD~1"))
    if variant == "wrong_checkpoint_paths":
        source["apply_checkpoint"]["committed_paths"] = ["not-main.go"]
    if variant == "wrong_owner_id":
        source["id"] = "plan-someone-else"
    if variant == "unapplied_owner":
        source["status"] = "ready"
    if variant == "unrelated_patch":
        dump(out / "plan-earliest.json", {"id": "plan-earliest", "status": "applied", "changes": [{"path": "other.go", "kind": "patch"}]})
    if variant != "missing_owner":
        dump(out / "plan-source.json", source)
    if variant in {"tree_mismatch", "overlap_wrong_tree"}:
        source_file.write_text(source_file.read_text() + "\n// unrelated later modification\n")
        git(repo, "add", "main.go")
        later_env = {**os.environ, "GIT_COMMITTER_DATE": str(int(git(repo, "show", "-s", "--format=%ct", "HEAD")) + 1) + " +0000"}
        git(repo, "-c", "user.email=eval@codrax", "-c", "user.name=eval", "commit", "-qm", "unrelated checkpoint", env=later_env)
        git(repo, "update-ref", "refs/codrax/applied/plan-unrelated", git(repo, "rev-parse", "HEAD"))
    final_plan = source if variant == "ordinary_apply" else {
        "id": "plan-proof", "summary": "verification follow-up", "status": "applied", "changes": None,
        "persistence_kind": "proof_probe_only", "verification_probes": [{"id": "probe-check"}],
        "worktree_path": str(repo), "target_paths": paths,
        "cumulative_verification_scope": {"source_plan_ids": owners, "target_paths": paths},
    }
    dump(plan_file, final_plan)
    final_id = final_plan["id"]
    report = {"plan_id": final_id, "channel": "post_apply_verify", "passed": variant != "report_failed", "executed_commands": []}
    if variant == "wrong_report":
        report["plan_id"] = "plan-wrong"
    dump(out / (final_id + ".report.json"), report)
    final = {"kind": "final_report", "run_status": "complete", "completion": {"verdict": "verified", "reason_code": "all_batches_verified"},
             "plan": {"id": final_id}, "delivery": {"status": "coherent", "relation": "source_plan_owns_final_delivery",
             "final_plan_id": final_id, "report_plan_id": final_id, "primary_source_plan_id": owners[0],
             "source_owner_plan_ids": owners, "source_paths": paths}}
    if variant == "missing_delivery":
        final.pop("delivery")
    if variant == "wrong_primary":
        final["delivery"]["primary_source_plan_id"] = "plan-not-owner"
    if variant == "wrong_source_authority":
        final["source_authority"] = {"plan_id": "plan-not-owner", "source_paths": paths}
    if variant == "unsafe_owner_id":
        final["delivery"]["source_owner_plan_ids"] = ["../plan-source"]
    if variant == "wrong_delivery_plan":
        final["delivery"]["final_plan_id"] = "plan-wrong"
    if variant == "validation_relation":
        final["delivery"]["relation"] = "source_plan_with_later_validation_followup"
    if variant == "terminal_unverified":
        final["completion"] = {"verdict": "unverified", "reason_code": "verification_proof_incomplete"}
    dump(out / (final_id + ".final.json"), final)
    print("mock apply completed; no native verification claim")


class WritePlanOracleB1634cTest(unittest.TestCase):
    def setUp(self):
        temp_root = ROOT / ".codrax/tmp"
        temp_root.mkdir(parents=True, exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(prefix="b1634c-runner-", dir=temp_root)
        self.addCleanup(self.temp.cleanup)
        self.work = Path(self.temp.name)
        self.fake = self.work / "fake-codrax"
        self.fake.write_text("#!/bin/sh\nexec " + shlex.quote(sys.executable) + " " + shlex.quote(str(Path(__file__).resolve())) + ' --fake-cli "$@"\n')
        self.fake.chmod(0o700)

    def run_case(self, variant, mode="apply"):
        case = self.work / (variant + ".case")
        text = "source " + shlex.quote(str(ROOT / "eval/cases/patch_go_typo.case")) + "\n"
        text += "ID=" + shlex.quote("b1634c_" + variant) + "\nMODE=" + shlex.quote(mode) + "\n"
        if variant in {"multi", "multi_reverse"}:
            text += "PLAN_EXPECT_REGEX=" + shlex.quote('"kind":[[:space:]]*"patch"\n"kind":[[:space:]]*"create"') + "\n"
        if mode == "plan":
            text += 'EXPECT_CONTAINS=""\nEXPECT_NOT_CONTAINS=""\nEXPECT_MATCHES_REGEX=""\n'
        case.write_text(text)
        env = {**os.environ, "CODRAX_BIN": str(self.fake), "CODRAX_PROVIDER_ARGS_RAW": "", "EVAL_RESULTS_ROOT": str(self.work / "results"), "B1634C_VARIANT": variant}
        result = subprocess.run(["bash", "eval/run.sh", str(case), "1"], cwd=ROOT, env=env, text=True, capture_output=True, timeout=60)
        dirs = list((self.work / "results").glob("b1634c_" + variant + "-*"))
        self.assertEqual(len(dirs), 1, result.stdout + result.stderr)
        verdict = (dirs[0] / "run-1.verdict").read_text().strip()
        return verdict, dirs[0]

    def test_proof_only_uses_final_source_owner(self):
        verdict, result = self.run_case("proof")
        self.assertEqual(verdict, "PASS")
        self.assertIsNone(json.loads((result / "run-1.plan.json").read_text())["changes"])
        self.assertEqual(json.loads((result / "run-1.write-apply.json").read_text())["plan_id"], "plan-proof")
        receipt = json.loads((result / "run-1.plan-oracle.json").read_text())
        self.assertEqual(receipt["selection"], "final_delivery_source_plans")
        self.assertEqual([row["plan_id"] for row in receipt["source_owners"]], ["plan-source"])
        self.assertEqual(receipt["final_plan_id"], "plan-proof")
        self.assertEqual(json.loads((result / "run-1.source-plans.json").read_text())[0]["changes"][0]["kind"], "patch")

    def test_multi_source_and_ordinary_modes(self):
        for variant, mode in [("multi", "apply"), ("multi_reverse", "apply"), ("overlap_linear", "apply"), ("overlap_reverse", "apply"), ("validation_relation", "apply"), ("ordinary_apply", "apply"), ("ordinary_plan", "plan")]:
            with self.subTest(variant=variant):
                self.assertEqual(self.run_case(variant, mode)[0], "PASS")

    def test_invalid_ownership_and_terminal_failure_never_pass(self):
        invalid = {
            "missing_owner": "artifact_missing:plan-source.json",
            "wrong_owner_id": "source_owner_id_mismatch:plan-source",
            "unapplied_owner": "source_owner_not_applied:plan-source",
            "missing_delivery": "delivery_missing",
            "wrong_primary": "primary_source_owner_mismatch",
            "wrong_source_authority": "source_authority_owner_mismatch",
            "unsafe_owner_id": "invalid_plan_id",
            "wrong_delivery_plan": "delivery_plan_mismatch",
            "wrong_checkpoint": "source_checkpoint_mismatch:plan-source",
            "wrong_ref": "source_ref_mismatch:plan-source",
            "wrong_checkpoint_paths": "source_checkpoint_paths_mismatch:plan-source",
            "multi_missing_owner": "delivery_source_paths_not_owned",
            "tree_mismatch": "applied_tree_mismatch:main.go",
            "overlap_fork": "source_path_owner_ambiguous:main.go",
            "overlap_wrong_tree": "applied_tree_mismatch:main.go",
            "wrong_report": "report_plan_mismatch",
        }
        for variant in [*invalid, "unrelated_patch", "report_failed", "terminal_unverified"]:
            with self.subTest(variant=variant):
                verdict, _ = self.run_case(variant)
                self.assertTrue(verdict.startswith("FAIL"), (variant, verdict))
                if variant in invalid:
                    self.assertIn("plan_oracle_source_invalid:" + invalid[variant], verdict)
                if variant == "unrelated_patch":
                    self.assertIn("no_plan_regex:", verdict)
                if variant == "report_failed":
                    self.assertIn("write_report_failed", verdict)
                if variant == "terminal_unverified":
                    self.assertIn("write_final_verdict:unverified", verdict)

    def test_same_second_stale_materialization_is_not_greenlit(self):
        verdict, result = self.run_case("overlap_same_second")
        self.assertIn("plan_oracle_source_invalid:applied_tree_mismatch:main.go", verdict)
        second = json.loads((result / "plan-second.json").read_text())
        source = git(result / "run-1.repo", "show", second["applied_commit_sha"] + ":main.go")
        self.assertIn("second source owner", source)
        self.assertNotIn("second source owner", (result / "run-1.applied-tree/main.go").read_text())

    @unittest.expectedFailure
    def test_b1693_same_second_materializer_should_keep_descendant_bytes(self):
        """Independent open B1693 witness; never weaken B1634c's guard above.

        Run this named test alone to reproduce the pre-existing ref-name tie:
        runner_lib.sh materializes plan-second before its parent plan-source,
        despite the git ancestry. A future ancestry-order repair should turn
        this into an unexpected success until this explicit debt marker closes.
        """
        _, result = self.run_case("overlap_same_second")
        second = json.loads((result / "plan-second.json").read_text())
        expected = git(result / "run-1.repo", "show", second["applied_commit_sha"] + ":main.go")
        self.assertEqual((result / "run-1.applied-tree/main.go").read_text().strip(), expected)


if __name__ == "__main__":
    if "--fake-cli" in sys.argv:
        fake_cli()
    else:
        unittest.main()
