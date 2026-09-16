#!/usr/bin/env python3
"""Public shell-entry delivery regressions using real, temporary Git objects.

No model, live eval, fixture rewrite, or product verifier is invoked.  The seed
receipt is deliberately written directly: a missing production helper must not
turn a fixture setup error into the alleged RED for materialization.
"""

import json
import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent.parent


def dump(path, value):
    Path(path).write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


class WriteDeliveryMaterializeTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="codrax-delivery-public-")
        self.addCleanup(self.temp.cleanup)
        self.work = Path(self.temp.name).resolve()
        self.repo = self.work / "repo"
        self.out = self.work / "out"
        self.repo.mkdir()
        self.out.mkdir()
        self.git("init", "-q", "-b", "main")
        self.git("config", "core.filemode", "true")
        for name, content in {
            "main.txt": "seed\n",
            "keep.txt": "untouched\n",
            "delete-me.txt": "old deletion target\n",
            "rename-old.txt": "rename payload\n",
            "mode.sh": "#!/bin/sh\nexit 0\n",
        }.items():
            (self.repo / name).write_text(content)
        self.git("add", "-A")
        self.git("commit", "-qm", "seed", stamp=1700000000)
        self.seed = self.git("rev-parse", "HEAD")
        self.seed_tree = self.git("rev-parse", "HEAD^{tree}")
        self.seed_path = self.out / "run-1.delivery-seed.json"
        dump(self.seed_path, {
            "schema_version": 1, "repo_root": str(self.repo),
            "commit_sha": self.seed, "tree_sha": self.seed_tree,
        })
        self.plan_path = self.out / "run-1.plan.json"
        self.final_path = self.out / "plan-final.final.json"
        self.receipt_path = self.out / "run-1.materialization.json"
        self.tree = self.out / "run-1.applied-tree"
        self.owners = []
        self.plans = {}

    def git(self, *args, stamp=1700000010, input=None):
        env = {**os.environ, "GIT_AUTHOR_DATE": f"{stamp} +0000",
               "GIT_COMMITTER_DATE": f"{stamp} +0000"}
        result = subprocess.run(
            ["git", "-C", str(self.repo), "-c", "user.name=delivery-test",
             "-c", "user.email=delivery-test@codrax", *args],
            text=True, capture_output=True, env=env, check=True, input=input,
        )
        return result.stdout.strip()

    def commit(self, plan_id, edits, *, base=None, paths=None, status="applied"):
        if base is not None:
            self.git("checkout", "--detach", "-q", base)
        for name, value in edits.items():
            path = self.repo / name
            if path.is_symlink() or path.is_file():
                path.unlink()
            if value is None:
                continue
            path.parent.mkdir(parents=True, exist_ok=True)
            if isinstance(value, tuple) and value[0] == "symlink":
                path.symlink_to(value[1])
            else:
                content, mode = (value[1], value[2]) if isinstance(value, tuple) else (value, 0o644)
                path.write_bytes(content if isinstance(content, bytes) else content.encode())
                path.chmod(mode)
        self.git("add", "-A")
        self.git("commit", "--allow-empty", "-qm", plan_id)
        sha = self.git("rev-parse", "HEAD")
        ref = "refs/codrax/applied/" + plan_id
        self.git("update-ref", ref, sha)
        allowed = list(edits) if paths is None else list(paths)
        plan = {
            "id": plan_id, "status": status, "applied_commit_sha": sha,
            "changes": [{"path": path, "kind": "modify"} for path in allowed],
            "applied_paths": allowed, "worktree_base_sha": self.seed,
            "apply_checkpoint": {"commit_sha": sha, "recovery_ref": ref,
                                 "committed_paths": allowed},
        }
        self.plans[plan_id] = plan
        dump(self.out / (plan_id + ".json"), plan)
        self.owners.append({"plan_id": plan_id, "commit_sha": sha, "paths": allowed})
        return sha

    def publish(self, roots, *, owners=None, materialization_status="available"):
        # The scratch checkout remains the original seed, as in real write mode.
        self.git("checkout", "--detach", "-q", self.seed)
        dump(self.plan_path, {
            "id": "plan-final", "status": "applied", "changes": None,
            "persistence_kind": "proof_probe_only", "worktree_path": str(self.repo),
            "verification_probes": [{"id": "probe-only-not-an-owner"}],
        })
        final = {
            "kind": "final_report", "run_id": "wf-test", "plan": {"id": "plan-final"},
            "delivery": {"materialization": {
                "schema_version": 1, "status": materialization_status,
                "run_id": "wf-test", "final_plan_id": "plan-final",
                "retained_plan_ids": list(roots),
                "owners": self.owners if owners is None else owners,
            }},
        }
        dump(self.final_path, final)
        return final

    def shell(self, function="eval_materialize_write_apply_source", path=None):
        args = [str(self.plan_path), str(self.out), str(self.repo), "1"]
        if path is not None:
            args.append(path)
        script = "source " + shlex.quote(str(ROOT / "eval/runner_lib.sh")) + "\n"
        script += function + " " + " ".join(shlex.quote(arg) for arg in args)
        return subprocess.run(["bash", "-c", script], cwd=ROOT, text=True,
                              capture_output=True, timeout=30)

    def capture(self, repo=None):
        return subprocess.run(
            ["python3", str(ROOT / "eval/write_delivery_materialize.py"), "capture",
             "--repo", str(self.repo if repo is None else repo), "--seed", str(self.seed_path)],
            cwd=ROOT, text=True, capture_output=True, timeout=30,
        )

    def materialize(self):
        result = self.shell()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stdout.strip(), str(self.tree))
        self.assertTrue(self.tree.is_dir())
        return self.tree

    def assert_receipt(self, status):
        self.assertTrue(self.receipt_path.is_file(), "public entry did not write materialization receipt")
        receipt = json.loads(self.receipt_path.read_text())
        self.assertEqual(receipt["status"], status, receipt)
        if status == "invalid":
            self.assertTrue(receipt.get("reason") or receipt.get("reason_code"), receipt)
        return receipt

    def assert_invalid(self, *, post_path=None):
        result = self.shell() if post_path is None else self.shell("eval_post_apply_source_file", post_path)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stdout.strip(), "", "invalid delivery leaked a fallback source")
        self.assert_receipt("invalid")

    def test_same_second_descendant_wins_even_with_reversed_roots(self):
        ancestor = self.commit("plan-source", {"main.txt": "first\n"})
        self.commit("plan-second", {"main.txt": "second\n"}, base=ancestor)
        self.publish(["plan-second", "plan-source"])
        tree = self.materialize()
        self.assertEqual((tree / "main.txt").read_text(), "second\n")
        self.assert_receipt("resolved")

    def test_sibling_disjoint_paths_union_independent_of_root_order(self):
        self.commit("plan-left", {"left.txt": "left\n"}, base=self.seed)
        self.commit("plan-right", {"right.txt": "right\n"}, base=self.seed)
        self.publish(["plan-right", "plan-left"])
        tree = self.materialize()
        self.assertEqual((tree / "left.txt").read_text(), "left\n")
        self.assertEqual((tree / "right.txt").read_text(), "right\n")
        self.assertEqual((tree / "keep.txt").read_text(), "untouched\n")
        self.assert_receipt("resolved")

    def test_sibling_overlapping_paths_are_ambiguous(self):
        self.commit("plan-left", {"main.txt": "left\n"}, base=self.seed)
        self.commit("plan-right", {"main.txt": "right\n"}, base=self.seed)
        self.publish(["plan-left", "plan-right"])
        self.assert_invalid()

    def test_sibling_file_child_conflict_is_rejected(self):
        self.commit("plan-file", {"node": "file\n"}, base=self.seed)
        self.commit("plan-child", {"node/child": "child\n"}, base=self.seed)
        self.publish(["plan-file", "plan-child"])
        self.assert_invalid()

    def test_restore_root_b_keeps_a_b_and_excludes_later_c(self):
        a = self.commit("plan-a", {"a.txt": "A\n"}, base=self.seed)
        b = self.commit("plan-b", {"b.txt": "B\n"}, base=a)
        self.commit("plan-c", {"main.txt": "C discarded\n"}, base=b)
        self.publish(["plan-b"])
        tree = self.materialize()
        self.assertEqual((tree / "a.txt").read_text(), "A\n")
        self.assertEqual((tree / "b.txt").read_text(), "B\n")
        self.assertEqual((tree / "main.txt").read_text(), "seed\n")
        self.assert_receipt("resolved")

    def test_unselected_historical_ref_does_not_pollute_delivery(self):
        self.commit("plan-good", {"main.txt": "good\n"}, base=self.seed)
        good = list(self.owners)
        self.commit("plan-z-unrelated", {"main.txt": "stale\n", "pollution.txt": "bad\n"}, base=self.seed)
        self.publish(["plan-good"], owners=good)
        tree = self.materialize()
        self.assertEqual((tree / "main.txt").read_text(), "good\n")
        self.assertFalse((tree / "pollution.txt").exists())
        self.assert_receipt("resolved")

    def test_unlisted_intermediate_same_path_preimage_is_rejected(self):
        hidden = self.commit("plan-hidden", {"main.txt": "hidden\n"}, base=self.seed)
        self.commit("plan-visible", {"main.txt": "hidden\nvisible\n"}, base=hidden)
        self.publish(["plan-visible"], owners=[self.owners[-1]])
        self.assert_invalid()

    def test_noop_and_authorized_path_superset_are_not_false_mutations(self):
        a = self.commit("plan-a", {"main.txt": "changed\n"}, paths=["main.txt", "keep.txt"])
        self.commit("plan-noop", {}, base=a, paths=["main.txt", "keep.txt"])
        self.publish(["plan-noop"])
        tree = self.materialize()
        self.assertEqual((tree / "main.txt").read_text(), "changed\n")
        self.assertEqual((tree / "keep.txt").read_text(), "untouched\n")
        self.assert_receipt("resolved")

    def test_empty_delivery_is_seed_not_arbitrary_ref(self):
        self.commit("plan-unselected", {"main.txt": "not selected\n"})
        self.publish([], owners=[])
        tree = self.materialize()
        self.assertEqual((tree / "main.txt").read_text(), "seed\n")
        self.assert_receipt("resolved")

    def test_empty_checkpoint_path_list_has_zero_delta(self):
        self.commit("plan-empty", {}, paths=[])
        self.publish(["plan-empty"])
        tree = self.materialize()
        self.assertEqual((tree / "main.txt").read_text(), "seed\n")
        self.assert_receipt("resolved")

    def test_empty_checkpoint_paths_go_omitempty_is_supported(self):
        self.commit("plan-empty", {}, paths=[])
        plan = self.plans["plan-empty"]
        for spelling in ["omitted", "null", "array"]:
            with self.subTest(spelling=spelling):
                if spelling == "omitted":
                    plan["apply_checkpoint"].pop("committed_paths", None)
                else:
                    plan["apply_checkpoint"]["committed_paths"] = None if spelling == "null" else []
                dump(self.out / "plan-empty.json", plan)
                self.publish(["plan-empty"])
                tree = self.materialize()
                self.assertEqual((tree / "main.txt").read_text(), "seed\n")
                self.assert_receipt("resolved")

    def test_nonempty_delta_cannot_borrow_empty_checkpoint_paths(self):
        self.commit("plan-owner", {"main.txt": "changed\n"})
        plan = self.plans["plan-owner"]
        plan["apply_checkpoint"].pop("committed_paths")
        dump(self.out / "plan-owner.json", plan)
        self.owners[0]["paths"] = []
        self.publish(["plan-owner"])
        self.assert_invalid()

    def test_delete_rename_mode_and_symlink_are_preserved(self):
        self.commit("plan-effects", {
            "delete-me.txt": None, "rename-old.txt": None,
            "rename-new.txt": "rename payload\n",
            "mode.sh": ("file", "#!/bin/sh\nexit 0\n", 0o755),
            "link": ("symlink", "keep.txt"),
        })
        self.publish(["plan-effects"])
        tree = self.materialize()
        self.assertFalse(os.path.lexists(tree / "delete-me.txt"))
        self.assertFalse(os.path.lexists(tree / "rename-old.txt"))
        self.assertEqual((tree / "rename-new.txt").read_text(), "rename payload\n")
        self.assertEqual((tree / "mode.sh").stat().st_mode & 0o111, 0o111)
        self.assertTrue((tree / "link").is_symlink())
        self.assertEqual(os.readlink(tree / "link"), "keep.txt")
        self.assert_receipt("resolved")

    def test_deleted_post_apply_file_never_falls_back_to_seed(self):
        self.commit("plan-delete", {"delete-me.txt": None})
        self.publish(["plan-delete"])
        result = self.shell("eval_post_apply_source_file", "delete-me.txt")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stdout.strip(), "", "deleted file resurrected from scratch fallback")
        self.assert_receipt("resolved")

    def test_post_apply_external_symlink_is_preserved_but_not_read(self):
        (self.out / "outside.txt").write_text("outside must not be consumed\n")
        self.commit("plan-link", {"main.txt": ("symlink", "../outside.txt")})
        self.publish(["plan-link"])
        tree = self.materialize()
        self.assertTrue((tree / "main.txt").is_symlink())
        self.assertEqual(os.readlink(tree / "main.txt"), "../outside.txt")
        result = self.shell("eval_post_apply_source_file", "main.txt")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stdout.strip(), "", "external symlink became a readable delivery file")
        self.assert_receipt("resolved")

    def test_post_apply_internal_symlink_remains_readable(self):
        self.commit("plan-link", {"main.txt": ("symlink", "keep.txt")})
        self.publish(["plan-link"])
        result = self.shell("eval_post_apply_source_file", "main.txt")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stdout.strip(), str(self.tree / "main.txt"))
        self.assertEqual(Path(result.stdout.strip()).read_text(), "untouched\n")
        self.assert_receipt("resolved")

    def test_export_attributes_do_not_change_literal_git_bytes(self):
        self.commit("plan-attrs", {
            ".gitattributes": "hidden.txt export-ignore\nliteral.txt export-subst\n",
            "hidden.txt": "must survive\n", "literal.txt": "$Format:%H$\n",
        })
        self.publish(["plan-attrs"])
        tree = self.materialize()
        self.assertTrue((tree / "hidden.txt").is_file(), "export-ignore suppressed delivered bytes")
        self.assertEqual((tree / "hidden.txt").read_text(), "must survive\n")
        self.assertEqual((tree / "literal.txt").read_text(), "$Format:%H$\n")
        self.assert_receipt("resolved")

    def test_literal_pathspec_characters_and_unicode(self):
        names = ["-dash.txt", "a[1].txt", "a1.txt", "literal*.txt", ":(glob)*.txt", "odd space Ω.txt"]
        self.commit("plan-names", {name: name + "\n" for name in names})
        self.publish(["plan-names"])
        tree = self.materialize()
        for name in names:
            self.assertEqual((tree / name).read_text(), name + "\n")
        self.assert_receipt("resolved")

    def test_git_case_collision_is_exact_or_explicitly_invalid(self):
        # Construct the Git tree through the index: a case-folding host cannot
        # create these distinct leaves by writing its working tree normally.
        names = {"Case.txt": "UPPER\n", "case.txt": "lower\n"}
        self.commit("plan-case", {}, paths=list(names))
        self.git("read-tree", self.seed)
        for name, content in names.items():
            blob = self.git("hash-object", "-w", "--stdin", input=content)
            self.git("-c", "core.ignorecase=false", "update-index", "--add",
                     "--cacheinfo", "100644," + blob + "," + name)
        tree_id = self.git("write-tree")
        sha = self.git("commit-tree", tree_id, "-p", self.seed, "-m", "literal case collision")
        self.git("read-tree", "HEAD")
        plan = self.plans["plan-case"]
        plan["applied_commit_sha"] = sha
        plan["apply_checkpoint"]["commit_sha"] = sha
        dump(self.out / "plan-case.json", plan)
        self.git("update-ref", "refs/codrax/applied/plan-case", sha)
        self.owners[0]["commit_sha"] = sha
        self.publish(["plan-case"])
        result = self.shell()
        if result.returncode:
            self.assertEqual(result.stdout.strip(), "")
            receipt = self.assert_receipt("invalid")
            self.assertIn(receipt.get("reason_code"), {
                "materialized_literal_paths_mismatch",
                "materialized_blob_mismatch:Case.txt",
                "materialized_blob_mismatch:case.txt",
            }, "collision must fail during exact filesystem verification, not fixture setup")
        else:
            self.assertEqual(result.stdout.strip(), str(self.tree))
            actual_names = {path.name for path in self.tree.iterdir()}
            self.assertTrue(set(names) <= actual_names, actual_names)
            for name, content in names.items():
                self.assertEqual((self.tree / name).read_text(), content)
            self.assert_receipt("resolved")

    def test_verified_state_is_not_required_for_durable_owner(self):
        self.commit("plan-owner", {"main.txt": "real applied bytes\n"}, status="verify_failed")
        self.publish(["plan-owner"])
        tree = self.materialize()
        self.assertEqual((tree / "main.txt").read_text(), "real applied bytes\n")
        self.assert_receipt("resolved")

    def test_owner_from_unrelated_commit_history_is_rejected(self):
        # A real commit object with the same repository's tree but no seed
        # ancestry is not authorized merely because its ref name looks right.
        sha = self.commit("plan-foreign", {"main.txt": "foreign\n"})
        foreign = self.git("commit-tree", self.git("rev-parse", sha + "^{tree}"),
                           "-m", "unrelated root")
        plan = self.plans["plan-foreign"]
        plan["applied_commit_sha"] = foreign
        plan["apply_checkpoint"]["commit_sha"] = foreign
        dump(self.out / "plan-foreign.json", plan)
        self.git("update-ref", "refs/codrax/applied/plan-foreign", foreign)
        self.owners[0]["commit_sha"] = foreign
        self.publish(["plan-foreign"])
        self.assert_invalid()

    def test_invalid_authority_never_uses_live_or_seed_fallback(self):
        self.commit("plan-owner", {"main.txt": "changed\n"})
        valid_owner = dict(self.owners[0])
        valid_plan = json.loads(json.dumps(self.plans["plan-owner"]))
        valid_seed = json.loads(self.seed_path.read_text())
        for variant in ["no_seed", "wrong_seed_tree", "wrong_seed_repo", "unknown_status",
                        "missing_root", "missing_plan", "partial", "checkpoint_mismatch", "bad_recovery_ref",
                        "traversal", "foreign_run", "foreign_final", "uncovered_delta"]:
            with self.subTest(variant=variant):
                dump(self.seed_path, valid_seed)
                dump(self.out / "plan-owner.json", valid_plan)
                final = self.publish(["plan-owner"], owners=[dict(valid_owner)])
                materialization = final["delivery"]["materialization"]
                if variant == "no_seed":
                    self.seed_path.unlink()
                elif variant == "wrong_seed_tree":
                    dump(self.seed_path, {**valid_seed, "tree_sha": self.git("rev-parse", valid_owner["commit_sha"] + "^{tree}")})
                elif variant == "wrong_seed_repo":
                    dump(self.seed_path, {**valid_seed, "repo_root": str(self.out)})
                elif variant == "unknown_status":
                    materialization["status"] = "future-unknown-status"
                elif variant == "missing_root":
                    materialization["retained_plan_ids"] = ["plan-absent"]
                elif variant == "missing_plan":
                    (self.out / "plan-owner.json").unlink()
                elif variant in {"partial", "checkpoint_mismatch", "bad_recovery_ref"}:
                    altered = json.loads(json.dumps(valid_plan))
                    if variant == "partial":
                        altered["apply_checkpoint"]["partial"] = True
                    elif variant == "checkpoint_mismatch":
                        altered["apply_checkpoint"]["commit_sha"] = self.seed
                    else:
                        altered["apply_checkpoint"]["recovery_ref"] = {"unexpected": "object"}
                    dump(self.out / "plan-owner.json", altered)
                elif variant == "traversal":
                    materialization["owners"][0]["paths"] = ["../escape"]
                elif variant == "foreign_run":
                    materialization["run_id"] = "wf-other"
                elif variant == "foreign_final":
                    materialization["final_plan_id"] = "plan-other"
                elif variant == "uncovered_delta":
                    materialization["owners"][0]["paths"] = ["keep.txt"]
                dump(self.final_path, final)
                self.assert_invalid(post_path="main.txt")

    def test_missing_terminal_plan_has_no_post_apply_fallback(self):
        self.commit("plan-owner", {"main.txt": "changed\n"})
        self.publish(["plan-owner"])
        self.plan_path.unlink()
        result = self.shell("eval_post_apply_source_file", "main.txt")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stdout.strip(), "", "missing plan exposed scratch source")

    def test_exact_owner_artifact_cannot_be_replaced_by_similar_name(self):
        self.commit("plan-owner", {"main.txt": "changed\n"})
        self.publish(["plan-owner"])
        exact = self.out / "plan-owner.json"
        exact.rename(self.out / "plan-owner-archived.json")
        self.assert_invalid()

    def test_owner_artifact_symlink_cannot_escape_exact_root(self):
        self.commit("plan-owner", {"main.txt": "changed\n"})
        self.publish(["plan-owner"])
        exact = self.out / "plan-owner.json"
        external = self.work / "outside-plan.json"
        exact.rename(external)
        exact.symlink_to(external)
        self.assert_invalid()

    def test_duplicate_json_authority_is_rejected(self):
        self.commit("plan-owner", {"main.txt": "changed\n"})
        self.publish(["plan-owner"])
        raw = self.final_path.read_text()
        self.final_path.write_text(raw.replace('"run_id": "wf-test",',
                                               '"run_id": "wf-other", "run_id": "wf-test",', 1))
        self.assert_invalid()

    def test_capture_records_canonical_repo_commit_and_tree(self):
        self.seed_path.unlink()
        alias = self.work / "repo-alias"
        alias.symlink_to(self.repo, target_is_directory=True)
        result = self.capture(alias)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(json.loads(self.seed_path.read_text()), {
            "schema_version": 1, "repo_root": str(self.repo),
            "commit_sha": self.seed, "tree_sha": self.seed_tree,
        })

    def test_capture_cannot_overwrite_seed_after_head_changes(self):
        original = self.seed_path.read_bytes()
        self.commit("plan-later", {"main.txt": "later\n"})
        self.assertNotEqual(self.git("rev-parse", "HEAD"), self.seed)
        result = self.capture()
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.seed_path.read_bytes(), original)

    def test_capture_invalid_repo_does_not_create_seed(self):
        self.seed_path.unlink()
        result = self.capture(self.work / "missing-repo")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(os.path.lexists(self.seed_path))


if __name__ == "__main__":
    unittest.main()
