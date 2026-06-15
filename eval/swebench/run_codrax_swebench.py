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
import json
import os
import re
import shutil
import signal
import subprocess
import sys
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
            raise RuntimeError(f"git fetch failed for {mirror}\n{result.output[-4000:]}")
    return mirror


def checkout_instance(instance: dict[str, Any], mirror: Path, repo_dir: Path, args: argparse.Namespace) -> str:
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
    return resolved.output.strip()


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
    for base in (repo_dir, repo_dir / "src"):
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


def venv_bin_dir(venv_dir: Path) -> Path:
    if os.name == "nt":
        return venv_dir / "Scripts"
    return venv_dir / "bin"


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
        return {}, record
    if not is_python_project(repo_dir):
        record["status"] = "skipped_non_python"
        return {}, record

    venv_dir = inst_dir / "python-env"
    if venv_dir.exists():
        shutil.rmtree(venv_dir)
    py_timeout = int(args.env_prepare_timeout)

    def step(name: str, cmd: list[str], *, cwd: Path | None = None) -> CommandResult:
        result = run_cmd(cmd, cwd=cwd, timeout=py_timeout)
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

    created = step("create_venv", [sys.executable, "-m", "venv", str(venv_dir)])
    if created.code != 0 or created.timed_out:
        record["status"] = "failed_create_venv"
        return {}, record

    python = venv_bin_dir(venv_dir) / "python"
    if not python.exists():
        record["status"] = "failed_missing_python"
        return {}, record

    env_updates = {
        "VIRTUAL_ENV": str(venv_dir),
        "PATH": f"{venv_bin_dir(venv_dir)}{os.pathsep}{os.environ.get('PATH', '')}",
    }
    constraint_files = discover_python_constraint_files(repo_dir)
    if constraint_files:
        record["python_constraint_files"] = constraint_files

    def pip_install_cmd(*items: str) -> list[str]:
        cmd = [str(python), "-m", "pip", "install"]
        for rel in constraint_files:
            cmd.extend(["-c", str(repo_dir / rel)])
        cmd.extend(items)
        return cmd

    upgraded = step("upgrade_packaging", [str(python), "-m", "pip", "install", "--upgrade", "pip", "setuptools", "wheel"])
    if upgraded.code != 0 or upgraded.timed_out:
        record["status"] = "failed_packaging"
        record["env_path"] = str(venv_dir)
        return env_updates, record

    pytest = step("install_pytest", pip_install_cmd("pytest<9", "pytest-json-report"))
    if pytest.code != 0 or pytest.timed_out:
        record["status"] = "failed_pytest"
        record["env_path"] = str(venv_dir)
        return env_updates, record
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
    requirement_files = discover_python_requirement_files(repo_dir)
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
    setup_requires = setup_py_declared_requires(repo_dir)
    if setup_requires:
        record["setup_declared_requires"] = setup_requires
        setup_req = step("install_setup_declared_requires", pip_install_cmd(*setup_requires), cwd=repo_dir)
        project_failed = project_failed or setup_req.code != 0 or setup_req.timed_out
        if not ensure_legacy_pkg_resources("legacy_pkg_resources_post_setup_requires"):
            record["pkg_resources_available"] = False
            project_failed = True
        else:
            record["pkg_resources_available"] = True
    if (repo_dir / "pyproject.toml").exists() or (repo_dir / "setup.py").exists() or (repo_dir / "setup.cfg").exists():
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
        project_failed = project_failed or editable.code != 0 or editable.timed_out
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
        probe = step("import_probe", [str(python), "-c", probe_code, *import_roots], cwd=repo_dir)
        record["import_probe_passed"] = probe.code == 0 and not probe.timed_out
        project_failed = project_failed or probe.code != 0 or probe.timed_out

    record["status"] = "partial" if project_failed else "ready"
    record["env_path"] = str(venv_dir)
    return env_updates, record


def build_request(instance: dict[str, Any], args: argparse.Namespace) -> str:
    instance_id = required_field(instance, "instance_id")
    problem = required_field(instance, "problem_statement")
    prefix = args.request_prefix.strip()
    if prefix:
        return f"{prefix}\n\nSWE-bench instance: {instance_id}\n\n{problem}".strip()
    return (
        f"SWE-bench instance: {instance_id}\n\n"
        f"{problem}\n\n"
        "Fix the repository behavior described above. Do not read or infer the gold patch; "
        "do not change tests merely to hide the failure."
    )


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
        "--request",
        build_request(instance, args),
    ]
    if args.providers:
        cmd[1:1] = ["--providers", str(Path(args.providers).resolve())]
    result = run_cmd(cmd, cwd=repo_dir, env=env, timeout=args.codrax_timeout)
    (inst_dir / "codrax.out").write_text(result.output, encoding="utf-8", errors="replace")
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


def report_failure_kind(report: dict[str, Any]) -> str:
    failure_kind = str(report.get("failure_kind") or "").strip()
    if failure_kind:
        return failure_kind
    if report_verification_status(report) == "unavailable" and report.get("no_tests_runners"):
        return "no_tests"
    return ""


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


