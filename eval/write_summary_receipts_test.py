#!/usr/bin/env python3
"""Actual shell receipt handoff with real Git materialization, no model calls."""

import json
from pathlib import Path
import shlex
import subprocess
import unittest

import write_delivery_materialize_test as delivery
from write_summary_receipts import resolve

ROOT = delivery.ROOT
dump = delivery.dump


class WriteSummaryReceiptsTest(unittest.TestCase):
    def setUp(self):
        self.fixture = delivery.WriteDeliveryMaterializeTest("runTest")
        self.fixture.setUp()
        self.addCleanup(self.fixture.doCleanups)
        f = self.fixture
        repo = f.out / "run-1.repo"
        f.repo.rename(repo)
        f.repo = repo
        seed = json.loads(f.seed_path.read_text())
        seed["repo_root"] = str(repo)
        dump(f.seed_path, seed)
        f.commit("plan-source", {"main.txt": "delivered\n"})
        f.plan_path = repo / ".codrax/plans/plan-final.json"
        f.plan_path.parent.mkdir(parents=True)
        f.publish(["plan-source"])
        f.materialize()
        self.receipt = f.out / "run-1.write-apply.json"
        self.capture()

    def capture(self, plan=None, attempted="1"):
        f = self.fixture
        args = [str(self.receipt), str(plan or f.plan_path), str(f.out), str(f.repo), "1", attempted, "0"]
        script = "source " + shlex.quote(str(ROOT / "eval/runner_lib.sh")) + "\n"
        script += "eval_write_apply_result_record " + " ".join(map(shlex.quote, args))
        result = subprocess.run(["bash", "-c", script], cwd=ROOT, text=True, capture_output=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def resolve(self, run="1", scratch=None):
        f = self.fixture
        return resolve(str(f.out), str(scratch or f.repo), run)

    def test_commandless_existing_product_and_resolve_are_read_only(self):
        f = self.fixture
        self.assertFalse((f.out / "run-1.plan.json").exists())
        before = {p: (p.read_bytes(), p.stat().st_mtime_ns) for p in f.out.rglob("*") if p.is_file()}
        result = self.resolve()
        self.assertEqual(result, {"plan_path": str(f.plan_path), "source_path": str(f.tree)})
        self.assertEqual((Path(result["source_path"]) / "main.txt").read_text(), "delivered\n")
        self.assertEqual(before, {p: (p.read_bytes(), p.stat().st_mtime_ns) for p in f.out.rglob("*") if p.is_file()})
        value = json.loads(self.receipt.read_text())
        self.assertTrue(value["plan_written"])
        self.assertFalse(value["verify_authoritative"], "summary must not grant authority")

    def test_explicit_plan_and_plan_without_apply_keep_plan_independent(self):
        f = self.fixture
        f.plan_path.rename(f.out / "run-1.plan.json")
        f.plan_path = f.out / "run-1.plan.json"
        self.capture()
        self.assertEqual(self.resolve()["source_path"], str(f.tree))
        f.receipt_path.unlink()
        self.capture(attempted="0")
        result = self.resolve()
        self.assertEqual(result["plan_path"], str(f.plan_path))
        self.assertEqual(result["source_path"], "")
        self.assertFalse(json.loads(self.receipt.read_text())["apply_attempted"])

    def test_missing_invalid_legacy_or_foreign_receipt_never_guesses_plan(self):
        original = self.receipt.read_bytes()
        for value in (None, b"{", b'{"plan_id":"x","plan_id":"y"}', b'{"plan_written":true,"plan_id":"plan-final"}'):
            with self.subTest(value=value):
                if value is None:
                    self.receipt.unlink()
                else:
                    self.receipt.write_bytes(value)
                result = self.resolve()
                self.assertEqual(result["plan_path"], "")
                self.assertEqual(result["source_path"], "")
                self.receipt.write_bytes(original)
        foreign = self.fixture.out / "run-2.write-apply.json"
        foreign.write_bytes(original)
        self.assertEqual(self.resolve(run="2")["plan_path"], "")
        self.assertEqual(self.resolve(scratch=self.fixture.out / "run-2.repo")["plan_path"], "")

    def test_plan_identity_and_exact_raw_bytes_fail_closed(self):
        f = self.fixture
        original = f.plan_path.read_bytes()
        for raw in (original + b" ", original.replace(b'"plan-final"', b'"foreign-id"')):
            with self.subTest(raw=raw[-20:]):
                f.plan_path.write_bytes(raw)
                result = self.resolve()
                self.assertEqual(result["plan_path"], "")
                self.assertEqual(result["source_path"], "")
        f.plan_path.write_bytes(original)
        receipt = json.loads(self.receipt.read_text())
        receipt["plan_id"] = "other-plan"
        dump(self.receipt, receipt)
        self.assertEqual(self.resolve()["plan_path"], "")

    def test_foreign_plan_and_symlink_cannot_be_captured(self):
        f = self.fixture
        foreign = f.work / "foreign.json"
        foreign.write_bytes(f.plan_path.read_bytes())
        self.capture(plan=foreign)
        self.assertEqual(self.resolve()["plan_path"], "")
        f.plan_path.unlink()
        f.plan_path.symlink_to(foreign)
        self.capture()
        self.assertEqual(self.resolve()["plan_path"], "")

    def test_missing_or_linked_source_keeps_plan_but_not_preview(self):
        f = self.fixture
        moved = f.out / "saved-product"
        f.tree.rename(moved)
        self.assertEqual(self.resolve()["plan_path"], str(f.plan_path))
        self.assertEqual(self.resolve()["source_path"], "")
        f.tree.symlink_to(moved, target_is_directory=True)
        self.assertEqual(self.resolve()["source_path"], "")

    def test_materialization_seed_and_final_are_independently_bound(self):
        f = self.fixture
        mutations = ((f.receipt_path, "final_plan_id", "other-plan"),
                     (f.receipt_path, "run_id", "other-workflow"),
                     (f.receipt_path, "seed_commit_sha", "0" * 40),
                     (f.receipt_path, "path", str(f.out / "run-2.applied-tree")),
                     (f.seed_path, "repo_root", str(f.out / "run-2.repo")),
                     (f.final_path, "run_id", "other-workflow"))
        for file, key, value in mutations:
            with self.subTest(file=file.name, key=key):
                original = file.read_bytes()
                changed = json.loads(original)
                changed[key] = value
                dump(file, changed)
                self.assertEqual(self.resolve()["plan_path"], str(f.plan_path))
                self.assertEqual(self.resolve()["source_path"], "")
                file.write_bytes(original)
        for file in (f.receipt_path, f.seed_path, f.final_path):
            with self.subTest(missing=file.name):
                original = file.read_bytes()
                file.unlink()
                self.assertEqual(self.resolve()["plan_path"], str(f.plan_path))
                self.assertEqual(self.resolve()["source_path"], "")
                file.write_bytes(original)


if __name__ == "__main__":
    unittest.main()
