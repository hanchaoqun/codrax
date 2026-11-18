#!/usr/bin/env python3
"""Zero-model self-tests for the HMC batch tools.

  python3 -B eval/hmc_tools/hmc_tools_test.py

Everything runs in temporary directories with a throw-away git repository, a
ten-line stand-in for eval/run.sh and a stub product binary. No network, no
provider file, no model. The overlap test uses the real
eval/request_intervals.py of the repository the tools live in (override with
HMC_TOOLS_TEST_REPO when running the tools from outside a checkout).
"""
import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest

sys.dont_write_bytecode = True
HERE = os.path.dirname(os.path.realpath(__file__))
sys.path.insert(0, HERE)

import hmc_common as c  # noqa: E402
import suite_stats  # noqa: E402

REPO = os.environ.get("HMC_TOOLS_TEST_REPO") or c.default_repo()

FAKE_RUN_SH = r"""#!/bin/bash
# Stand-in with the same contract as eval/run.sh: <case-file> <N>, CODRAX_BIN,
# CODRAX_PROVIDER_ARGS_RAW, EVAL_RESULTS_ROOT, one product entry per run.
set -u
MODE=""; REQUIRED=""
source "$1"; N="$2"
[[ "$N" == "1" ]] || { echo "unexpected N=$N" >&2; exit 2; }
out="$EVAL_RESULTS_ROOT/$ID-20260101-000000"; mkdir -p "$out/run-1.logs"
[[ -n "$REQUIRED" ]] && { echo "SKIP missing:$REQUIRED" >"$out/run-1.verdict"; exit 0; }
read -r -a providers <<<"$CODRAX_PROVIDER_ARGS_RAW"
extra=(); [[ "$MODE" == "apply" ]] && extra=(--mode=write)
"$CODRAX_BIN" "${providers[@]}" --repo . --branch main ${extra[@]+"${extra[@]}"} \
  --log-dir "$out/run-1.logs" --request "$QUESTION" >"$out/run-1.out" 2>&1
if [[ "${TWO_LANES:-}" == "1" ]]; then  # plan lane then apply lane of one run
  "$CODRAX_BIN" "${providers[@]}" --repo . --branch main --mode=write --write-phase=apply \
    --log-dir "$out/run-1.logs" --request "$QUESTION" >>"$out/run-1.out" 2>&1
fi
echo "FAIL stub exit=$?" >"$out/run-1.verdict"
"""

STUB_BIN = r"""#!/bin/bash
logdir=""; prev=""; for a in "$@"; do [[ "$prev" == "--log-dir" ]] && logdir="$a"; prev="$a"; done
printf '%s\n' "$@"
exec python3 -B - "$logdir" "$$" <<'PY'
import datetime, sys, time
log = open("%s/codrax-stub-%s.log" % (sys.argv[1], sys.argv[2]), "w")
for kind, pause in (("request", 0.5), ("response", 0.0)):
    log.write("%s DEBUG [llm] %s: model=stub\n" % (datetime.datetime.now().isoformat(timespec="milliseconds"), kind))
    log.flush(); time.sleep(pause)
PY
"""


def run(tool, *args, **kwargs):
    done = subprocess.run([sys.executable, "-B", os.path.join(HERE, tool)] + list(args),
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs)
    return done.returncode, done.stdout.decode("utf-8"), done.stderr.decode("utf-8")


def git(repo, *args):
    subprocess.run(["git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.invalid"] + list(args),
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def write(path, text, mode=None):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(text)
    if mode:
        os.chmod(path, mode)


class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = os.path.realpath(tempfile.mkdtemp(prefix="hmc-tools-test-"))
        self.addCleanup(self._cleanup)

    def _cleanup(self):
        for base, dirs, files in os.walk(self.tmp):
            for name in dirs + files:
                path = os.path.join(base, name)
                if not os.path.islink(path):
                    os.chmod(path, os.stat(path).st_mode | stat.S_IWUSR | stat.S_IRUSR)
        shutil.rmtree(self.tmp, ignore_errors=True)

    def repo(self):
        repo = os.path.join(self.tmp, "repo")
        write(os.path.join(repo, "eval", "run.sh"), FAKE_RUN_SH)
        write(os.path.join(repo, "internal", "a", "a.go"), "package a\n")
        write(os.path.join(repo, "internal", "config", "providers.go"), "package config\n")
        write(os.path.join(repo, "docs", "note.md"), "doc\n")
        write(os.path.join(repo, "README.md"), "readme\n")
        write(os.path.join(repo, "go.mod"), "module x\n")
        write(os.path.join(repo, ".gitignore"), "/providers.yaml\n/eval/results/\n")
        write(os.path.join(repo, "providers.yaml"), "secret: never read\n")
        write(os.path.join(repo, "eval", "cases", "a.case"), 'ID="case_a"\nQUESTION="read question"\n')
        write(os.path.join(repo, "eval", "cases", "b.case"), 'ID="case_b"\nMODE="apply"\nQUESTION="write question"\n')
        write(os.path.join(repo, "eval", "cases", "skip.case"), 'ID="case_s"\nREQUIRED="x"\nQUESTION="q"\n')
        write(os.path.join(repo, "eval", "cases", "two.case"),
              'ID="case_t"\nMODE="apply"\nTWO_LANES=1\nQUESTION="write question"\n')
        shutil.copy(os.path.join(REPO, "eval", "request_intervals.py"), os.path.join(repo, "eval"))
        git(repo, "init", "-q")
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "base")
        return repo