def export_patch_between(repo_dir: Path, base: str, head: str, include_test_patches: bool) -> tuple[str, list[str]]:
    names = run_cmd(["git", "diff", "--name-only", base, head], cwd=repo_dir, timeout=120)
    if names.code != 0:
        result = run_cmd(["git", "diff", "--binary", base, head], cwd=repo_dir, timeout=120)
        return (result.output if result.code == 0 else ""), []
    changed = [line.strip() for line in names.output.splitlines() if line.strip()]
    test_paths = [path for path in changed if is_test_patch_path(path)]
    dropped = [] if include_test_patches else test_paths
    selected = changed if include_test_patches else [path for path in changed if not is_test_patch_path(path)]
    if not selected:
        return "", dropped
    result = run_cmd(["git", "diff", "--binary", base, head, "--", *selected], cwd=repo_dir, timeout=120)
    if result.code == 0:
        return result.output, dropped
    result = run_cmd(["git", "diff", "--binary", base, head], cwd=repo_dir, timeout=120)
    return (result.output if result.code == 0 else ""), []


def export_patch(repo_dir: Path, base_commit: str, plan: dict[str, Any], include_test_patches: bool) -> tuple[str, str, list[str]]:
    plan_id = str(plan.get("id") or "").strip()
    applied_sha = str(plan.get("applied_commit_sha") or "").strip()
    worktree = str(plan.get("worktree_path") or "").strip()
    commit = ""
    if commit_exists(repo_dir, applied_sha):
        commit = applied_sha
    elif plan_id and commit_exists(repo_dir, f"refs/codrax/applied/{plan_id}"):
        commit = f"refs/codrax/applied/{plan_id}"
    if commit:
        patch, dropped = export_patch_between(repo_dir, base_commit, commit, include_test_patches)
        return patch, commit, dropped
    if worktree and Path(worktree).is_dir():
        patch, dropped = export_patch_between(Path(worktree), base_commit, "HEAD", include_test_patches)
        return patch, "worktree:HEAD", dropped
    return "", "", []


def write_jsonl(path: Path, rows: Iterable[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    with tmp.open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(json.dumps(row, ensure_ascii=False, sort_keys=True))
            handle.write("\n")
    tmp.replace(path)


def process_instance(instance: dict[str, Any], args: argparse.Namespace) -> tuple[dict[str, Any], dict[str, Any]]:
    instance_id = required_field(instance, "instance_id")
    inst_dir = Path(args.workdir).resolve() / "instances" / safe_id(instance_id)
    repo_dir = inst_dir / "repo"
    inst_dir.mkdir(parents=True, exist_ok=True)
    (inst_dir / "instance.json").write_text(json.dumps(instance, ensure_ascii=False, indent=2, sort_keys=True), encoding="utf-8")
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
        base = checkout_instance(instance, mirror, repo_dir, args)
        result["base_commit_resolved"] = base
        env_updates, env_record = prepare_python_env(repo_dir, inst_dir, args)
        result["env_prepare"] = env_record
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
        report = load_report_for_plan(plan_path)
        if report:
            result["report_path"] = str(plan_path.with_name(plan_path.stem + ".report.json")) if plan_path else ""
            result["verify_status"] = report_verification_status(report)
            result["verify_passed"] = bool(report.get("passed"))
            result["verify_failure_kind"] = report_failure_kind(report)
            result["verify_summary"] = str(report.get("failure_summary") or "")
            result["verify_no_tests_runners"] = report.get("no_tests_runners") or []
            result["verify_test_count"] = len(report.get("test_results") or [])
        patch, source, dropped_test_paths = export_patch(repo_dir, base, plan, bool(args.include_test_patches))
        prediction["model_patch"] = patch
        result["patch_source"] = source
        result["dropped_test_patch_paths"] = dropped_test_paths
        result["patch_bytes"] = len(patch.encode("utf-8"))
        result["status"] = "predicted" if patch.strip() else "empty_patch"
    except Exception as exc:  # keep batch runs moving; official harness can score empty patches.
        result["status"] = "error"
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
    parser.add_argument("--codrax-bin", default=str(ROOT / "codrax"))
    parser.add_argument("--settings", default=str(DEFAULT_SETTINGS))
    parser.add_argument("--providers", help="Optional providers.yaml path forwarded to Codrax")
    parser.add_argument("--model-name", default="codrax")
    parser.add_argument("--max-steps", type=int, default=50)
    parser.add_argument("--codrax-timeout", type=int, default=1800)
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
    parser.add_argument("--env-prepare-timeout", type=int, default=600, help="Timeout in seconds per Python environment setup command")
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

    predictions: list[dict[str, Any]] = []
    results: list[dict[str, Any]] = []
    for index, instance in enumerate(instances, 1):
        instance_id = str(instance.get("instance_id") or "")
        print(f"[{index}/{len(instances)}] {instance_id}", file=sys.stderr)
        prediction, result = process_instance(instance, args)
        predictions.append(prediction)
        results.append(result)
        write_jsonl(predictions_path, predictions)
        write_jsonl(results_path, results)
        print(f"  status={result.get('status')} patch_bytes={result.get('patch_bytes', 0)}", file=sys.stderr)

    print(f"predictions: {predictions_path}", file=sys.stderr)
    print(f"results: {results_path}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
