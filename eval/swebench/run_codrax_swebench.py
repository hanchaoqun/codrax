#!/usr/bin/env python3
"""Generate SWE-bench predictions with Codrax write mode.

The script is an adapter, not a scorer:

1. Read SWE-bench instances from a local JSONL file or Hugging Face dataset.
2. Clone/cache each repository and checkout its `base_commit`.
3. Run `codrax --mode=write` on the instance problem statement.
4. Export the applied Codrax ref as a unified diff prediction.

The official SWE-bench harness remains the authority for scoring.
"""

from __future__ import annotations

import argparse
import ast
import configparser
import hashlib
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import threading
import textwrap
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

try:
    import tomllib
except ModuleNotFoundError:  # pragma: no cover - Python < 3.11 fallback.
    tomllib = None  # type: ignore[assignment]


SCRIPT_DIR = Path(__file__).resolve().parent
ROOT = SCRIPT_DIR.parent.parent
DEFAULT_SETTINGS = SCRIPT_DIR / "codrax_swebench.yaml"
GOLD_ARTIFACT_FIELDS = {
    "patch",
    "test_patch",
    "FAIL_TO_PASS",
    "PASS_TO_PASS",
    "fail_to_pass",
    "pass_to_pass",
}
PYTHON_COMPAT_CONSTRAINT_POLICIES = {
    "jinja2": {
        "constraint": "Jinja2<3.1",
        "reason_code": "legacy_jinja2_filter_api_without_major_ceiling",
    },
    "numpy": {
        "constraint": "numpy<2",
        "reason_code": "lower_bound_without_major_ceiling",
    },
}
SETUP_CFG_TEST_EXTRA_KEYS = {"test", "tests", "testing"}
CALLER_RETURN_ADAPTER_NAMES = {
    "array",
    "asarray",
    "flatten",
    "ravel",
    "reshape",
    "squeeze",
    "to_numpy",
    "tolist",
}
PATCH_REVIEW_LOCAL_BLOCKER_CODES = {
    "behavior_contract_without_verify_coverage",
    "changed_symbol_without_probe_coverage",
    "dependent_surface_without_verify_coverage",
    "related_test_surface_unverified",
}


@dataclass
class CommandResult:
    code: int
    output: str
    timed_out: bool = False


def safe_id(raw: str) -> str:
    text = re.sub(r"[^A-Za-z0-9_.-]+", "__", raw.strip())
    return text[:180] or "instance"