class SuiteStats(Base):
    def events(self, *rows):
        return [json.dumps(row).encode() if isinstance(row, dict) else row for row in rows]

    def test_counts_duplicates_unfinished_and_non_json(self):
        p = "example/pkg"
        stats = suite_stats.stats_from_lines(self.events(
            {"Action": "start", "Package": p},
            {"Action": "run", "Package": p, "Test": "TestA"},
            {"Action": "run", "Package": p, "Test": "TestA/sub"},
            {"Action": "output", "Package": p, "Test": "TestA/sub", "Output": "x"},
            {"Action": "pass", "Package": p, "Test": "TestA/sub"},
            {"Action": "pass", "Package": p, "Test": "TestA"},
            {"Action": "pass", "Package": p, "Test": "TestA"},          # duplicate terminal
            {"Action": "run", "Package": p, "Test": "TestB"},
            {"Action": "fail", "Package": p, "Test": "TestB"},
            {"Action": "run", "Package": p, "Test": "TestC"},
            {"Action": "skip", "Package": p, "Test": "TestC"},
            {"Action": "run", "Package": p, "Test": "TestHang"},        # never terminal
            b"panic: not json", b"", b"[1,2]",
            {"Action": "fail", "Package": p},
            {"Action": "start", "Package": "example/empty"},
            {"Action": "skip", "Package": "example/empty"},
        ))
        self.assertEqual(stats["counts"], {"sub_pass": 1, "top_pass": 2, "top_fail": 1, "top_skip": 1,
                                           "package_fail": 1, "package_skip": 1})
        self.assertEqual(stats["duplicates"], [[p, "TestA"]])
        self.assertEqual(stats["unfinished"], [[p, "TestHang"]])
        self.assertEqual(stats["non_json"], 3)
        self.assertEqual(stats["failed"], [[p, "TestB"], [p, None]])

    def test_run_records_formal_exit_and_never_overwrites(self):
        repo, out = self.repo(), os.path.join(self.tmp, "ev")
        code, text, err = run("suite_stats.py", "run", "--label", "raw", "--out-dir", out, "--repo", repo,
                              "--raw", "--", "bash", "-c", "echo hi; exit 3")
        self.assertEqual(code, 1, err)
        receipt = c.load_json(os.path.join(out, "raw-receipt.json"))
        self.assertEqual((receipt["exit_code"], receipt["signal"], receipt["source_unchanged"]), (3, None, True))
        code, _, err = run("suite_stats.py", "run", "--label", "raw", "--out-dir", out, "--repo", repo,
                           "--raw", "--", "true")
        self.assertEqual(code, 2)
        self.assertIn("refusing to overwrite", err)

    def test_run_detects_a_command_that_edits_source(self):
        repo, out = self.repo(), os.path.join(self.tmp, "ev")
        code, _, _ = run("suite_stats.py", "run", "--label", "edit", "--out-dir", out, "--repo", repo, "--raw",
                         "--", "bash", "-c", "echo '// x' >> internal/a/a.go")
        self.assertEqual(code, 1)
        self.assertFalse(c.load_json(os.path.join(out, "edit-receipt.json"))["source_unchanged"])


