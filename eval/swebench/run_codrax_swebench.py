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

    upgraded = step("upgrade_packaging", [str(python), "-m", "pip", "install", "--upgrade", "pip", "setuptools", "wheel"])
    if upgraded.code != 0 or upgraded.timed_out:
        record["status"] = "failed_packaging"
        record["env_path"] = str(venv_dir)
        return env_updates, record

    pytest = step("install_pytest", [str(python), "-m", "pip", "install", "pytest<9", "pytest-json-report"])
    if pytest.code != 0 or pytest.timed_out:
        record["status"] = "failed_pytest"
        record["env_path"] = str(venv_dir)
        return env_updates, record

    project_failed = False
    requirements = repo_dir / "requirements.txt"
    if requirements.exists() and requirements.stat().st_size < 1_000_000:
        req = step("install_requirements", [str(python), "-m", "pip", "install", "-r", str(requirements)], cwd=repo_dir)
        project_failed = project_failed or req.code != 0 or req.timed_out
    if (repo_dir / "pyproject.toml").exists() or (repo_dir / "setup.py").exists() or (repo_dir / "setup.cfg").exists():
        editable = step("install_editable", [str(python), "-m", "pip", "install", "-e", "."], cwd=repo_dir)
        project_failed = project_failed or editable.code != 0 or editable.timed_out

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


def commit_exists(repo_dir: Path, rev: str) -> bool:
    if not rev:
        return False
    result = run_cmd(["git", "cat-file", "-e", f"{rev}^{{commit}}"], cwd=repo_dir, timeout=60)
    return result.code == 0


def export_patch(repo_dir: Path, base_commit: str, plan: dict[str, Any]) -> tuple[str, str]:
    plan_id = str(plan.get("id") or "").strip()
    applied_sha = str(plan.get("applied_commit_sha") or "").strip()
    worktree = str(plan.get("worktree_path") or "").strip()
    commit = ""
    if commit_exists(repo_dir, applied_sha):
        commit = applied_sha
    elif plan_id and commit_exists(repo_dir, f"refs/codrax/applied/{plan_id}"):
        commit = f"refs/codrax/applied/{plan_id}"
    if commit:
        result = run_cmd(["git", "diff", "--binary", base_commit, commit], cwd=repo_dir, timeout=120)
        if result.code == 0:
            return result.output, commit
    if worktree and Path(worktree).is_dir():
        result = run_cmd(["git", "diff", "--binary", base_commit, "HEAD"], cwd=Path(worktree), timeout=120)
        if result.code == 0:
            return result.output, "worktree:HEAD"
    return "", ""


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
        patch, source = export_patch(repo_dir, base, plan)
        prediction["model_patch"] = patch
        result["patch_source"] = source
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