def run_cmd(
    cmd: list[str],
    *,
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
    timeout: int | None = None,
    check: bool = False,
) -> CommandResult:
    proc = subprocess.Popen(
        cmd,
        cwd=str(cwd) if cwd else None,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        start_new_session=True,
    )
    timed_out = False
    try:
        output, _ = proc.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        try:
            os.killpg(proc.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            output, _ = proc.communicate(timeout=10)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            output, _ = proc.communicate()
    result = CommandResult(proc.returncode or 0, output or "", timed_out)
    if check and (result.code != 0 or result.timed_out):
        printable = " ".join(cmd)
        suffix = " (timeout)" if result.timed_out else ""
        raise RuntimeError(f"command failed{suffix}: {printable}\n{result.output[-4000:]}")
    return result


def run_cmd_streaming_to_file(
    cmd: list[str],
    *,
    output_path: Path,
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
    timeout: int | None = None,
    progress_interval: int = 0,
    progress_fn: Any | None = None,
) -> CommandResult:
    """Run a long command while writing output and optional typed heartbeats.

    This is used for Codrax itself, not for short setup commands. Heartbeats are
    produced from typed durable state supplied by progress_fn; stdout text is
    kept as user-visible evidence but is not parsed for control.
    """

    output_path.parent.mkdir(parents=True, exist_ok=True)
    proc = subprocess.Popen(
        cmd,
        cwd=str(cwd) if cwd else None,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        start_new_session=True,
    )
    chunks: list[str] = []
    chunks_lock = threading.Lock()

    def reader() -> None:
        assert proc.stdout is not None
        with output_path.open("w", encoding="utf-8", errors="replace") as handle:
            for line in proc.stdout:
                with chunks_lock:
                    chunks.append(line)
                handle.write(line)
                handle.flush()

    thread = threading.Thread(target=reader, daemon=True)
    thread.start()

    started = time.monotonic()
    next_progress = started + max(progress_interval, 0) if progress_interval > 0 else float("inf")
    timed_out = False
    try:
        while proc.poll() is None:
            now = time.monotonic()
            if timeout is not None and now - started >= timeout:
                timed_out = True
                try:
                    os.killpg(proc.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                break
            if progress_interval > 0 and now >= next_progress:
                if progress_fn is not None:
                    try:
                        line = str(progress_fn(int(now - started)) or "").strip()
                    except Exception:  # pragma: no cover - defensive progress path.
                        line = (
                            f"  progress elapsed={int(now - started)}s "
                            "workflow=unavailable reason=progress_callback_unavailable"
                        )
                    if line:
                        print(line, file=sys.stderr, flush=True)
                next_progress = now + progress_interval
            time.sleep(0.25)
        if timed_out:
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(proc.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                proc.wait()
    finally:
        thread.join(timeout=5)
    with chunks_lock:
        output = "".join(chunks)
    return CommandResult(proc.returncode or 0, output, timed_out)


def load_jsonl(path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    with path.open(encoding="utf-8") as handle:
        for line_no, line in enumerate(handle, 1):
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
            except json.JSONDecodeError as exc:
                raise SystemExit(f"{path}:{line_no}: invalid JSON: {exc}") from exc
            if not isinstance(row, dict):
                raise SystemExit(f"{path}:{line_no}: each row must be an object")
            rows.append(row)
    return rows


def load_dataset_rows(dataset_name: str, split: str) -> list[dict[str, Any]]:
    try:
        from datasets import load_dataset  # type: ignore
    except Exception as exc:
        raise SystemExit(
            "Loading a Hugging Face SWE-bench dataset requires the optional "
            "'datasets' package. Install it or pass --instances-jsonl."
        ) from exc
    dataset = load_dataset(dataset_name, split=split)
    return [dict(row) for row in dataset]


def select_instances(rows: list[dict[str, Any]], instance_ids: set[str], limit: int) -> list[dict[str, Any]]:
    selected: list[dict[str, Any]] = []
    for row in rows:
        instance_id = str(row.get("instance_id") or "").strip()
        if not instance_id:
            continue
        if instance_ids and instance_id not in instance_ids:
            continue
        selected.append(row)
        if limit > 0 and len(selected) >= limit:
            break
    return selected


def read_instance_ids(values: list[str], file_path: Path | None) -> set[str]:
    out: set[str] = set()
    for value in values:
        for item in value.split(","):
            item = item.strip()
            if item:
                out.add(item)
    if file_path:
        with file_path.open(encoding="utf-8") as handle:
            for line in handle:
                item = line.strip()
                if item:
                    out.add(item)
    return out


def repo_url_for(instance: dict[str, Any], template: str) -> str:
    if instance.get("repo_url"):
        return str(instance["repo_url"])
    repo = str(instance.get("repo") or "").strip()
    if not repo:
        raise ValueError("instance missing repo")
    return template.format(repo=repo)


def required_field(instance: dict[str, Any], name: str) -> str:
    value = str(instance.get(name) or "").strip()
    if not value:
        raise ValueError(f"instance missing {name}")
    return value


def ensure_repo_cache(instance: dict[str, Any], args: argparse.Namespace) -> Path:
    repo = required_field(instance, "repo")
    cache_dir = Path(args.repo_cache).resolve()
    cache_dir.mkdir(parents=True, exist_ok=True)
    mirror = cache_dir / f"{safe_id(repo)}.git"
    repo_url = repo_url_for(instance, args.repo_url_template)
    if not mirror.exists():
        result = run_cmd(["git", "clone", "--mirror", repo_url, str(mirror)], timeout=args.git_timeout)
        if result.code != 0:
            raise RuntimeError(f"git clone --mirror failed for {repo_url}\n{result.output[-4000:]}")
    elif not args.no_fetch:
        result = run_cmd(["git", "-C", str(mirror), "fetch", "--prune"], timeout=args.git_timeout)
        if result.code != 0:
            base_commit = str(instance.get("base_commit") or "").strip()
            if base_commit and mirror_has_revision(mirror, base_commit, args.git_timeout):
                print(
                    f"warning: git fetch failed for {mirror}; using cached mirror because base_commit {base_commit} is present",
                    file=sys.stderr,
                )
            else:
                raise RuntimeError(f"git fetch failed for {mirror}\n{result.output[-4000:]}")
    return mirror


def mirror_has_revision(mirror: Path, revision: str, timeout: int) -> bool:
    if not revision:
        return False
    result = run_cmd(["git", "-C", str(mirror), "cat-file", "-e", f"{revision}^{{commit}}"], timeout=timeout)
    return result.code == 0


def isolate_git_history(repo_dir: Path, args: argparse.Namespace) -> dict[str, Any]:
    """Prune refs/history not reachable from the checked-out base commit.

    Fair SWE-bench runs should not expose future fix commits through `git show`
    or shell git history. The adapter keeps HEAD at the original base commit
    (so patches remain comparable) but deletes branch/tag refs, expires reflogs,
    and prunes unreachable objects inside the per-instance clone.
    """

    head = run_cmd(["git", "rev-parse", "HEAD"], cwd=repo_dir, timeout=args.git_timeout, check=True).output.strip()
    refs_result = run_cmd(["git", "for-each-ref", "--format=%(refname)"], cwd=repo_dir, timeout=args.git_timeout, check=True)
    refs = [line.strip() for line in refs_result.output.splitlines() if line.strip()]
    deleted_refs: list[str] = []
    for ref in refs:
        result = run_cmd(["git", "update-ref", "-d", ref], cwd=repo_dir, timeout=args.git_timeout)
        if result.code != 0:
            raise RuntimeError(f"git update-ref -d {ref} failed\n{result.output[-4000:]}")
        deleted_refs.append(ref)
    for cmd in (
        ["git", "reflog", "expire", "--expire=now", "--expire-unreachable=now", "--all"],
        ["git", "gc", "--prune=now", "--quiet"],
    ):
        result = run_cmd(cmd, cwd=repo_dir, timeout=args.git_timeout)
        if result.code != 0:
            raise RuntimeError(f"{' '.join(cmd)} failed\n{result.output[-4000:]}")
    after = run_cmd(["git", "rev-parse", "HEAD"], cwd=repo_dir, timeout=args.git_timeout, check=True).output.strip()
    if after != head:
        raise RuntimeError(f"git history isolation changed HEAD: before={head} after={after}")
    refs_after = run_cmd(["git", "for-each-ref", "--format=%(refname)"], cwd=repo_dir, timeout=args.git_timeout, check=True)
    remaining_refs = [line.strip() for line in refs_after.output.splitlines() if line.strip()]
    if remaining_refs:
        raise RuntimeError(f"git history isolation left refs behind: {remaining_refs[:20]}")
    return {
        "enabled": True,
        "head": head,
        "deleted_ref_count": len(deleted_refs),
        "deleted_refs_preview": deleted_refs[:20],
    }


def checkout_instance(instance: dict[str, Any], mirror: Path, repo_dir: Path, args: argparse.Namespace) -> tuple[str, dict[str, Any]]:
    base_commit = required_field(instance, "base_commit")
    if repo_dir.exists():
        shutil.rmtree(repo_dir)
    repo_dir.parent.mkdir(parents=True, exist_ok=True)
    result = run_cmd(["git", "clone", str(mirror), str(repo_dir)], timeout=args.git_timeout)
    if result.code != 0:
        raise RuntimeError(f"git clone from cache failed\n{result.output[-4000:]}")
    for cmd in (
        ["git", "checkout", "--detach", base_commit],
        ["git", "reset", "--hard", base_commit],
        ["git", "clean", "-fdx"],
        ["git", "config", "user.email", "swebench@codrax.local"],
        ["git", "config", "user.name", "Codrax SWE-bench"],
    ):
        result = run_cmd(cmd, cwd=repo_dir, timeout=args.git_timeout)
        if result.code != 0:
            raise RuntimeError(f"{' '.join(cmd)} failed\n{result.output[-4000:]}")
    resolved = run_cmd(["git", "rev-parse", "HEAD"], cwd=repo_dir, timeout=args.git_timeout, check=True)
    head = resolved.output.strip()
    isolation = {"enabled": False}
    if args.isolate_git_history:
        isolation = isolate_git_history(repo_dir, args)
        head = str(isolation.get("head") or head)
    return head, isolation


def is_python_project(repo_dir: Path) -> bool:
    markers = (
        "pyproject.toml",
        "setup.py",
        "setup.cfg",
        "requirements.txt",
        "tox.ini",
        "pytest.ini",
    )
    if any((repo_dir / marker).exists() for marker in markers):
        return True
    return any(repo_dir.glob("**/*.py"))


def pyproject_build_requires(repo_dir: Path) -> list[str]:
    pyproject = repo_dir / "pyproject.toml"
    if tomllib is None or not pyproject.exists() or pyproject.stat().st_size > 1_000_000:
        return []
    try:
        data = tomllib.loads(pyproject.read_text(encoding="utf-8"))
    except Exception:
        return []
    build_system = data.get("build-system")
    if not isinstance(build_system, dict):
        return []
    requires = build_system.get("requires")
    if not isinstance(requires, list):
        return []
    out: list[str] = []
    for item in requires:
        if not isinstance(item, str):
            continue
        value = item.strip()
        if value:
            out.append(value)
    return out[:64]


def setup_py_declared_requires(repo_dir: Path) -> list[str]:
    setup_py = repo_dir / "setup.py"
    if not setup_py.exists() or setup_py.stat().st_size > 2_000_000:
        return []
    try:
        tree = ast.parse(setup_py.read_text(encoding="utf-8"), filename=str(setup_py))
    except Exception:
        return []

    names: dict[str, Any] = {}

    def eval_expr(node: ast.AST) -> Any:
        if isinstance(node, ast.Constant):
            return node.value
        if isinstance(node, ast.Name):
            return names.get(node.id)
        if isinstance(node, (ast.List, ast.Tuple, ast.Set)):
            values = []
            for item in node.elts:
                value = eval_expr(item)
                if value is None:
                    return None
                values.append(value)
            return values
        if isinstance(node, ast.Dict):
            out: dict[Any, Any] = {}
            for key_node, value_node in zip(node.keys, node.values):
                if key_node is None:
                    continue
                key = eval_expr(key_node)
                value = eval_expr(value_node)
                if key is not None:
                    out[key] = value
            return out
        if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Add):
            left = eval_expr(node.left)
            right = eval_expr(node.right)
            if isinstance(left, str) and isinstance(right, str):
                return left + right
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute) and node.func.attr == "format":
            base = eval_expr(node.func.value)
            args = [eval_expr(arg) for arg in node.args]
            kwargs = {kw.arg: eval_expr(kw.value) for kw in node.keywords if kw.arg}
            if isinstance(base, str) and all(arg is not None for arg in args) and all(value is not None for value in kwargs.values()):
                try:
                    return base.format(*args, **kwargs)
                except Exception:
                    return None
        return None

    def add_requires(value: Any, out: list[str]) -> None:
        if isinstance(value, str):
            item = value.strip()
            if item:
                out.append(item)
            return
        if isinstance(value, (list, tuple, set)):
            for item in value:
                add_requires(item, out)

    for stmt in tree.body:
        if isinstance(stmt, ast.Assign) and len(stmt.targets) == 1 and isinstance(stmt.targets[0], ast.Name):
            value = eval_expr(stmt.value)
            if isinstance(value, (str, int, float, list, tuple, set, dict)):
                names[stmt.targets[0].id] = value

    raw: list[str] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Dict):
            values = eval_expr(node)
            if isinstance(values, dict):
                add_requires(values.get("setup_requires"), raw)
                add_requires(values.get("install_requires"), raw)
        elif isinstance(node, ast.Call):
            for kw in node.keywords:
                if kw.arg in {"setup_requires", "install_requires"}:
                    add_requires(eval_expr(kw.value), raw)

    out: list[str] = []
    seen: set[str] = set()
    for item in raw:
        if item not in seen:
            out.append(item)
            seen.add(item)
    return out[:64]


def setup_cfg_declared_requires(repo_dir: Path) -> list[str]:
    setup_cfg = repo_dir / "setup.cfg"
    if not setup_cfg.exists() or setup_cfg.stat().st_size > 2_000_000:
        return []
    parser = configparser.ConfigParser(interpolation=None)
    try:
        parser.read(setup_cfg, encoding="utf-8")
    except Exception:
        return []
    raw: list[str] = []
    if parser.has_section("options"):
        for key in ("install_requires", "setup_requires"):
            if parser.has_option("options", key):
                raw.extend(parser.get("options", key).splitlines())
    out: list[str] = []
    seen: set[str] = set()
    for item in raw:
        value = item.split("#", 1)[0].strip()
        if not value or value.startswith("#"):
            continue
        if value not in seen:
            out.append(value)
            seen.add(value)
    return out[:64]


def setup_cfg_test_requires(repo_dir: Path) -> list[str]:
    setup_cfg = repo_dir / "setup.cfg"
    if not setup_cfg.exists() or setup_cfg.stat().st_size > 2_000_000:
        return []
    parser = configparser.ConfigParser(interpolation=None)
    try:
        parser.read(setup_cfg, encoding="utf-8")
    except Exception:
        return []
    raw: list[str] = []
    if parser.has_section("options") and parser.has_option("options", "tests_require"):
        raw.extend(parser.get("options", "tests_require").splitlines())
    if parser.has_section("options.extras_require"):
        for key, value in parser.items("options.extras_require"):
            if key.strip().lower() in SETUP_CFG_TEST_EXTRA_KEYS:
                raw.extend(value.splitlines())
    out: list[str] = []
    seen: set[str] = set()
    for item in raw:
        value = item.split("#", 1)[0].strip()
        if not value or value in seen:
            continue
        out.append(value)
        seen.add(value)
    return out[:64]


def setup_declared_requires(repo_dir: Path) -> list[str]:
    raw: list[str] = []
    raw.extend(setup_py_declared_requires(repo_dir))
    raw.extend(setup_cfg_declared_requires(repo_dir))
    out: list[str] = []
    seen: set[str] = set()
    for item in raw:
        value = item.strip()
        if not value or value in seen:
            continue
        out.append(value)
        seen.add(value)
    return out[:64]


def setup_test_requires(repo_dir: Path) -> list[str]:
    return setup_cfg_test_requires(repo_dir)


def requirement_lines_from_file(path: Path) -> list[str]:
    if not path.is_file():
        return []
    try:
        if path.stat().st_size > 1_000_000:
            return []
        text = path.read_text(encoding="utf-8")
    except Exception:
        return []
    out: list[str] = []
    for raw in text.splitlines():
        line = raw.split("#", 1)[0].strip()
        if not line or line.startswith(("-", "http://", "https://", ".")):
            continue
        out.append(line)
    return out[:256]


def requirement_package_name(requirement: str) -> str:
    text = requirement.strip()
    if not text:
        return ""
    match = re.match(r"^([A-Za-z0-9_.-]+)", text)
    if not match:
        return ""
    return match.group(1).replace("_", "-").lower()


def requirement_has_major_ceiling(requirement: str) -> bool:
    text = requirement.split(";", 1)[0].strip()
    if not text:
        return False
    return any(op in text for op in ("<", "~=", "===", "=="))


def declared_python_requirements(repo_dir: Path, requirement_files: list[dict[str, str]]) -> list[str]:
    raw: list[str] = []
    raw.extend(setup_declared_requires(repo_dir))
    for item in requirement_files:
        rel = item.get("path") or ""
        if rel:
            raw.extend(requirement_lines_from_file(repo_dir / rel))
    out: list[str] = []
    seen: set[str] = set()
    for item in raw:
        value = item.strip()
        if value and value not in seen:
            out.append(value)
            seen.add(value)
    return out[:512]


def python_compat_constraints(requirements: list[str]) -> list[dict[str, str]]:
    by_name: dict[str, list[str]] = {}
    for req in requirements:
        name = requirement_package_name(req)
        if name:
            by_name.setdefault(name, []).append(req)
    out: list[dict[str, str]] = []
    for package, policy in sorted(PYTHON_COMPAT_CONSTRAINT_POLICIES.items()):
        package_reqs = by_name.get(package, [])
        if not package_reqs:
            continue
        if any(requirement_has_major_ceiling(req) for req in package_reqs):
            continue
        out.append(
            {
                "package": package,
                "constraint": str(policy["constraint"]),
                "reason_code": str(policy["reason_code"]),
            }
        )
    return out


def discover_python_requirement_files(repo_dir: Path) -> list[dict[str, str]]:
    """Return runtime/test requirement files worth installing best-effort.

    The adapter is not a full environment solver. It recognizes common Python
    project file layouts and avoids heavyweight docs-only/dev files unless no
    test-focused requirements are present.
    """

    max_size = 1_000_000
    runtime = (
        "requirements.txt",
        "requirements/base.txt",
        "requirements/common.txt",
    )
    tests = (
        "test-requirements.txt",
        "tests-requirements.txt",
        "requirements-test.txt",
        "requirements-tests.txt",
        "requirements/test.txt",
        "requirements/tests.txt",
        "requirements/ci.txt",
        "tests/requirements.txt",
    )
    dev = (
        "dev-requirements.txt",
        "requirements-dev.txt",
        "requirements/dev.txt",
    )

    def add(out: list[dict[str, str]], seen: set[Path], kind: str, rel: str) -> None:
        path = repo_dir / rel
        try:
            resolved = path.resolve()
            rel_path = path.relative_to(repo_dir).as_posix()
        except Exception:
            return
        if resolved in seen or not path.is_file():
            return
        try:
            if path.stat().st_size > max_size:
                return
        except OSError:
            return
        out.append({"kind": kind, "path": rel_path})
        seen.add(resolved)

    out: list[dict[str, str]] = []
    seen: set[Path] = set()
    for rel in runtime:
        add(out, seen, "runtime", rel)
    before_tests = len(out)
    for rel in tests:
        add(out, seen, "test", rel)
    found_test = len(out) > before_tests
    if not found_test:
        for rel in dev:
            add(out, seen, "dev_fallback", rel)
    return out[:16]


def discover_python_constraint_files(repo_dir: Path) -> list[str]:
    candidates = (
        "constraints.txt",
        "requirements/constraints.txt",
        "requirements/constraints-test.txt",
        "requirements/test-constraints.txt",
    )
    out: list[str] = []
    seen: set[Path] = set()
    for rel in candidates:
        path = repo_dir / rel
        try:
            resolved = path.resolve()
            rel_path = path.relative_to(repo_dir).as_posix()
        except Exception:
            continue
        if resolved in seen or not path.is_file():
            continue
        try:
            if path.stat().st_size > 1_000_000:
                continue
        except OSError:
            continue
        out.append(rel_path)
        seen.add(resolved)
    return out[:8]


def discover_python_import_roots(repo_dir: Path) -> list[str]:
    out: list[str] = []
    seen: set[str] = set()
    ignored = {"test", "tests", "docs", "doc", "example", "examples", "build", "dist"}
    for base in (repo_dir, repo_dir / "src", repo_dir / "lib"):
        if not base.is_dir():
            continue
        try:
            entries = sorted(base.iterdir(), key=lambda item: item.name)
        except OSError:
            continue
        for entry in entries:
            name = entry.name
            if name.startswith(".") or name.startswith("_") or name in ignored:
                continue
            if entry.is_dir() and (entry / "__init__.py").is_file():
                import_name = name.replace("-", "_")
            elif entry.is_file() and entry.suffix == ".py" and name != "setup.py":
                import_name = entry.stem.replace("-", "_")
            else:
                continue
            if import_name.isidentifier() and import_name not in seen:
                out.append(import_name)
                seen.add(import_name)
    return out[:12]


def discover_python_source_roots(repo_dir: Path) -> list[Path]:
    roots: list[Path] = []
    seen: set[str] = set()
    ignored = {"test", "tests", "docs", "doc", "example", "examples", "build", "dist"}
    for base in (repo_dir / "src", repo_dir / "lib", repo_dir):
        if not base.is_dir():
            continue
        try:
            entries = sorted(base.iterdir(), key=lambda item: item.name)
        except OSError:
            continue
        has_importable = False
        for entry in entries:
            name = entry.name
            if name.startswith(".") or name in ignored:
                continue
            if entry.is_dir() and (entry / "__init__.py").is_file():
                has_importable = True
                break
            if entry.is_file() and entry.suffix == ".py" and name != "setup.py":
                has_importable = True
                break
        if not has_importable:
            continue
        key = str(base.resolve())
        if key in seen:
            continue
        seen.add(key)
        roots.append(base)
    return roots


def prepend_pythonpath(env: dict[str, str], roots: list[Path]) -> str:
    out: list[str] = []
    seen: set[str] = set()

    def add(value: str) -> None:
        value = value.strip()
        if not value or value in seen:
            return
        seen.add(value)
        out.append(value)

    for root in roots:
        add(str(root))
    for item in env.get("PYTHONPATH", "").split(os.pathsep):
        add(item)
    return os.pathsep.join(out)


def venv_bin_dir(venv_dir: Path) -> Path:
    if os.name == "nt":
        return venv_dir / "Scripts"
    return venv_dir / "bin"


def env_prepare_failure_kind(status: str) -> str:
    if status.startswith("failed_"):
        return status.removeprefix("failed_")
    if status == "partial":
        return "project_setup_partial"
    return ""


def finalize_env_prepare_record(
    record: dict[str, Any],
    *,
    status: str | None = None,
    venv_dir: Path | None = None,
    python: Path | None = None,
) -> dict[str, Any]:
    """Populate stable, typed telemetry fields for SWE-bench env setup.

    These fields are intentionally observational. Prediction export and the
    official harness path must keep moving even when the local verification
    sandbox is partial or unavailable.
    """

    if status:
        record["status"] = status
    status = str(record.get("status") or "skipped")
    if venv_dir is not None:
        record["env_path"] = str(venv_dir)
        record["venv_python"] = str(python or (venv_bin_dir(venv_dir) / "python"))
    elif record.get("env_path") and not record.get("venv_python"):
        record["venv_python"] = str(venv_bin_dir(Path(str(record["env_path"]))) / "python")

    commands = [cmd for cmd in record.get("commands", []) if isinstance(cmd, dict)]
    failed_steps = [
        str(cmd.get("name") or "")
        for cmd in commands
        if str(cmd.get("name") or "") and (bool(cmd.get("timed_out")) or int(cmd.get("code") or 0) != 0)
    ]
    record["failed_step_names"] = failed_steps
    record["failure_kind"] = env_prepare_failure_kind(status)
    record["success"] = status == "ready"
    record["env_available"] = bool(record.get("env_path"))
    record["pytest_available"] = any(
        cmd.get("name") == "install_pytest" and int(cmd.get("code") or 0) == 0 and not bool(cmd.get("timed_out"))
        for cmd in commands
    )
    record["pytest_json_report_available"] = bool(record["pytest_available"])
    roots = [str(item) for item in record.get("python_import_roots") or [] if str(item)]
    record["import_roots"] = roots
    source_roots = [str(item) for item in record.get("python_source_roots") or [] if str(item)]
    record["source_roots"] = source_roots
    record["import_probe_ok"] = record.get("import_probe_passed") if "import_probe_passed" in record else None
    record["hard_gate"] = False
    return record


def prepare_python_env(repo_dir: Path, inst_dir: Path, args: argparse.Namespace) -> tuple[dict[str, str], dict[str, Any]]:
    """Best-effort Python verification environment for local Codrax runs.

    The official SWE-bench harness remains the scoring authority. This helper
    only makes Codrax's own verify stage more useful during smoke/eval runs.
    Failures are recorded and the adapter continues with the host environment.
    """

    record: dict[str, Any] = {
        "enabled": bool(args.prepare_python_env),
        "status": "skipped",
        "commands": [],
    }
    if not args.prepare_python_env:
        return {}, finalize_env_prepare_record(record, status="skipped")
    if not is_python_project(repo_dir):
        return {}, finalize_env_prepare_record(record, status="skipped_non_python")

    venv_dir = inst_dir / "python-env"
    if venv_dir.exists():
        shutil.rmtree(venv_dir)
    py_timeout = int(args.env_prepare_timeout)

    def step(name: str, cmd: list[str], *, cwd: Path | None = None, env: dict[str, str] | None = None) -> CommandResult:
        result = run_cmd(cmd, cwd=cwd, env=env, timeout=py_timeout)
        record["commands"].append(
            {
                "name": name,
                "cmd": cmd,
                "code": result.code,
                "timed_out": result.timed_out,
                "output_tail": result.output[-4000:],
            }
        )
        return result

    def ensure_legacy_pkg_resources(label: str) -> bool:
        check = step(label + "_check", [str(python), "-c", "import pkg_resources"])
        if check.code == 0 and not check.timed_out:
            return True
        compat = step(label + "_install_setuptools_compat", [str(python), "-m", "pip", "install", "setuptools<81"])
        if compat.code != 0 or compat.timed_out:
            return False
        recheck = step(label + "_recheck", [str(python), "-c", "import pkg_resources"])
        return recheck.code == 0 and not recheck.timed_out

    def ensure_legacy_setuptools_dep_util(label: str) -> bool:
        check = step(label + "_check", [str(python), "-c", "from setuptools.dep_util import newer_group"])
        if check.code == 0 and not check.timed_out:
            return True
        compat = step(
            label + "_install_setuptools_compat",
            [str(python), "-m", "pip", "install", "setuptools>=64,<66"],
        )
        if compat.code != 0 or compat.timed_out:
            return False
        recheck = step(label + "_recheck", [str(python), "-c", "from setuptools.dep_util import newer_group"])
        return recheck.code == 0 and not recheck.timed_out

    created = step("create_venv", [sys.executable, "-m", "venv", str(venv_dir)])
    if created.code != 0 or created.timed_out:
        return {}, finalize_env_prepare_record(record, status="failed_create_venv", venv_dir=venv_dir)

    python = venv_bin_dir(venv_dir) / "python"
    if not python.exists():
        return {}, finalize_env_prepare_record(record, status="failed_missing_python", venv_dir=venv_dir, python=python)

    env_updates = {
        "VIRTUAL_ENV": str(venv_dir),
        "PATH": f"{venv_bin_dir(venv_dir)}{os.pathsep}{os.environ.get('PATH', '')}",
    }
    source_roots = discover_python_source_roots(repo_dir)
    if source_roots:
        record["python_source_roots"] = [str(root.relative_to(repo_dir)) for root in source_roots]
        env_base = os.environ.copy()
        env_base.update(env_updates)
        env_updates["PYTHONPATH"] = prepend_pythonpath(env_base, source_roots)
        record["pythonpath"] = env_updates["PYTHONPATH"]
    constraint_files = discover_python_constraint_files(repo_dir)
    requirement_files = discover_python_requirement_files(repo_dir)
    declared_requires = declared_python_requirements(repo_dir, requirement_files)
    if declared_requires:
        record["python_declared_requires"] = declared_requires
    compat_constraint_path: Path | None = None
    compat_constraints: list[dict[str, str]] = []
    if not bool(args.disable_python_compat_constraints):
        compat_constraints = python_compat_constraints(declared_requires)
        if compat_constraints:
            compat_constraint_path = inst_dir / "python-compat-constraints.txt"
            compat_constraint_path.write_text(
                "\n".join(item["constraint"] for item in compat_constraints) + "\n",
                encoding="utf-8",
            )
            record["python_compat_constraints"] = compat_constraints
            record["python_compat_constraints_path"] = str(compat_constraint_path)
    if constraint_files:
        record["python_constraint_files"] = constraint_files
    constraint_paths = [repo_dir / rel for rel in constraint_files]
    if compat_constraint_path is not None:
        constraint_paths.append(compat_constraint_path)

    def pip_install_cmd(*items: str) -> list[str]:
        cmd = [str(python), "-m", "pip", "install"]
        for path in constraint_paths:
            cmd.extend(["-c", str(path)])
        cmd.extend(items)
        return cmd

    upgraded = step("upgrade_packaging", [str(python), "-m", "pip", "install", "--upgrade", "pip", "setuptools", "wheel"])
    if upgraded.code != 0 or upgraded.timed_out:
        return env_updates, finalize_env_prepare_record(record, status="failed_packaging", venv_dir=venv_dir, python=python)

    pytest = step("install_pytest", pip_install_cmd("pytest<9", "pytest-json-report"))
    if pytest.code != 0 or pytest.timed_out:
        return env_updates, finalize_env_prepare_record(record, status="failed_pytest", venv_dir=venv_dir, python=python)
    record["pkg_resources_available"] = ensure_legacy_pkg_resources("legacy_pkg_resources")

    project_failed = False
    build_requires = pyproject_build_requires(repo_dir)
    if build_requires:
        record["pyproject_build_requires"] = build_requires
        build_req = step("install_pyproject_build_requires", pip_install_cmd(*build_requires), cwd=repo_dir)
        project_failed = project_failed or build_req.code != 0 or build_req.timed_out
        if not ensure_legacy_pkg_resources("legacy_pkg_resources_post_build_requires"):
            record["pkg_resources_available"] = False
            project_failed = True
        else:
            record["pkg_resources_available"] = True
    if requirement_files:
        record["python_requirement_files"] = requirement_files
        for index, item in enumerate(requirement_files, 1):
            kind = item.get("kind") or "requirements"
            rel = item.get("path") or ""
            if not rel:
                continue
            req = step(
                f"install_{kind}_requirements_{index}",
                pip_install_cmd("-r", str(repo_dir / rel)),
                cwd=repo_dir,
            )
            project_failed = project_failed or req.code != 0 or req.timed_out
    setup_requires = setup_declared_requires(repo_dir)
    if setup_requires:
        record["setup_declared_requires"] = setup_requires
        setup_req = step("install_setup_declared_requires", pip_install_cmd(*setup_requires), cwd=repo_dir)
        project_failed = project_failed or setup_req.code != 0 or setup_req.timed_out
        if not ensure_legacy_pkg_resources("legacy_pkg_resources_post_setup_requires"):
            record["pkg_resources_available"] = False
            project_failed = True
        else:
            record["pkg_resources_available"] = True
    test_requires = setup_test_requires(repo_dir)
    if test_requires:
        record["setup_test_requires"] = test_requires
        test_req = step("install_setup_test_requires", pip_install_cmd(*test_requires), cwd=repo_dir)
        project_failed = project_failed or test_req.code != 0 or test_req.timed_out
    if (repo_dir / "pyproject.toml").exists() or (repo_dir / "setup.py").exists() or (repo_dir / "setup.cfg").exists():
        project_install_failed = False
        editable = step("install_editable", pip_install_cmd("-e", "."), cwd=repo_dir)
        if editable.code != 0 and record.get("pkg_resources_available"):
            # Legacy projects often import pkg_resources from setup.py, but
            # pip's isolated build env can ignore the venv setuptools pin. A
            # bounded no-isolation retry lets the already-prepared compat
            # environment participate without making env setup a hard gate.
            editable = step(
                "install_editable_no_build_isolation",
                pip_install_cmd("--no-build-isolation", "-e", "."),
                cwd=repo_dir,
            )
        project_install_failed = editable.code != 0 or editable.timed_out
        if project_install_failed and (repo_dir / "setup.py").exists():
            if ensure_legacy_setuptools_dep_util("legacy_setuptools_dep_util"):
                record["legacy_setuptools_dep_util_available"] = True
                build_ext = step("build_ext_inplace", [str(python), "setup.py", "build_ext", "--inplace"], cwd=repo_dir)
                project_install_failed = build_ext.code != 0 or build_ext.timed_out
            else:
                record["legacy_setuptools_dep_util_available"] = False
        project_failed = project_failed or project_install_failed
    if not ensure_legacy_pkg_resources("legacy_pkg_resources_post_project"):
        record["pkg_resources_available"] = False
        project_failed = True
    else:
        record["pkg_resources_available"] = True
    import_roots = discover_python_import_roots(repo_dir)
    if import_roots:
        record["python_import_roots"] = import_roots
        probe_code = (
            "import importlib, sys\n"
            "failed=[]\n"
            "for name in sys.argv[1:]:\n"
            "    try:\n"
            "        importlib.import_module(name)\n"
            "    except Exception as exc:\n"
            "        failed.append(f'{name}: {type(exc).__name__}: {exc}')\n"
            "if failed:\n"
            "    print('\\n'.join(failed))\n"
            "    sys.exit(1)\n"
            "print('ok')\n"
        )
        probe_env = os.environ.copy()
        probe_env.update(env_updates)
        probe = step("import_probe", [str(python), "-c", probe_code, *import_roots], cwd=repo_dir, env=probe_env)
        record["import_probe_passed"] = probe.code == 0 and not probe.timed_out
        project_failed = project_failed or probe.code != 0 or probe.timed_out

    status = "partial" if project_failed else "ready"
    return env_updates, finalize_env_prepare_record(record, status=status, venv_dir=venv_dir, python=python)


def build_request(instance: dict[str, Any], args: argparse.Namespace) -> str:
    instance_id = required_field(instance, "instance_id")
    problem = required_field(instance, "problem_statement")
    prefix = args.request_prefix.strip()
    if prefix:
        return f"{prefix}\n\nSWE-bench instance: {instance_id}\n\n{problem}".strip()
    return (
        f"SWE-bench instance: {instance_id}\n\n"
        f"{problem}\n\n"
        "Fix the repository behavior described above."
    )


def latest_workflow_progress(workflow: dict[str, Any]) -> str:
    latest = latest_workflow_progress_event(workflow)
    if not latest:
        return ""
    parts = []
    for key in ("stage", "status", "reason_code"):
        value = str(latest.get(key) or "").strip()
        if value:
            parts.append(value)
    return "/".join(parts)


def latest_workflow_progress_event(workflow: dict[str, Any]) -> dict[str, Any]:
    progress = [row for row in workflow.get("progress_ledger") or [] if isinstance(row, dict)]
    if not progress:
        return {}
    return progress[-1]


def active_workflow_batch_summary(workflow: dict[str, Any]) -> str:
    active_id = str(workflow.get("active_batch_id") or "").strip()
    if not active_id:
        return ""
    for batch in workflow.get("batches") or []:
        if not isinstance(batch, dict):
            continue
        if str(batch.get("id") or "").strip() != active_id:
            continue
        status = str(batch.get("status") or "").strip()
        slice_id = str(batch.get("active_slice_id") or "").strip()
        if slice_id:
            return f"{active_id}:{status}:slice={slice_id}"
        return f"{active_id}:{status}"
    return active_id


def codrax_progress_status(repo_dir: Path, instance_id: str, elapsed_seconds: int) -> str:
    prefix = f"  progress {instance_id} elapsed={elapsed_seconds}s"
    try:
        workflow, _ = load_latest_workflow_run(repo_dir, repo_dir)
        if not workflow:
            return prefix + " workflow=pending"
        run_status = str(workflow.get("status") or "").strip() or "unknown"
        batch = active_workflow_batch_summary(workflow)
        latest = latest_workflow_progress(workflow)
        parts = [prefix, f"workflow={run_status}"]
        if batch:
            parts.append(f"batch={batch}")
        if latest:
            parts.append(f"last={latest}")
        return " ".join(parts)
    except Exception:
        return prefix + " workflow=unavailable reason=workflow_progress_unavailable"


def run_codrax(
    instance: dict[str, Any],
    repo_dir: Path,
    inst_dir: Path,
    args: argparse.Namespace,
    env_updates: dict[str, str] | None = None,
) -> CommandResult:
    log_dir = inst_dir / "logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    if env_updates:
        env.update(env_updates)
    if args.settings:
        env["CODRAX_SETTINGS"] = str(Path(args.settings).resolve())
    cmd = [
        str(Path(args.codrax_bin).resolve()),
        "--mode=write",
        "--repo",
        str(repo_dir),
        "--pipeline-max-steps",
        str(args.max_steps),
        "--log-level",
        args.log_level,
        "--log-dir",
        str(log_dir),
    ]
    if args.isolate_git_history:
        cmd.append("--eval-disable-git-history")
    cmd.extend(["--request", build_request(instance, args)])
    if args.providers:
        cmd[1:1] = ["--providers", str(Path(args.providers).resolve())]
    instance_id = required_field(instance, "instance_id")
    result = run_cmd_streaming_to_file(
        cmd,
        cwd=repo_dir,
        env=env,
        timeout=args.codrax_timeout,
        output_path=inst_dir / "codrax.out",
        progress_interval=max(0, int(args.codrax_progress_interval or 0)),
        progress_fn=lambda elapsed: codrax_progress_status(repo_dir, instance_id, elapsed),
    )
    (inst_dir / "codrax.rc").write_text(f"{result.code}\ntimeout={result.timed_out}\n", encoding="utf-8")
    return result


def find_latest_change_plan(*roots: Path) -> Path | None:
    candidates: list[tuple[float, Path]] = []
    for root in roots:
        if not root.exists():
            continue
        for path in root.rglob("*.json"):
            if path.name.endswith(".report.json") or "workflows" in path.parts:
                continue
            try:
                row = json.loads(path.read_text(encoding="utf-8"))
            except Exception:
                continue
            if isinstance(row, dict) and row.get("id") and row.get("summary") and isinstance(row.get("changes"), list):
                candidates.append((path.stat().st_mtime, path))
    if not candidates:
        return None
    candidates.sort(key=lambda item: item[0])
    return candidates[-1][1]


def load_plan(path: Path | None) -> dict[str, Any]:
    if not path:
        return {}
    try:
        row = json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return {}
    return row if isinstance(row, dict) else {}


def load_plan_by_id(repo_dir: Path, inst_dir: Path, plan_id: str) -> dict[str, Any]:
    path = find_plan_path_by_id(repo_dir, inst_dir, plan_id)
    return load_plan(path)


def find_plan_path_by_id(repo_dir: Path, inst_dir: Path, plan_id: str) -> Path | None:
    plan_id = str(plan_id or "").strip()
    if not plan_id or "/" in plan_id or "\\" in plan_id:
        return None
    for root in (
        repo_dir / ".codrax" / "plans",
        inst_dir / ".codrax" / "plans",
    ):
        path = root / f"{plan_id}.json"
        plan = load_plan(path)
        if str(plan.get("id") or "").strip() == plan_id:
            return path
    return None


def load_report_for_plan(plan_path: Path | None) -> dict[str, Any]:
    if not plan_path:
        return {}
    report_path = plan_path.with_name(plan_path.stem + ".report.json")
    if not report_path.exists():
        return {}
    try:
        row = json.loads(report_path.read_text(encoding="utf-8"))
    except Exception:
        return {}
    return row if isinstance(row, dict) else {}


def plan_change_paths(plan: dict[str, Any]) -> list[str]:
    out: list[str] = []
    for change in plan.get("changes") or []:
        if not isinstance(change, dict):
            continue
        path = str(change.get("path") or "").strip()
        if path:
            out.append(path)
    return out


def plan_source_paths(plan: dict[str, Any]) -> list[str]:
    return [
        path
        for path in plan_change_paths(plan)
        if path and not is_test_patch_path(path)
    ]


def plan_test_paths(plan: dict[str, Any]) -> list[str]:
    return [
        path
        for path in plan_change_paths(plan)
        if path and is_test_patch_path(path)
    ]


def workflow_applied_plan_ids(workflow: dict[str, Any]) -> list[str]:
    ids: list[str] = []
    seen: set[str] = set()
    for batch in workflow.get("batches") or []:
        if not isinstance(batch, dict):
            continue
        for attempt in batch.get("attempts") or []:
            if not isinstance(attempt, dict):
                continue
            if str(attempt.get("kind") or "").strip() != "apply":
                continue
            if str(attempt.get("status") or "").strip() != "applied":
                continue
            plan_id = str(attempt.get("plan_id") or "").strip()
            if not plan_id or plan_id in seen:
                continue
            seen.add(plan_id)
            ids.append(plan_id)
    return ids


def summarize_workflow_applied_plan_provenance(
    workflow: dict[str, Any],
    repo_dir: Path,
    inst_dir: Path,
    final_plan_id: str = "",
) -> dict[str, Any]:
    """Return typed provenance for source/test changes that actually applied.

    The SWE-bench adapter exports the cumulative worktree patch, while the
    latest durable ChangePlan may be a replan that only changed tests. These
    fields explain that drift by reading workflow attempt records and plan JSON
    artifacts. No stdout, prompt text, or model prose is inspected.
    """

    plan_ids = workflow_applied_plan_ids(workflow)
    source_paths: set[str] = set()
    test_paths: set[str] = set()
    for plan_id in plan_ids:
        plan = load_plan_by_id(repo_dir, inst_dir, plan_id)
        for path in plan_change_paths(plan):
            normalized = normalize_repo_rel_file_path(path)
            if not normalized:
                continue
            if is_test_patch_path(normalized):
                test_paths.add(normalized)
            else:
                source_paths.add(normalized)
    latest = plan_ids[-1] if plan_ids else ""
    final_plan_id = str(final_plan_id or "").strip()
    return {
        "workflow_applied_plan_ids": plan_ids,
        "workflow_latest_applied_plan_id": latest,
        "workflow_final_plan_is_latest_applied": bool(final_plan_id and latest and final_plan_id == latest),
        "workflow_applied_source_paths": sorted(source_paths),
        "workflow_applied_test_paths": sorted(test_paths),
    }


def workflow_applied_plan_summaries(
    workflow: dict[str, Any],
    repo_dir: Path,
    inst_dir: Path,
) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    for plan_id in workflow_applied_plan_ids(workflow):
        path = find_plan_path_by_id(repo_dir, inst_dir, plan_id)
        plan = load_plan(path)
        if not plan:
            continue
        source_paths = plan_source_paths(plan)
        test_paths = plan_test_paths(plan)
        out.append({
            "plan_id": plan_id,
            "plan_path": str(path) if path else "",
            "source_paths": source_paths,
            "test_paths": test_paths,
            "all_paths": plan_change_paths(plan),
            "plan": plan,
        })
    return out


def export_allowed_patch_paths(
    workflow: dict[str, Any],
    repo_dir: Path,
    inst_dir: Path,
    final_plan: dict[str, Any],
    include_test_patches: bool,
) -> list[str]:
    """Return the typed path allowlist for prediction export.

    Environment preparation and editable installs can touch generated files in
    historical projects. Official predictions must export only paths owned by
    applied ChangePlans, with optional test paths controlled by the existing
    include-test flag. Falling back to all git diff paths would let build
    artifacts masquerade as code patches.
    """

    allowed: set[str] = set()
    summaries = workflow_applied_plan_summaries(workflow, repo_dir, inst_dir) if workflow else []
    for summary in summaries:
        for path in summary.get("source_paths") or []:
            normalized = normalize_repo_rel_file_path(path)
            if normalized:
                allowed.add(normalized)
        if include_test_patches:
            for path in summary.get("test_paths") or []:
                normalized = normalize_repo_rel_file_path(path)
                if normalized:
                    allowed.add(normalized)
    if not allowed and final_plan:
        for path in plan_source_paths(final_plan):
            normalized = normalize_repo_rel_file_path(path)
            if normalized:
                allowed.add(normalized)
        if include_test_patches:
            for path in plan_test_paths(final_plan):
                normalized = normalize_repo_rel_file_path(path)
                if normalized:
                    allowed.add(normalized)
    return sorted(allowed)


def combine_patch_review_summaries(plans: Iterable[dict[str, Any]]) -> dict[str, Any]:
    """Merge source-owner patch reviews without reading prose or issue text."""

    summaries = [plan_patch_review_summary(plan) for plan in plans if plan]
    if not summaries:
        return plan_patch_review_summary({})
    reason_codes: set[str] = set()
    semantic_unverified: set[str] = set()
    finding_count = 0
    hard_block = False
    status_values: set[str] = set()
    coverage_rank = {
        "failed": 5,
        "unverified": 4,
        "advisory": 3,
        "verified": 2,
        "clean": 1,
        "": 0,
    }
    coverage_verdict = ""
    block_reason = ""
    for summary in summaries:
        hard_block = hard_block or bool(summary.get("hard_block"))
        status = str(summary.get("status") or "").strip()
        if status:
            status_values.add(status)
        verdict = str(summary.get("coverage_verdict") or "").strip()
        if coverage_rank.get(verdict, 0) > coverage_rank.get(coverage_verdict, 0):
            coverage_verdict = verdict
        for code in summary.get("reason_codes") or []:
            code = str(code or "").strip()
            if code:
                reason_codes.add(code)
        for code in summary.get("semantic_unverified_codes") or []:
            code = str(code or "").strip()
            if code:
                semantic_unverified.add(code)
        finding_count += int(summary.get("finding_count") or 0)
        reason = str(summary.get("block_reason") or "").strip()
        if reason and not block_reason:
            block_reason = reason
    if not block_reason and semantic_unverified:
        block_reason = "patch_review_semantic_unverified:" + sorted(semantic_unverified)[0]
    if not block_reason and hard_block:
        block_reason = "patch_review_hard_block"
    status = "passed"
    if "failed" in status_values:
        status = "failed"
    elif not status_values:
        status = ""
    elif len(status_values) == 1:
        status = next(iter(status_values))
    return {
        "status": status,
        "hard_block": hard_block,
        "coverage_verdict": coverage_verdict,
        "reason_codes": sorted(reason_codes),
        "semantic_unverified_codes": sorted(semantic_unverified),
        "finding_count": finding_count,
        "block_reason": block_reason,
    }


def build_write_delivery_candidate(
    *,
    repo_dir: Path,
    inst_dir: Path,
    workflow: dict[str, Any],
    final_plan: dict[str, Any],
    final_plan_path: Path | None,
    exported_source_paths: list[str],
    exported_test_paths: list[str],
    patch_source: str,
    patch_fingerprint: str = "",
) -> dict[str, Any]:
    """Bind exported patch, source-owner plans, and verification into one fact.

    The adapter exports a cumulative git diff, while the latest durable plan can
    be a validation-only or test-only follow-up. This candidate is the typed
    bridge between those worlds: it records which applied source plans own the
    exported source paths and which report should be used as the verification
    authority. No stdout, model rationale, or problem statement text is read.
    """

    final_plan_id = str(final_plan.get("id") or "").strip()
    final_source_paths = plan_source_paths(final_plan)
    final_test_paths = plan_test_paths(final_plan)
    final_test_only = bool(plan_change_paths(final_plan)) and not final_source_paths
    summaries = workflow_applied_plan_summaries(workflow, repo_dir, inst_dir) if workflow else []
    if not summaries and final_plan:
        summaries = [{
            "plan_id": final_plan_id,
            "plan_path": str(final_plan_path) if final_plan_path else "",
            "source_paths": final_source_paths,
            "test_paths": final_test_paths,
            "all_paths": plan_change_paths(final_plan),
            "plan": final_plan,
        }]

    exported_source_set = set(exported_source_paths)
    owner_summaries: list[dict[str, Any]] = []
    covered_source_paths: set[str] = set()
    for summary in summaries:
        source_paths = {str(path) for path in summary.get("source_paths") or []}
        if not source_paths:
            continue
        if exported_source_set:
            intersection = source_paths & exported_source_set
            if not intersection:
                continue
            covered_source_paths.update(intersection)
        else:
            covered_source_paths.update(source_paths)
        owner_summaries.append(summary)

    source_owner_ids = [str(summary.get("plan_id") or "") for summary in owner_summaries if str(summary.get("plan_id") or "")]
    source_owner_plans = [summary.get("plan") for summary in owner_summaries if isinstance(summary.get("plan"), dict)]
    primary_summary = owner_summaries[-1] if owner_summaries else {}
    primary_plan = primary_summary.get("plan") if isinstance(primary_summary.get("plan"), dict) else {}
    primary_plan_path = Path(str(primary_summary.get("plan_path"))) if str(primary_summary.get("plan_path") or "") else None
    source_paths = sorted({path for summary in owner_summaries for path in summary.get("source_paths") or []})
    source_covers_export = None if not exported_source_paths else exported_source_set <= covered_source_paths

    report_plan_id = ""
    report_plan_path: Path | None = None
    final_report_path = final_plan_path.with_name(final_plan_path.stem + ".report.json") if final_plan_path else None
    primary_report_path = primary_plan_path.with_name(primary_plan_path.stem + ".report.json") if primary_plan_path else None
    if final_report_path and final_report_path.exists():
        report_plan_id = final_plan_id
        report_plan_path = final_plan_path
    elif primary_report_path and primary_report_path.exists():
        report_plan_id = str(primary_summary.get("plan_id") or "")
        report_plan_path = primary_plan_path

    status = "coherent"
    reason_code = ""
    relation = "source_plan_owns_exported_patch"
    if exported_source_paths and not owner_summaries:
        status = "incoherent"
        reason_code = "delivery_source_plan_missing"
        relation = ""
    elif source_covers_export is False:
        status = "incoherent"
        reason_code = "delivery_source_paths_not_owned"
        relation = ""
    elif final_test_only and source_owner_ids and final_plan_id and final_plan_id not in source_owner_ids:
        relation = "source_plan_with_later_test_followup"
    elif not exported_source_paths and exported_test_paths:
        relation = "test_patch_only"
    elif not exported_source_paths:
        status = "empty"
        reason_code = "delivery_no_exported_source_patch"
        relation = ""

    return {
        "status": status,
        "reason_code": reason_code,
        "relation": relation,
        "patch_source": patch_source,
        "patch_fingerprint": patch_fingerprint,
        "final_plan_id": final_plan_id,
        "final_plan_path": str(final_plan_path) if final_plan_path else "",
        "final_plan_source_paths": final_source_paths,
        "final_plan_test_paths": final_test_paths,
        "final_plan_test_only": final_test_only,
        "source_owner_plan_ids": source_owner_ids,
        "primary_source_plan_id": str(primary_summary.get("plan_id") or ""),
        "primary_source_plan_path": str(primary_summary.get("plan_path") or ""),
        "source_paths": source_paths,
        "source_plan_covers_exported_source_patch": source_covers_export,
        "exported_source_paths": exported_source_paths,
        "exported_test_paths": exported_test_paths,
        "report_plan_id": report_plan_id,
        "report_plan_path": str(report_plan_path) if report_plan_path else "",
        "primary_plan": primary_plan,
        "source_owner_plans": source_owner_plans,
    }


def normalize_repo_rel_path(raw: Any) -> str:
    text = str(raw or "").strip().strip("`'\"")
    if not text:
        return ""
    text = text.replace("\\", "/")
    if text.startswith("./"):
        text = text[2:]
    if text.startswith("/") or "://" in text or text in {".", ".."}:
        return ""
    parts = [part for part in text.split("/") if part not in {"", "."}]
    if not parts or any(part == ".." for part in parts):
        return ""
    return "/".join(parts)


def normalize_repo_rel_file_path(raw: Any) -> str:
    """Normalize a repo-relative path-like token for file-level comparison.

    Context packs sometimes carry symbol-qualified anchors such as
    `src/pkg/module.py:ClassName`. SWE-bench audit coverage compares changed
    source files, not symbols, so collapse those anchors to their file path.
    """

    path = normalize_repo_rel_path(raw)
    if not path or ":" not in path:
        return path
    head, _tail = path.split(":", 1)
    if head and ("/" in head or "." in Path(head).name):
        return head
    return path


def load_latest_workflow_run(repo_dir: Path, inst_dir: Path) -> tuple[dict[str, Any], str]:
    candidates: list[tuple[float, Path]] = []
    for root in (
        repo_dir / ".codrax" / "plans" / "workflows",
        inst_dir / ".codrax" / "plans" / "workflows",
    ):
        if not root.is_dir():
            continue
        for path in root.glob("*.json"):
            try:
                row = json.loads(path.read_text(encoding="utf-8"))
            except Exception:
                continue
            if isinstance(row, dict) and row.get("run_id"):
                candidates.append((path.stat().st_mtime, path))
    if not candidates:
        return {}, ""
    candidates.sort(key=lambda item: item[0])
    path = candidates[-1][1]
    try:
        row = json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return {}, ""
    return (row if isinstance(row, dict) else {}), str(path)


def workflow_context_paths(workflow: dict[str, Any]) -> list[str]:
    paths: list[str] = []
    for pack in workflow.get("context_packs") or []:
        if not isinstance(pack, dict):
            continue
        pack_stage = str(pack.get("source_stage") or "").strip()
        pack_id = str(pack.get("pack_id") or "").strip()
        if pack_stage == "plan" or pack_id == "change-plan":
            continue
        for item in pack.get("items") or []:
            if not isinstance(item, dict):
                continue
            if str(item.get("source_stage") or "").strip() == "plan":
                continue
            priority = str(item.get("priority") or "").strip().lower()
            kind = str(item.get("kind") or "").strip()
            if priority not in {"p0", "p1"}:
                continue
            path = ""
            evidence = item.get("evidence_ref")
            if kind == "evidence_ref" and isinstance(evidence, dict):
                path = normalize_repo_rel_file_path(evidence.get("source"))
            elif kind in {"target_file", "scope_anchor"}:
                path = normalize_repo_rel_file_path(item.get("text"))
            if path and not is_test_patch_path(path):
                paths.append(path)
    return sorted(set(paths))


def summarize_plan_context_coverage(
    workflow: dict[str, Any],
    target_paths: list[str],
    change_paths: list[str],
) -> dict[str, Any]:
    context_paths = workflow_context_paths(workflow)
    plan_paths = sorted(set(
        path for path in (normalize_repo_rel_file_path(p) for p in [*target_paths, *change_paths]) if path
    ))
    covered = [path for path in context_paths if path in plan_paths]
    uncovered = [path for path in context_paths if path not in plan_paths]
    ratio = None
    if context_paths:
        ratio = round(len(covered) / len(context_paths), 4)
    return {
        "plan_context_paths": context_paths,
        "plan_context_covered_paths": covered,
        "plan_context_uncovered_paths": uncovered,
        "plan_context_coverage_ratio": ratio,
    }


def plan_source_paths_missing_prior_context(
    plan_source_paths: Iterable[str],
    context_paths: Iterable[str],
) -> list[str]:
    """Return changed source files that lack prior typed localization context.

    This is intentionally audit-only. It consumes persisted typed context-pack
    paths and ChangePlan paths; it never reads issue text or model prose.
    """

    context = {
        path
        for path in (normalize_repo_rel_file_path(p) for p in context_paths)
        if path and not is_test_patch_path(path)
    }
    source = {
        path
        for path in (normalize_repo_rel_file_path(p) for p in plan_source_paths)
        if path and not is_test_patch_path(path)
    }
    if not context:
        return sorted(source)
    out = {
        path
        for path in source
        if path not in context
    }
    return sorted(out)


def plan_owner_boundary_signals(plan: dict[str, Any] | None) -> list[dict[str, Any]]:
    """Return typed audit hints that a plan may be fixing the caller boundary.

    This is deliberately audit-only. It consumes structured ChangePlan edits and
    Python expression ASTs. It does not read issue text, model rationale, or
    natural-language summaries.
    """

    if not plan:
        return []
    out: list[dict[str, Any]] = []
    for change in plan.get("changes") or []:
        if not isinstance(change, dict):
            continue
        path = normalize_repo_rel_file_path(change.get("path"))
        if not path.endswith(".py"):
            continue
        edits = change.get("edits") or []
        for idx, edit in enumerate(edits):
            if not isinstance(edit, dict):
                continue
            old_expr = python_changed_line_expr(edit.get("old_text"))
            new_expr = python_changed_line_expr(edit.get("content"))
            if old_expr is not None and new_expr is not None:
                signal = caller_return_adapter_signal(old_expr, new_expr)
                if signal:
                    signal.update({
                        "reason_code": "caller_return_shape_adapter",
                        "path": path,
                        "edit_index": idx,
                    })
                    out.append(signal)
            for signal in python_patch_quality_signals(edit.get("old_text"), edit.get("content")):
                signal.update({
                    "path": path,
                    "edit_index": idx,
                })
                out.append(signal)
    return out


def python_patch_quality_signals(old_text: Any, new_text: Any) -> list[dict[str, Any]]:
    """Return typed audit signals for likely symptom-level Python patches.

    The checks are structural and audit-only. They inspect Python AST produced
    from the structured edit payload, never issue text, model summaries, or
    natural-language rationales.
    """

    old_body = parse_python_edit_body(old_text)
    new_body = parse_python_edit_body(new_text)
    if not new_body:
        return []
    out: list[dict[str, Any]] = []
    if old_body and warning_call_count(old_body) > 0 and warning_call_count(new_body) > 0:
        if warning_call_under_conditional_count(new_body) > warning_call_under_conditional_count(old_body):
            out.append({"reason_code": "diagnostic_signal_conditionally_suppressed"})
    private_writes = external_private_attribute_assignments(new_body)
    if private_writes:
        out.append({
            "reason_code": "external_private_state_sync_workaround",
            "targets": sorted(private_writes),
        })
    return out


def parse_python_edit_body(raw: Any) -> list[ast.stmt]:
    text = str(raw or "").rstrip()
    if not text.strip():
        return []
    candidates = [text]
    stripped = textwrap.dedent(text)
    if stripped != text:
        candidates.append(stripped)
    if stripped.startswith("except ") or stripped.startswith("except:") or stripped.startswith("finally:"):
        candidates.append("try:\n    pass\n" + stripped)
    indented = "\n".join("    " + line if line.strip() else line for line in stripped.splitlines())
    candidates.append("def __codrax_edit__():\n" + indented)
    for candidate in candidates:
        try:
            module = ast.parse(candidate)
        except SyntaxError:
            continue
        if (
            len(module.body) == 1
            and isinstance(module.body[0], ast.FunctionDef)
            and module.body[0].name == "__codrax_edit__"
        ):
            return list(module.body[0].body)
        return list(module.body)
    return []


def warning_call_count(body: list[ast.stmt]) -> int:
    return sum(1 for node in ast.walk(ast.Module(body=body, type_ignores=[])) if is_warning_call(node))


def warning_call_under_conditional_count(body: list[ast.stmt]) -> int:
    count = 0
    for stmt in ast.walk(ast.Module(body=body, type_ignores=[])):
        if not isinstance(stmt, ast.If):
            continue
        count += sum(1 for node in ast.walk(stmt) if is_warning_call(node))
    return count


def is_warning_call(node: ast.AST) -> bool:
    if not isinstance(node, ast.Call):
        return False
    name = python_call_name(node)
    if not name:
        return False
    tail = name.rsplit(".", 1)[-1]
    return tail in {"warn", "warning"}


def external_private_attribute_assignments(body: list[ast.stmt]) -> set[str]:
    out: set[str] = set()
    for node in ast.walk(ast.Module(body=body, type_ignores=[])):
        targets: list[ast.expr] = []
        if isinstance(node, ast.Assign):
            targets.extend(node.targets)
        elif isinstance(node, ast.AnnAssign):
            targets.append(node.target)
        elif isinstance(node, ast.AugAssign):
            targets.append(node.target)
        for target in targets:
            if not isinstance(target, ast.Attribute) or not target.attr.startswith("_"):
                continue
            owner = python_expr_name(target.value)
            if not owner or owner == "self" or owner.startswith("self."):
                continue
            out.add(owner + "." + target.attr)
    return out


def python_changed_line_expr(raw: Any) -> ast.AST | None:
    text = str(raw or "").strip()
    if not text:
        return None
    try:
        module = ast.parse(text)
    except SyntaxError:
        return None
    if len(module.body) != 1:
        return None
    stmt = module.body[0]
    if isinstance(stmt, ast.Assign) and stmt.value is not None:
        return stmt.value
    if isinstance(stmt, ast.AnnAssign) and stmt.value is not None:
        return stmt.value
    if isinstance(stmt, ast.Return) and stmt.value is not None:
        return stmt.value
    if isinstance(stmt, ast.Expr):
        return stmt.value
    return None


def caller_return_adapter_signal(old_expr: ast.AST, new_expr: ast.AST) -> dict[str, Any] | None:
    old_calls = {
        ast_identity(call): call
        for call in ast.walk(old_expr)
        if isinstance(call, ast.Call)
    }
    if not old_calls:
        return None
    for call in ast.walk(new_expr):
        if not isinstance(call, ast.Call):
            continue
        adapter = python_call_name(call)
        if not is_return_adapter_call(adapter):
            continue
        for nested in ast.walk(call):
            if not isinstance(nested, ast.Call) or nested is call:
                continue
            key = ast_identity(nested)
            if key not in old_calls:
                continue
            return {
                "adapter": adapter,
                "inner_call": python_call_name(nested),
            }
    return None


def ast_identity(node: ast.AST) -> str:
    return ast.dump(node, include_attributes=False)


def python_call_name(call: ast.Call) -> str:
    return python_func_name(call.func)


def python_func_name(node: ast.AST) -> str:
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        parent = python_func_name(node.value)
        if parent:
            return parent + "." + node.attr
        return node.attr
    if isinstance(node, ast.Call):
        return python_func_name(node.func)
    return ""


def python_expr_name(node: ast.AST) -> str:
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        parent = python_expr_name(node.value)
        if parent:
            return parent + "." + node.attr
        return node.attr
    if isinstance(node, ast.Call):
        return python_expr_name(node.func)
    return ""


def is_return_adapter_call(name: str) -> bool:
    if not name:
        return False
    tail = name.rsplit(".", 1)[-1]
    return tail in CALLER_RETURN_ADAPTER_NAMES


def report_verification_status(report: dict[str, Any]) -> str:
    status = str(report.get("verification_status") or "").strip()
    if status:
        return status
    failure_kind = str(report.get("failure_kind") or "").strip()
    if failure_kind in {"runner_missing", "parser_error"}:
        return "unavailable"
    if report.get("no_tests_runners") and not report.get("test_results"):
        return "unavailable"
    if not bool(report.get("passed")):
        return "failed"
    return "passed"


def report_authoritative_verify_passed(report: dict[str, Any]) -> bool:
    """Return whether the typed local verifier produced an authoritative pass.

    ChangeReport.passed is a raw executor boolean and can be true for
    unavailable/no-tests outcomes. Dashboards must not treat that raw bit as a
    functional correctness signal; the normalized verification status is the
    typed verdict boundary.
    """

    return report_verification_status(report) == "passed"


def report_failure_kind(report: dict[str, Any]) -> str:
    failure_kind = str(report.get("failure_kind") or "").strip()
    if failure_kind:
        return failure_kind
    if report_verification_status(report) == "unavailable" and report.get("no_tests_runners"):
        return "no_tests"
    return ""


def report_failure_reason_code(report: dict[str, Any]) -> str:
    return str(report.get("failure_reason_code") or "").strip()


def prediction_verdict(
    patch: str,
    verify_status: str,
    plan_status: str,
    audit_block_reason: str = "",
    confidence_downgrade_reason: str = "",
) -> tuple[str, str, bool]:
    if not patch.strip():
        return "empty_patch", "none", bool(audit_block_reason)
    status = str(verify_status or "").strip()
    plan = str(plan_status or "").strip()
    if audit_block_reason == "workflow_blocked_after_verify_infra" and status != "failed":
        return "predicted_audit_blocked", "unknown", True
    if status == "failed" or plan == "verify_failed":
        return "predicted_failed_verify", "failed", True
    if audit_block_reason:
        return "predicted_audit_blocked", "failed", True
    if status == "passed":
        if confidence_downgrade_reason:
            return "predicted_passed_low_confidence", "unknown", False
        return "predicted_passed", "high", False
    if status == "unavailable" or plan == "unverified":
        return "predicted_unverified", "unknown", False
    return "predicted_unchecked", "unknown", False


def plan_patch_review_summary(plan: dict[str, Any] | None) -> dict[str, Any]:
    """Return typed patch-review telemetry from the final ChangePlan.

    This consumes only the structured `patch_review` record produced from the
    actual applied diff. It does not inspect issue text, model prose, or manual
    audit notes.
    """

    review = plan.get("patch_review") if isinstance(plan, dict) else None
    if not isinstance(review, dict):
        return {
            "status": "",
            "hard_block": False,
            "coverage_verdict": "",
            "reason_codes": [],
            "semantic_unverified_codes": [],
            "finding_count": 0,
            "block_reason": "",
        }
    coverage_summary = review.get("coverage_summary") if isinstance(review.get("coverage_summary"), dict) else {}
    reason_codes: set[str] = set()
    semantic_unverified: set[str] = set()
    for code in coverage_summary.get("reason_codes") or []:
        code = str(code or "").strip()
        if code:
            reason_codes.add(code)
    block_reason = str(coverage_summary.get("block_reason") or "").strip()
    findings = [row for row in review.get("findings") or [] if isinstance(row, dict)]
    for finding in findings:
        code = str(finding.get("code") or "").strip()
        if not code:
            continue
        reason_codes.add(code)
        severity = str(finding.get("severity") or "").strip()
        category = str(finding.get("category") or "").strip()
        coverage = str(finding.get("coverage_status") or "").strip()
        if severity == "error" and not block_reason:
            block_reason = "patch_review_error:" + code
        if (
            category == "semantic_coverage"
            and coverage == "unverified"
            and code in PATCH_REVIEW_LOCAL_BLOCKER_CODES
        ):
            semantic_unverified.add(code)
    hard_block = bool(review.get("hard_block"))
    if hard_block and not block_reason:
        block_reason = "patch_review_hard_block"
    if not block_reason and semantic_unverified:
        block_reason = "patch_review_semantic_unverified:" + sorted(semantic_unverified)[0]
    return {
        "status": str(review.get("status") or "").strip(),
        "hard_block": hard_block,
        "coverage_verdict": str(coverage_summary.get("verdict") or "").strip(),
        "reason_codes": sorted(reason_codes),
        "semantic_unverified_codes": sorted(semantic_unverified),
        "finding_count": len(findings),
        "block_reason": block_reason,
    }


MANUAL_AUDIT_VERDICTS = {"pass", "fail", "unknown"}


def normalize_manual_audit_record(row: dict[str, Any]) -> dict[str, str]:
    """Normalize an optional human audit row into typed telemetry.

    The adapter treats this as an explicit verdict field only. Free-form notes
    are preserved for audit, but they never drive acceptance logic.
    """

    instance_id = str(row.get("instance_id") or "").strip()
    verdict = str(row.get("manual_audit_verdict") or row.get("verdict") or "").strip().lower()
    if verdict not in MANUAL_AUDIT_VERDICTS:
        verdict = ""
    return {
        "instance_id": instance_id,
        "verdict": verdict,
        "reason_code": str(row.get("manual_audit_reason_code") or row.get("reason_code") or "").strip(),
        "source": str(row.get("manual_audit_source") or row.get("source") or "").strip(),
        "notes": str(row.get("manual_audit_notes") or row.get("notes") or "").strip(),
    }


def load_manual_audits(path: Path | None) -> dict[str, dict[str, str]]:
    if path is None:
        return {}
    out: dict[str, dict[str, str]] = {}
    for row in load_jsonl(path):
        if not isinstance(row, dict):
            continue
        record = normalize_manual_audit_record(row)
        instance_id = record["instance_id"]
        if not instance_id or not record["verdict"]:
            continue
        out[instance_id] = record
    return out


def local_acceptance_verdict(
    *,
    verify_status: str,
    prediction_blocks_local_acceptance: bool,
    manual_audit_verdict: str = "",
) -> tuple[str, str]:
    """Return Codrax's internal acceptance proxy.

    This is not the official SWE-bench score. It combines authoritative local
    verification with an optional typed human verdict, while keeping hard local
    blockers and true failed verification ahead of manual pass annotations.
    """

    status = str(verify_status or "").strip()
    manual = str(manual_audit_verdict or "").strip().lower()
    if bool(prediction_blocks_local_acceptance):
        return "fail", "local_audit_block"
    if status == "failed":
        return "fail", "local_verify"
    if manual == "fail":
        return "fail", "manual_audit"
    if status == "passed":
        return "pass", "local_verify"
    if manual == "pass":
        return "pass", "manual_audit"
    return "unknown", ""


def required_behavior_contract_ids(plan: dict[str, Any], *, include_fallback: bool = False) -> set[str]:
    ids: set[str] = set()
    for contract in plan.get("behavior_contracts") or []:
        if not isinstance(contract, dict):
            continue
        if not bool(contract.get("required")):
            continue
        if not include_fallback and str(contract.get("source") or "").strip() == "expected_outcome_fallback":
            continue
        cid = str(contract.get("id") or "").strip()
        if cid:
            ids.add(cid)
    return ids


def report_passed_by_verification_probe(report: dict[str, Any]) -> bool:
    commands = [cmd for cmd in report.get("executed_commands") or [] if isinstance(cmd, dict)]
    probe_commands = [
        cmd for cmd in commands
        if str(cmd.get("runner") or "").strip() == "verification_probe"
    ]
    if not probe_commands:
        return False
    non_probe_executed = [
        cmd for cmd in commands
        if str(cmd.get("runner") or "").strip() != "verification_probe"
        and str(cmd.get("outcome") or "").strip() == "executed"
        and int(cmd.get("exit_code") or 0) == 0
    ]
    if non_probe_executed:
        return False
    test_results = [row for row in report.get("test_results") or [] if isinstance(row, dict)]
    return bool(test_results) and all(
        str(row.get("suite") or "").strip() == "verification_probe/python"
        for row in test_results
    )


def prediction_confidence_downgrade_reason(
    *,
    plan: dict[str, Any] | None,
    report: dict[str, Any] | None,
    verify_status: str,
    plan_source_paths: Iterable[str] | None = None,
    plan_context_paths: Iterable[str] | None = None,
    owner_boundary_reason_codes: Iterable[str] | None = None,
) -> str:
    """Return a typed reason why local confidence is below high.

    This is audit telemetry only. It does not block SWE-bench prediction export
    and does not route on user text or model prose.
    """

    status = str(verify_status or "").strip()
    if report:
        from_report = prediction_confidence_downgrade_reason_from_report(report)
        if from_report:
            return from_report
        failure_reason = report_failure_reason_code(report)
        if status in {"failed", "unavailable"} and failure_reason:
            return failure_reason
    if status and status != "passed":
        return "local_verification_" + status
    if not plan or not report:
        return ""
    for reason in owner_boundary_reason_codes or []:
        reason = str(reason or "").strip()
        if reason:
            return reason
    probes = [probe for probe in plan.get("verification_probes") or [] if isinstance(probe, dict)]
    if not probes or not report_passed_by_verification_probe(report):
        return ""
    required_contracts = required_behavior_contract_ids(plan, include_fallback=True)
    covered_contracts: set[str] = set()
    changed_symbols: set[str] = set()
    baseline_expected = False
    for probe in probes:
        covered_contracts.update(str(ref).strip() for ref in probe.get("contract_refs") or [] if str(ref).strip())
        changed_symbols.update(str(ref).strip() for ref in probe.get("changed_symbol_refs") or [] if str(ref).strip())
        baseline_expected = baseline_expected or bool(probe.get("expects_baseline_failure"))
    if required_contracts and not (required_contracts & covered_contracts):
        return "verification_probe_missing_required_contract_ref"
    if required_contracts and not changed_symbols:
        return "verification_probe_missing_changed_symbol_ref"
    if baseline_expected:
        return "verification_probe_baseline_not_run"
    if plan_source_paths is not None and plan_context_paths is not None:
        missing_source_context = plan_source_paths_missing_prior_context(plan_source_paths, plan_context_paths)
        if missing_source_context:
            return "verification_probe_changed_source_not_context_covered"
    return ""


def prediction_confidence_downgrade_reason_from_report(report: dict[str, Any] | None) -> str:
    if not report:
        return ""
    for record in report.get("verification_confidence") or []:
        if not isinstance(record, dict):
            continue
        status = str(record.get("status") or "").strip()
        reason = str(record.get("reason_code") or "").strip()
        if status not in {"missing", "unavailable", "failed", "error"} or not reason:
            continue
        if reason:
            return reason
    return ""


def verification_confidence_reason_codes(report: dict[str, Any] | None) -> list[str]:
    if not report:
        return []
    out: list[str] = []
    seen: set[str] = set()
    for record in report.get("verification_confidence") or []:
        if not isinstance(record, dict):
            continue
        reason = str(record.get("reason_code") or "").strip()
        if not reason or reason in seen:
            continue
        seen.add(reason)
        out.append(reason)
    return out


def prediction_audit_block_reason(
    *,
    exported_source_paths: list[str],
    final_plan_source_paths: list[str],
    final_plan_test_only: bool,
    final_plan_covers_exported_source_patch: bool | None,
    workflow_status: str = "",
    verify_status: str = "",
    plan_status: str = "",
    workflow_latest_reason_code: str = "",
) -> str:
    """Return a typed local-acceptance blocker for final-plan/export drift.

    SWE-bench predictions may still be exported for the official harness. This
    reason only controls Codrax's local confidence so a passed local verifier
    cannot hide that the final durable plan no longer owns the source patch
    being submitted.
    """

    if not exported_source_paths:
        return ""
    workflow = str(workflow_status or "").strip()
    verify = str(verify_status or "").strip()
    plan = str(plan_status or "").strip()
    latest_reason = str(workflow_latest_reason_code or "").strip()
    if workflow == "blocked" and latest_reason == "verify_infra_retry_budget_exhausted" and verify != "failed":
        return "workflow_blocked_after_verify_infra"
    if workflow == "in_progress" and (verify == "failed" or plan == "verify_failed"):
        return "workflow_incomplete_after_failed_verify"
    if workflow == "blocked" and (verify == "failed" or plan == "verify_failed"):
        return "workflow_blocked_after_failed_verify"
    if final_plan_test_only:
        return "final_plan_test_only_exported_source_patch"
    if final_plan_covers_exported_source_patch is False:
        return "final_plan_exported_source_drift"
    if not final_plan_source_paths:
        return "final_plan_missing_source_paths"
    return ""


def empty_patch_reason(
    *,
    patch: str,
    workflow_status: str = "",
    plan_status: str = "",
    plan_approval_action: str = "",
    plan_path: str = "",
    codrax_timed_out: bool = False,
    codrax_exit_code: int = 0,
    workflow_latest_reason_code: str = "",
    workflow_latest_message: str = "",
) -> str:
    """Return a typed audit reason for an empty exported patch.

    Official SWE-bench predictions may still contain an empty patch, but local
    Codrax audit must explain whether the empty output came from timeout, a
    non-terminal workflow, a blocked workflow, or missing durable plan state.
    This reads only process status and durable typed workflow/plan fields.
    """

    if str(patch or "").strip():
        return ""
    workflow = str(workflow_status or "").strip()
    has_plan = bool(str(plan_path or "").strip())
    latest_reason = str(workflow_latest_reason_code or "").strip()
    latest_message = str(workflow_latest_message or "").strip()
    if codrax_timed_out:
        return "codrax_timeout_empty_patch"
    if latest_reason == "plan_batch_canceled":
        if "write mode wall-time exceeded" in latest_message:
            return "write_wall_time_empty_patch"
        return "workflow_canceled_empty_patch"
    if latest_reason == "plan_batch_failed" and "write mode wall-time exceeded" in latest_message:
        return "write_wall_time_empty_patch"
    if latest_reason == "verify_infra_retry_budget_exhausted":
        if "write mode wall-time exceeded" in latest_message:
            return "write_wall_time_empty_patch"
        return "verify_infra_budget_empty_patch"
    if latest_reason == "plan_batch_failed_blocked" and not has_plan:
        return "workflow_blocked_no_plan"
    if workflow == "in_progress":
        if (
            str(plan_status or "").strip() == "pending_approval"
            and str(plan_approval_action or "").strip() == "manual_approval"
        ):
            return "workflow_pending_approval_empty_patch"
        if not has_plan:
            return "workflow_in_progress_no_plan"
        return "workflow_in_progress_empty_patch"
    if workflow == "blocked":
        if not has_plan:
            return "workflow_blocked_no_plan"
        return "workflow_blocked_empty_patch"
    if int(codrax_exit_code or 0) != 0:
        return "codrax_failed_empty_patch"
    if not has_plan:
        return "no_plan_empty_patch"
    return "empty_patch"


def commit_exists(repo_dir: Path, rev: str) -> bool:
    if not rev:
        return False
    result = run_cmd(["git", "cat-file", "-e", f"{rev}^{{commit}}"], cwd=repo_dir, timeout=60)
    return result.code == 0


def is_test_patch_path(path: str) -> bool:
    norm = path.replace("\\", "/").strip("/")
    if not norm:
        return False
    parts = norm.split("/")
    if any(part in {"test", "tests", "spec", "specs", "__tests__"} for part in parts[:-1]):
        return True
    name = parts[-1]
    lower = name.lower()
    return (
        lower.startswith("test_")
        or lower.endswith("_test.py")
        or lower.endswith("_test.go")
        or lower.endswith(".test.js")
        or lower.endswith(".spec.js")
        or lower.endswith(".test.ts")
        or lower.endswith(".spec.ts")
        or lower.endswith("_spec.rb")
    )


def export_patch_between(
    repo_dir: Path,
    base: str,
    head: str,
    include_test_patches: bool,
    allowed_paths: Iterable[str] | None = None,
) -> tuple[str, list[str], list[str], list[str]]:
    names = run_cmd(["git", "diff", "--name-only", base, head], cwd=repo_dir, timeout=120)
    if names.code != 0:
        result = run_cmd(["git", "diff", "--binary", base, head], cwd=repo_dir, timeout=120)
        return (result.output if result.code == 0 else ""), [], [], []
    changed = [line.strip() for line in names.output.splitlines() if line.strip()]
    allowed_input = list(allowed_paths) if allowed_paths is not None else None
    allowed = {
        normalize_repo_rel_file_path(path)
        for path in (allowed_input or [])
        if normalize_repo_rel_file_path(path)
    }
    test_paths = [path for path in changed if is_test_patch_path(path)]
    dropped = [] if include_test_patches else test_paths
    if allowed_input is not None:
        selected = [path for path in changed if normalize_repo_rel_file_path(path) in allowed]
        dropped_test_set = set(dropped)
        dropped_unowned = [
            path for path in changed
            if normalize_repo_rel_file_path(path) not in allowed and path not in dropped_test_set
        ]
    else:
        selected = changed if include_test_patches else [path for path in changed if not is_test_patch_path(path)]
        dropped_unowned = []
    if not selected:
        return "", dropped, [], dropped_unowned
    result = run_cmd(["git", "diff", "--binary", base, head, "--", *selected], cwd=repo_dir, timeout=120)
    if result.code == 0:
        return result.output, dropped, selected, dropped_unowned
    result = run_cmd(["git", "diff", "--binary", base, head], cwd=repo_dir, timeout=120)
    return (result.output if result.code == 0 else ""), [], changed if result.code == 0 else [], []


def export_patch(
    repo_dir: Path,
    base_commit: str,
    plan: dict[str, Any],
    include_test_patches: bool,
    allowed_paths: Iterable[str] | None = None,
) -> tuple[str, str, list[str], list[str], list[str]]:
    plan_id = str(plan.get("id") or "").strip()
    applied_sha = str(plan.get("applied_commit_sha") or "").strip()
    worktree = str(plan.get("worktree_path") or "").strip()
    commit = ""
    if commit_exists(repo_dir, applied_sha):
        commit = applied_sha
    elif plan_id and commit_exists(repo_dir, f"refs/codrax/applied/{plan_id}"):
        commit = f"refs/codrax/applied/{plan_id}"
    if commit:
        patch, dropped, exported_paths, dropped_unowned = export_patch_between(repo_dir, base_commit, commit, include_test_patches, allowed_paths)
        return patch, commit, dropped, exported_paths, dropped_unowned
    if worktree and Path(worktree).is_dir():
        patch, dropped, exported_paths, dropped_unowned = export_patch_between(Path(worktree), base_commit, "HEAD", include_test_patches, allowed_paths)
        return patch, "worktree:HEAD", dropped, exported_paths, dropped_unowned
    return "", "", [], [], []


def write_jsonl(path: Path, rows: Iterable[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    with tmp.open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(json.dumps(row, ensure_ascii=False, sort_keys=True))
            handle.write("\n")
    tmp.replace(path)


def sanitized_instance_artifact(instance: dict[str, Any]) -> dict[str, Any]:
    """Return a run artifact safe to place near the checked-out repository.

    SWE-bench rows often include the gold patch and oracle test lists. The
    adapter must never make those fields available to Codrax while it is
    generating a prediction. The request text is built only from typed public
    fields, and this artifact mirrors that boundary for files on disk.
    """

    out: dict[str, Any] = {}
    redacted: list[str] = []
    for key, value in instance.items():
        if key in GOLD_ARTIFACT_FIELDS:
            redacted.append(key)
            continue
        out[key] = value
    if redacted:
        out["_codrax_redacted_fields"] = sorted(redacted)
    return out


def process_instance(
    instance: dict[str, Any],
    args: argparse.Namespace,
    manual_audits: dict[str, dict[str, str]] | None = None,
) -> tuple[dict[str, Any], dict[str, Any]]:
    instance_id = required_field(instance, "instance_id")
    inst_dir = Path(args.workdir).resolve() / "instances" / safe_id(instance_id)
    repo_dir = inst_dir / "repo"
    inst_dir.mkdir(parents=True, exist_ok=True)
    (inst_dir / "instance.json").write_text(
        json.dumps(sanitized_instance_artifact(instance), ensure_ascii=False, indent=2, sort_keys=True),
        encoding="utf-8",
    )
    result: dict[str, Any] = {
        "instance_id": instance_id,
        "repo": instance.get("repo", ""),
        "status": "started",
        "instance_dir": str(inst_dir),
    }
    prediction = {
        "instance_id": instance_id,
        "model_name_or_path": args.model_name,
        "model_patch": "",
    }
    try:
        mirror = ensure_repo_cache(instance, args)
        base, history_isolation = checkout_instance(instance, mirror, repo_dir, args)
        result["base_commit_resolved"] = base
        result["git_history_isolated"] = bool(history_isolation.get("enabled"))
        result["git_history_isolation"] = history_isolation
        env_updates, env_record = prepare_python_env(repo_dir, inst_dir, args)
        result["env_prepare"] = env_record
        result["env_prepare_status"] = str(env_record.get("status") or "")
        result["env_prepare_success"] = bool(env_record.get("success"))
        result["env_prepare_env_available"] = bool(env_record.get("env_available"))
        result["env_prepare_failure_kind"] = str(env_record.get("failure_kind") or "")
        result["env_prepare_pytest_available"] = bool(env_record.get("pytest_available"))
        result["env_prepare_pytest_json_report_available"] = bool(env_record.get("pytest_json_report_available"))
        result["env_prepare_import_probe_ok"] = env_record.get("import_probe_ok")
        result["env_prepare_import_roots"] = env_record.get("import_roots") or []
        result["env_prepare_source_roots"] = env_record.get("source_roots") or []
        result["env_prepare_python_compat_constraints"] = env_record.get("python_compat_constraints") or []
        result["env_prepare_venv_python"] = str(env_record.get("venv_python") or "")
        result["env_prepare_failed_step_names"] = env_record.get("failed_step_names") or []
        (inst_dir / "env_prepare.json").write_text(json.dumps(env_record, ensure_ascii=False, indent=2, sort_keys=True), encoding="utf-8")
        env_log = "\n\n".join(
            f"## {cmd.get('name')}\n$ {' '.join(cmd.get('cmd') or [])}\ncode={cmd.get('code')} timeout={cmd.get('timed_out')}\n{cmd.get('output_tail') or ''}"
            for cmd in env_record.get("commands", [])
        )
        (inst_dir / "env_prepare.log").write_text(env_log, encoding="utf-8", errors="replace")
        codrax = run_codrax(instance, repo_dir, inst_dir, args, env_updates)
        result["codrax_exit_code"] = codrax.code
        result["codrax_timed_out"] = codrax.timed_out
        plan_path = find_latest_change_plan(repo_dir / ".codrax", inst_dir)
        result["plan_path"] = str(plan_path) if plan_path else ""
        plan = load_plan(plan_path)
        if plan:
            result["plan_id"] = str(plan.get("id") or "")
            result["plan_status"] = str(plan.get("status") or "")
            approval = plan.get("approval") if isinstance(plan.get("approval"), dict) else {}
            result["plan_approval_action"] = str(approval.get("action") or "")
            result["plan_approval_risk_level"] = str(approval.get("risk_level") or "")
            result["plan_approval_reason_code"] = str(approval.get("reason_code") or "")
            result["plan_approval_user_decision"] = str(approval.get("user_decision") or "")
            target_paths = [str(path).strip() for path in plan.get("target_paths") or [] if str(path).strip()]
            change_paths = plan_change_paths(plan)
            test_change_paths = [path for path in change_paths if is_test_patch_path(path)]
            result["plan_target_paths"] = target_paths
            result["plan_change_paths"] = change_paths
            result["plan_test_change_paths"] = test_change_paths
            result["plan_verification_probe_count"] = len(plan.get("verification_probes") or [])
        workflow, workflow_path = load_latest_workflow_run(repo_dir, inst_dir)
        result["workflow_path"] = workflow_path
        if workflow:
            result["workflow_run_id"] = str(workflow.get("run_id") or "")
            result["workflow_status"] = str(workflow.get("status") or "")
            latest_progress = latest_workflow_progress_event(workflow)
            result["workflow_latest_progress"] = latest_workflow_progress(workflow)
            result["workflow_latest_progress_stage"] = str(latest_progress.get("stage") or "")
            result["workflow_latest_progress_status"] = str(latest_progress.get("status") or "")
            result["workflow_latest_progress_reason_code"] = str(latest_progress.get("reason_code") or "")
            result["workflow_latest_progress_message"] = str(latest_progress.get("message") or "")
            target_paths = result.get("plan_target_paths") if isinstance(result.get("plan_target_paths"), list) else []
            change_paths = result.get("plan_change_paths") if isinstance(result.get("plan_change_paths"), list) else []
            result.update(summarize_plan_context_coverage(workflow, target_paths, change_paths))
            result.update(summarize_workflow_applied_plan_provenance(
                workflow,
                repo_dir,
                inst_dir,
                str(result.get("plan_id") or ""),
            ))
        else:
            result.update({
                "plan_context_paths": [],
                "plan_context_covered_paths": [],
                "plan_context_uncovered_paths": [],
                "plan_context_coverage_ratio": None,
                "workflow_applied_plan_ids": [],
                "workflow_latest_applied_plan_id": "",
                "workflow_final_plan_is_latest_applied": False,
                "workflow_applied_source_paths": [],
                "workflow_applied_test_paths": [],
            })
        export_allowed_paths = export_allowed_patch_paths(
            workflow,
            repo_dir,
            inst_dir,
            plan,
            bool(args.include_test_patches),
        )
        patch, source, dropped_test_paths, exported_paths, dropped_unowned_paths = export_patch(
            repo_dir,
            base,
            plan,
            bool(args.include_test_patches),
            export_allowed_paths,
        )
        patch_fingerprint = hashlib.sha256(patch.encode("utf-8")).hexdigest() if patch else ""
        exported_test_paths = [path for path in exported_paths if is_test_patch_path(path)]
        exported_source_paths = [path for path in exported_paths if not is_test_patch_path(path)]
        plan_source_paths = [
            path
            for path in (result.get("plan_change_paths") if isinstance(result.get("plan_change_paths"), list) else [])
            if isinstance(path, str) and not is_test_patch_path(path)
        ]
        plan_source_path_set = set(plan_source_paths)
        prediction["model_patch"] = patch
        result["patch_source"] = source
        result["dropped_test_patch_paths"] = dropped_test_paths
        result["dropped_unowned_patch_paths"] = dropped_unowned_paths
        result["export_allowed_patch_paths"] = export_allowed_paths
        result["exported_patch_paths"] = exported_paths
        result["exported_patch_source_paths"] = exported_source_paths
        result["exported_patch_test_paths"] = exported_test_paths
        result["final_plan_source_paths"] = plan_source_paths
        result["final_plan_test_only"] = bool(result.get("plan_change_paths")) and not plan_source_paths
        result["final_plan_covers_exported_source_patch"] = (
            None if not exported_source_paths else all(path in plan_source_path_set for path in exported_source_paths)
        )
        delivery = build_write_delivery_candidate(
            repo_dir=repo_dir,
            inst_dir=inst_dir,
            workflow=workflow,
            final_plan=plan,
            final_plan_path=plan_path,
            exported_source_paths=exported_source_paths,
            exported_test_paths=exported_test_paths,
            patch_source=source,
            patch_fingerprint=patch_fingerprint,
        )
        delivery_plan = (
            delivery.get("primary_plan")
            if isinstance(delivery.get("primary_plan"), dict) and delivery.get("primary_plan")
            else plan
        )
        delivery_source_owner_plans = [
            row for row in delivery.get("source_owner_plans") or [] if isinstance(row, dict)
        ]
        delivery_plan_source_paths = [
            str(path)
            for path in (delivery.get("source_paths") or [])
            if str(path).strip()
        ]
        delivery_report_plan_path = Path(str(delivery.get("report_plan_path"))) if str(delivery.get("report_plan_path") or "") else None
        report = load_report_for_plan(delivery_report_plan_path)
        if report:
            result["report_path"] = (
                str(delivery_report_plan_path.with_name(delivery_report_plan_path.stem + ".report.json"))
                if delivery_report_plan_path
                else ""
            )
            result["verify_status"] = report_verification_status(report)
            result["verify_passed"] = report_authoritative_verify_passed(report)
            result["verify_report_passed_raw"] = bool(report.get("passed"))
            result["verify_failure_kind"] = report_failure_kind(report)
            result["verify_failure_reason_code"] = report_failure_reason_code(report)
            result["verify_summary"] = str(report.get("failure_summary") or "")
            result["verify_no_tests_runners"] = report.get("no_tests_runners") or []
            result["verify_test_count"] = len(report.get("test_results") or [])
            result["verify_confidence_reason_codes"] = verification_confidence_reason_codes(report)
        result["delivery_candidate_status"] = str(delivery.get("status") or "")
        result["delivery_candidate_reason_code"] = str(delivery.get("reason_code") or "")
        result["delivery_candidate_relation"] = str(delivery.get("relation") or "")
        result["delivery_patch_fingerprint"] = str(delivery.get("patch_fingerprint") or "")
        result["delivery_final_plan_id"] = str(delivery.get("final_plan_id") or "")
        result["delivery_primary_source_plan_id"] = str(delivery.get("primary_source_plan_id") or "")
        result["delivery_source_owner_plan_ids"] = delivery.get("source_owner_plan_ids") or []
        result["delivery_source_paths"] = delivery_plan_source_paths
        result["delivery_report_plan_id"] = str(delivery.get("report_plan_id") or "")
        result["delivery_report_plan_path"] = str(delivery.get("report_plan_path") or "")
        result["delivery_source_plan_covers_exported_source_patch"] = delivery.get("source_plan_covers_exported_source_patch")
        workflow_applied_source_paths = (
            result.get("workflow_applied_source_paths")
            if isinstance(result.get("workflow_applied_source_paths"), list)
            else []
        )
        workflow_applied_source_path_set = {str(path) for path in workflow_applied_source_paths}
        result["workflow_applied_covers_exported_source_patch"] = (
            None if not exported_source_paths else all(path in workflow_applied_source_path_set for path in exported_source_paths)
        )
        result["plan_context_missing_source_paths"] = plan_source_paths_missing_prior_context(
            plan_source_paths,
            result.get("plan_context_paths") if isinstance(result.get("plan_context_paths"), list) else [],
        )
        result["delivery_context_missing_source_paths"] = plan_source_paths_missing_prior_context(
            delivery_plan_source_paths,
            result.get("plan_context_paths") if isinstance(result.get("plan_context_paths"), list) else [],
        )
        owner_boundary_signals = plan_owner_boundary_signals(delivery_plan)
        result["plan_owner_boundary_signals"] = owner_boundary_signals
        result["plan_owner_boundary_reason_codes"] = sorted({
            str(signal.get("reason_code") or "").strip()
            for signal in owner_boundary_signals
            if str(signal.get("reason_code") or "").strip()
        })
        final_patch_review_summary = plan_patch_review_summary(plan)
        result["final_plan_patch_review_status"] = str(final_patch_review_summary.get("status") or "")
        result["final_plan_patch_review_hard_block"] = bool(final_patch_review_summary.get("hard_block"))
        result["final_plan_patch_review_coverage_verdict"] = str(final_patch_review_summary.get("coverage_verdict") or "")
        result["final_plan_patch_review_reason_codes"] = final_patch_review_summary.get("reason_codes") or []
        result["final_plan_patch_review_semantic_unverified_codes"] = (
            final_patch_review_summary.get("semantic_unverified_codes") or []
        )
        result["final_plan_patch_review_finding_count"] = int(final_patch_review_summary.get("finding_count") or 0)
        result["final_plan_patch_review_block_reason"] = str(final_patch_review_summary.get("block_reason") or "")
        patch_review_summary = combine_patch_review_summaries(delivery_source_owner_plans or [delivery_plan])
        result["plan_patch_review_status"] = str(patch_review_summary.get("status") or "")
        result["plan_patch_review_hard_block"] = bool(patch_review_summary.get("hard_block"))
        result["plan_patch_review_coverage_verdict"] = str(patch_review_summary.get("coverage_verdict") or "")
        result["plan_patch_review_reason_codes"] = patch_review_summary.get("reason_codes") or []
        result["plan_patch_review_semantic_unverified_codes"] = (
            patch_review_summary.get("semantic_unverified_codes") or []
        )
        result["plan_patch_review_finding_count"] = int(patch_review_summary.get("finding_count") or 0)
        result["plan_patch_review_block_reason"] = str(patch_review_summary.get("block_reason") or "")
        result["patch_bytes"] = len(patch.encode("utf-8"))
        if result["delivery_candidate_status"] == "incoherent":
            audit_block_reason = str(result["delivery_candidate_reason_code"] or "delivery_candidate_incoherent")
        else:
            audit_block_reason = prediction_audit_block_reason(
                exported_source_paths=exported_source_paths,
                final_plan_source_paths=delivery_plan_source_paths,
                final_plan_test_only=False,
                final_plan_covers_exported_source_patch=result["delivery_source_plan_covers_exported_source_patch"],
                workflow_status=str(result.get("workflow_status") or ""),
                verify_status=str(result.get("verify_status") or ""),
                plan_status=str(result.get("plan_status") or ""),
                workflow_latest_reason_code=str(result.get("workflow_latest_progress_reason_code") or ""),
            )
        empty_reason = empty_patch_reason(
            patch=patch,
            workflow_status=str(result.get("workflow_status") or ""),
            plan_status=str(result.get("plan_status") or ""),
            plan_approval_action=str(result.get("plan_approval_action") or ""),
            plan_path=str(result.get("plan_path") or ""),
            codrax_timed_out=bool(result.get("codrax_timed_out")),
            codrax_exit_code=int(result.get("codrax_exit_code") or 0),
            workflow_latest_reason_code=str(result.get("workflow_latest_progress_reason_code") or ""),
            workflow_latest_message=str(result.get("workflow_latest_progress_message") or ""),
        )
        result["empty_patch_reason"] = empty_reason
        if empty_reason and not audit_block_reason:
            audit_block_reason = empty_reason
        if result["plan_patch_review_block_reason"] and not audit_block_reason:
            audit_block_reason = str(result["plan_patch_review_block_reason"])
        result["prediction_audit_block_reason"] = audit_block_reason
        confidence_downgrade_reason = prediction_confidence_downgrade_reason(
            plan=delivery_plan,
            report=report,
            verify_status=str(result.get("verify_status") or ""),
            plan_source_paths=delivery_plan_source_paths,
            plan_context_paths=result.get("plan_context_paths") if isinstance(result.get("plan_context_paths"), list) else [],
            owner_boundary_reason_codes=result.get("plan_owner_boundary_reason_codes") if isinstance(result.get("plan_owner_boundary_reason_codes"), list) else [],
        )
        result["prediction_confidence_downgrade_reason"] = confidence_downgrade_reason
        verdict, confidence, blocks_local_acceptance = prediction_verdict(
            patch,
            str(result.get("verify_status") or ""),
            str(result.get("plan_status") or ""),
            audit_block_reason,
            confidence_downgrade_reason,
        )
        result["prediction_verdict"] = verdict
        result["prediction_local_confidence"] = confidence
        result["prediction_blocks_local_acceptance"] = blocks_local_acceptance
        manual_audit = (manual_audits or {}).get(instance_id, {})
        result["manual_audit_verdict"] = str(manual_audit.get("verdict") or "")
        result["manual_audit_reason_code"] = str(manual_audit.get("reason_code") or "")
        result["manual_audit_source"] = str(manual_audit.get("source") or "")
        result["manual_audit_notes"] = str(manual_audit.get("notes") or "")
        local_verdict, local_source = local_acceptance_verdict(
            verify_status=str(result.get("verify_status") or ""),
            prediction_blocks_local_acceptance=bool(blocks_local_acceptance),
            manual_audit_verdict=str(result.get("manual_audit_verdict") or ""),
        )
        result["local_acceptance_verdict"] = local_verdict
        result["local_acceptance_source"] = local_source
        result["status"] = "predicted" if patch.strip() else "empty_patch"
    except Exception as exc:  # keep batch runs moving; official harness can score empty patches.
        result["status"] = "error"
        result["prediction_verdict"] = "adapter_error"
        result["prediction_local_confidence"] = "failed"
        result["prediction_blocks_local_acceptance"] = True
        result["local_acceptance_verdict"] = "fail"
        result["local_acceptance_source"] = "adapter_error"
        result["error"] = str(exc)
    (inst_dir / "prediction.json").write_text(json.dumps(prediction, ensure_ascii=False, indent=2, sort_keys=True), encoding="utf-8")
    (inst_dir / "result.json").write_text(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True), encoding="utf-8")
    return prediction, result


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--instances-jsonl", type=Path, help="Local SWE-bench-style instance JSONL")
    source.add_argument("--dataset-name", help="Hugging Face dataset name, e.g. SWE-bench/SWE-bench_Lite")
    parser.add_argument("--split", default="test", help="Dataset split for --dataset-name")
    parser.add_argument("--instance-id", action="append", default=[], help="Instance id to include; can be repeated or comma-separated")
    parser.add_argument("--instance-ids-file", type=Path, help="File with one instance id per line")
    parser.add_argument("--limit", type=int, default=0, help="Maximum number of selected instances; 0 means no limit")
    parser.add_argument("--repo-cache", default=str(ROOT / "eval" / "results" / "swebench" / "repo-cache"))
    parser.add_argument("--workdir", default=str(ROOT / "eval" / "results" / "swebench" / time.strftime("%Y%m%d-%H%M%S")))
    parser.add_argument("--predictions-path", help="Output JSONL path; default <workdir>/predictions.jsonl")
    parser.add_argument("--results-path", help="Output result JSONL path; default <workdir>/results.jsonl")
    parser.add_argument(
        "--manual-audit-jsonl",
        type=Path,
        help=(
            "Optional typed human audit rows with instance_id and verdict/pass|fail|unknown; "
            "used only for results.jsonl local_acceptance telemetry, never official predictions"
        ),
    )
    parser.add_argument("--codrax-bin", default=str(ROOT / "codrax"))
    parser.add_argument("--settings", default=str(DEFAULT_SETTINGS))
    parser.add_argument(
        "--providers",
        default=os.environ.get("CODRAX_PROVIDERS", ""),
        help="Optional providers.yaml path forwarded to Codrax. Defaults to CODRAX_PROVIDERS when set.",
    )
    parser.add_argument("--model-name", default="codrax")
    parser.add_argument("--max-steps", type=int, default=50)
    parser.add_argument("--codrax-timeout", type=int, default=1800)
    parser.add_argument(
        "--codrax-progress-interval",
        type=int,
        default=int(os.environ.get("CODRAX_PROGRESS_INTERVAL", "30")),
        help="Seconds between typed Codrax workflow progress heartbeats during long instance runs; 0 disables",
    )
    parser.add_argument("--git-timeout", type=int, default=600)
    parser.add_argument("--log-level", default="debug")
    parser.add_argument("--repo-url-template", default="https://github.com/{repo}.git")
    parser.add_argument("--request-prefix", default="")
    parser.add_argument(
        "--prepare-python-env",
        action="store_true",
        help="Best-effort per-instance Python venv for Codrax local verification; failures are recorded but do not block predictions",
    )
    parser.add_argument(
        "--include-test-patches",
        action="store_true",
        help="Include repository test/spec file changes in exported predictions; default strips them for SWE-bench scoring",
    )
    parser.add_argument(
        "--isolate-git-history",
        action="store_true",
        help="Fair-eval: keep checkout at base_commit but prune future git refs/history and pass --eval-disable-git-history to Codrax",
    )
    parser.add_argument("--env-prepare-timeout", type=int, default=600, help="Timeout in seconds per Python environment setup command")
    parser.add_argument(
        "--disable-python-compat-constraints",
        action="store_true",
        help="Disable adapter-generated Python compatibility constraints for broad lower-bound-only dependency specs",
    )
    parser.add_argument(
        "--fail-on-instance-error",
        action="store_true",
        help="Exit non-zero after writing artifacts when any instance fails in the adapter before Codrax can produce a patch",
    )
    parser.add_argument(
        "--fail-on-empty-patch",
        action="store_true",
        help="Exit non-zero after writing artifacts when any exported prediction has an empty model_patch",
    )
    parser.add_argument("--no-fetch", action="store_true", help="Do not fetch existing mirror caches")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    workdir = Path(args.workdir).resolve()
    workdir.mkdir(parents=True, exist_ok=True)
    if args.instances_jsonl:
        rows = load_jsonl(args.instances_jsonl)
    else:
        rows = load_dataset_rows(args.dataset_name, args.split)
    instance_ids = read_instance_ids(args.instance_id, args.instance_ids_file)
    instances = select_instances(rows, instance_ids, args.limit)
    if not instances:
        raise SystemExit("no instances selected")
    predictions_path = Path(args.predictions_path).resolve() if args.predictions_path else workdir / "predictions.jsonl"
    results_path = Path(args.results_path).resolve() if args.results_path else workdir / "results.jsonl"
    manual_audits = load_manual_audits(args.manual_audit_jsonl)

    predictions: list[dict[str, Any]] = []
    results: list[dict[str, Any]] = []
    for index, instance in enumerate(instances, 1):
        instance_id = str(instance.get("instance_id") or "")
        print(f"[{index}/{len(instances)}] {instance_id}", file=sys.stderr)
        prediction, result = process_instance(instance, args, manual_audits)
        predictions.append(prediction)
        results.append(result)
        write_jsonl(predictions_path, predictions)
        write_jsonl(results_path, results)
        print(f"  status={result.get('status')} patch_bytes={result.get('patch_bytes', 0)}", file=sys.stderr)

    print(f"predictions: {predictions_path}", file=sys.stderr)
    print(f"results: {results_path}", file=sys.stderr)
    failures: list[str] = []
    if args.fail_on_instance_error:
        failures.extend(
            f"{r.get('instance_id')}: {r.get('error') or r.get('status')}"
            for r in results
            if str(r.get("status") or "") == "error"
        )
    if args.fail_on_empty_patch:
        failures.extend(
            f"{r.get('instance_id')}: empty patch ({r.get('empty_patch_reason') or r.get('status')})"
            for idx, r in enumerate(results)
            if idx < len(predictions) and not str(predictions[idx].get("model_patch") or "").strip()
        )
    if failures:
        for failure in failures:
            print(f"smoke failure: {failure}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