class Freeze(Base):
    def test_inputs_changed_external_and_verify(self):
        repo = self.repo()
        inputs, legacy = c.source_inputs(repo)
        self.assertIn("internal/config/providers.go", inputs)       # tracked source is frozen
        self.assertEqual(legacy, ["internal/config/providers.go"])
        self.assertNotIn("providers.yaml", inputs)                   # ignored credential never listed
        self.assertNotIn("docs/note.md", inputs)
        self.assertNotIn("README.md", inputs)
        base = subprocess.check_output(["git", "-C", repo, "rev-parse", "HEAD"]).decode().strip()
        write(os.path.join(repo, "internal", "a", "a.go"), "package a\n// changed\n")
        write(os.path.join(repo, "internal", "a", "new_test.go"), "package a\n")
        external = os.path.join(self.tmp, "original.ftrace")
        write(external, "trace bytes\n")
        manifest = os.path.join(self.tmp, "frozen.json")
        code, text, err = run("freeze.py", "write", "--repo", repo, "--base", base, "--out", manifest,
                              "--external", external, "--snapshot-dir", os.path.join(self.tmp, "snap"))
        self.assertEqual(code, 0, err)
        counts = json.loads(text.splitlines()[0])
        self.assertEqual((counts["external"], counts["changed"]), (1, 2))
        self.assertEqual(counts["inputs"], len(inputs) + 1)
        self.assertEqual(sorted(os.listdir(os.path.join(self.tmp, "snap"))), ["001-a.go.txt", "002-new_test.go.txt"])
        self.assertEqual(run("freeze.py", "verify", "--manifest", manifest)[0], 0)
        write(external, "trace bytes tampered\n")
        code, text, _ = run("freeze.py", "verify", "--manifest", manifest)
        self.assertEqual(code, 1)
        self.assertIn("external modified", text)
        write(external, "trace bytes\n")
        write(os.path.join(repo, "internal", "a", "a.go"), "package a\n// changed again\n")
        code, text, _ = run("freeze.py", "verify", "--manifest", manifest)
        self.assertEqual(code, 1)
        self.assertIn("source modified: internal/a/a.go", text)

    def test_refusals(self):
        repo = self.repo()
        manifest = os.path.join(self.tmp, "frozen.json")
        write(os.path.join(repo, "docs", "note.md"), "doc changed\n")
        code, _, err = run("freeze.py", "write", "--repo", repo, "--base", "HEAD", "--out", manifest)
        self.assertEqual(code, 2)
        self.assertIn("not frozen source inputs", err)
        git(repo, "checkout", "--", "docs/note.md")
        code, _, err = run("freeze.py", "write", "--repo", repo, "--base", "HEAD", "--out", manifest,
                           "--snapshot-dir", os.path.join(repo, "snap"))
        self.assertIn("outside the repository", err)
        code, _, err = run("freeze.py", "write", "--repo", repo, "--base", "HEAD", "--out", manifest,
                           "--external", os.path.join(repo, "providers.yaml"))
        self.assertEqual(code, 2)
        self.assertIn("credential", err)
        write(os.path.join(repo, "internal", ".env.local"), "x")
        code, _, err = run("freeze.py", "write", "--repo", repo, "--base", "HEAD", "--out", manifest)
        self.assertIn("untracked credential-named", err)
        self.assertFalse(os.path.exists(manifest))


class Seal(Base):
    def test_round_trip_tamper_refuse_and_restore(self):
        root, work = os.path.join(self.tmp, "root"), os.path.join(self.tmp, "work")
        write(os.path.join(root, "A", "case-1", "run-1.out"), "answer\n")
        write(os.path.join(root, "A", "case-1", "run-1.repo", ".git", "HEAD"), "ref\n")
        write(os.path.join(work, "001-agent.go"), "package x\n")
        write(os.path.join(work, "stats.json"), '{"counts": {}}\n')
        code, text, err = run("seal.py", "seal", "--root", root, "--tree", root, "--tree", work,
                              "--attach", "full=" + os.path.join(work, "stats.json"))
        self.assertEqual(code, 0, err)
        index = c.load_json(os.path.join(root, "sealed-index-v1.json"))
        self.assertEqual([row["archived"] for row in index],
                         ["sealed-v1/0001-run-1.out", "sealed-v1/0002-001-agent.go.txt", "sealed-v1/0003-stats.json"])
        self.assertEqual(index[0]["sha256"], c.sha256_bytes(b"answer\n"))
        self.assertEqual(index[0]["source"], os.path.join(root, "A", "case-1", "run-1.out"))
        receipt = c.load_json(os.path.join(root, "sealed-receipt-v1.json"))
        self.assertEqual((receipt["count"], receipt["index_sha256"]),
                         (3, c.sha256_file(os.path.join(root, "sealed-index-v1.json"))))
        self.assertEqual(run("seal.py", "verify", "--root", root)[0], 0)

        code, _, err = run("seal.py", "seal", "--root", root, "--tree", work)
        self.assertEqual(code, 2)
        self.assertIn("nothing is rewritten", err)

        restored = os.path.join(self.tmp, "restored.go")
        code, _, err = run("seal.py", "restore", "--root", root, "--source", os.path.join(work, "001-agent.go"),
                           "--to", restored)
        self.assertEqual(code, 0, err)
        with open(restored, encoding="utf-8") as handle:
            self.assertEqual(handle.read(), "package x\n")

        # a successor generation may cite the first as --prior; a tampered prior stops it
        root2 = os.path.join(self.tmp, "root2")
        write(os.path.join(root2, "note.txt"), "n\n")
        piece = os.path.join(root, "sealed-v1", "0001-run-1.out")
        os.chmod(piece, 0o600)
        write(piece, "answeR\n")
        code, text, _ = run("seal.py", "verify", "--root", root)
        self.assertEqual(code, 1)
        self.assertEqual(json.loads(text.splitlines()[0])["sha_mismatch"], 1)
        code, _, err = run("seal.py", "seal", "--root", root2, "--tree", root2, "--prior", root)
        self.assertEqual(code, 2)
        self.assertIn("does not re-verify", err)
        self.assertFalse(os.path.exists(os.path.join(root2, "sealed-v1")))

    def test_credentials_symlinks_and_old_archives_are_refused(self):
        root, work = os.path.join(self.tmp, "root"), os.path.join(self.tmp, "work")
        os.makedirs(root)
        write(os.path.join(work, "providers.yaml"), "k\n")
        self.assertIn("credential", run("seal.py", "seal", "--root", root, "--file", os.path.join(work, "providers.yaml"))[2])
        os.remove(os.path.join(work, "providers.yaml"))
        write(os.path.join(work, "a.txt"), "a\n")
        os.symlink(os.path.join(work, "a.txt"), os.path.join(work, "link.txt"))
        self.assertIn("symlink", run("seal.py", "seal", "--root", root, "--tree", work)[2])
        os.remove(os.path.join(work, "link.txt"))
        write(os.path.join(work, "sealed-v1", "0001-old.txt"), "old\n")
        self.assertIn("old archive", run("seal.py", "seal", "--root", root, "--file",
                                         os.path.join(work, "sealed-v1", "0001-old.txt"))[2])
        self.assertEqual(os.listdir(root), [])


class LaunchAndOverlap(Base):
    def launch(self, repo, root, b_case="b.case", extra=()):
        stub = os.path.join(self.tmp, "stub-codrax")
        if not os.path.exists(stub):
            write(stub, STUB_BIN, 0o700)
        return run("launch.py", "run", "--repo", repo, "--root", root,
                   "--case", "A=" + os.path.join(repo, "eval", "cases", "a.case"),
                   "--case", "B=" + os.path.join(repo, "eval", "cases", b_case),
                   "--bin", stub, "--providers-args", "--providers /nonexistent/providers.yaml",
                   "--timeout", "60", "--barrier-timeout", "10", *extra)

    def test_two_cases_once_each_behind_one_barrier(self):
        repo, root = self.repo(), os.path.join(self.tmp, "batch")
        code, text, err = self.launch(repo, root, extra=("--env", "A:ONLY_A=1"))
        self.assertEqual(code, 0, text + err)
        barrier = os.path.join(root, "barrier")
        a, b = c.load_json(os.path.join(barrier, "A.json")), c.load_json(os.path.join(barrier, "B.json"))
        release = c.load_json(os.path.join(barrier, "release.json"))
        go = [c.load_json(os.path.join(barrier, side + ".go.json"))["at"] for side in "AB"]
        self.assertLessEqual(max(a["at"], b["at"]), release["at"])   # both arrived before the release
        self.assertLessEqual(release["at"], min(go))                 # neither started before it
        self.assertEqual(a["argv"][-2:], ["--root-causes-out", os.path.join(a["run_dir"], "run-1.root-causes.json")])
        self.assertNotIn("--root-causes-out", b["argv"])             # write lane untouched
        self.assertIn("--mode=write", b["argv"])
        self.assertEqual(sorted(os.listdir(os.path.join(root, "A"))), ["case_a-20260101-000000"])
        self.assertEqual(sorted(os.listdir(os.path.join(root, "B"))), ["case_b-20260101-000000"])
        receipt = c.load_json(os.path.join(root, "launch-A-v1-receipt.json"))
        self.assertEqual(receipt["argv"][:2] + receipt["argv"][3:], ["bash", "eval/run.sh", "1"])
        self.assertEqual((receipt["exit_code"], receipt["env"]["ONLY_A"]), (0, "1"))
        self.assertNotIn("ONLY_A", c.load_json(os.path.join(root, "launch-B-v1-receipt.json"))["env"])
        self.assertEqual(c.load_json(os.path.join(root, "launch-receipt-v1.json"))["product_entries"], {"A": 1, "B": 1})

        code, _, err = self.launch(repo, root)                        # never a second launch
        self.assertEqual(code, 2)
        self.assertIn("already launched", err)

        code, text, err = run("overlap.py", "--repo", repo, "--root", root)
        self.assertEqual(code, 0, err)
        counts = json.loads(text.splitlines()[0])
        self.assertEqual(counts["completed"], {"A": 1, "B": 1})
        self.assertEqual((counts["overlaps"], counts["ambiguous"], counts["actual_overlap_proven"]), (1, 0, True))

    def test_no_side_starts_alone(self):
        repo, root = self.repo(), os.path.join(self.tmp, "batch")
        code, text, err = self.launch(repo, root, b_case="skip.case")
        self.assertEqual(code, 1, text + err)
        barrier = os.path.join(root, "barrier")
        self.assertTrue(os.path.isfile(os.path.join(barrier, "aborted.json")))
        self.assertFalse(os.path.exists(os.path.join(barrier, "release.json")))
        self.assertFalse(os.path.exists(os.path.join(barrier, "A.go.json")))
        self.assertEqual(os.listdir(os.path.join(root, "A", "case_a-20260101-000000", "run-1.logs")), [])

    def test_later_product_entries_of_the_same_run_are_recorded_not_held(self):
        repo, root = self.repo(), os.path.join(self.tmp, "batch")
        code, text, err = self.launch(repo, root, b_case="two.case")
        self.assertEqual(code, 0, text + err)
        second = c.load_json(os.path.join(root, "barrier", "B.entry-2.json"))
        self.assertEqual(second["entry"], 2)
        self.assertIn("--write-phase=apply", second["argv"])
        self.assertEqual(c.load_json(os.path.join(root, "launch-receipt-v1.json"))["product_entries"], {"A": 1, "B": 2})
        code, text, err = run("overlap.py", "--repo", repo, "--root", root)
        self.assertEqual(json.loads(text.splitlines()[0])["completed"], {"A": 1, "B": 2}, err)

    def test_frozen_launch_runs_only_the_frozen_binary_and_reverifies_source(self):
        repo, root = self.repo(), os.path.join(self.tmp, "batch")
        stub = os.path.join(self.tmp, "stub-codrax")
        write(stub, STUB_BIN, 0o700)
        loose, tight = os.path.join(self.tmp, "loose.json"), os.path.join(self.tmp, "tight.json")
        self.assertEqual(run("freeze.py", "write", "--repo", repo, "--base", "HEAD", "--out", loose)[0], 0)
        code, _, err = self.launch(repo, root, extra=("--freeze", loose))
        self.assertEqual(code, 2)
        self.assertIn("not a frozen external input", err)
        self.assertFalse(os.path.exists(root))
        self.assertEqual(run("freeze.py", "write", "--repo", repo, "--base", "HEAD", "--out", tight,
                             "--external", stub)[0], 0)
        code, text, err = self.launch(repo, root, extra=("--freeze", tight))
        self.assertEqual(code, 0, text + err)
        receipt = c.load_json(os.path.join(root, "launch-B-v1-receipt.json"))
        self.assertEqual((receipt["source_verified_before"], receipt["source_verified_after"]), (True, True))
        self.assertEqual(receipt["freeze_sha256"], c.sha256_file(tight))
        self.assertEqual(c.load_json(os.path.join(root, "barrier", "release.json"))["freeze_sha256"],
                         c.sha256_file(tight))

    def test_exactly_two_cases(self):
        repo = self.repo()
        case = os.path.join(repo, "eval", "cases", "a.case")
        stub = os.path.join(self.tmp, "stub-codrax")
        write(stub, STUB_BIN, 0o700)
        for cases in (["A=" + case], ["A=" + case, "B=" + case, "C=" + case]):
            argv = ["run", "--repo", repo, "--root", os.path.join(self.tmp, "r"), "--bin", stub,
                    "--providers-args", "--providers x"]
            for item in cases:
                argv += ["--case", item]
            code, _, err = run("launch.py", *argv)
            self.assertEqual(code, 2)
            self.assertIn("exactly two cases", err)
        self.assertFalse(os.path.exists(os.path.join(self.tmp, "r")))


if __name__ == "__main__":
    unittest.main(verbosity=2)
