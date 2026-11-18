#!/usr/bin/env python3
"""Shared primitives for the HMC batch tools (standard library only).

Every tool in this directory obeys the same four rules:

* credentials are never read, hashed or copied (``providers*`` / ``.env*``
  path components that are not tracked source files are refused);
* evidence files are created exclusively (``O_EXCL``) and never rewritten;
* symlinks are refused wherever bytes are hashed or copied;
* nothing here calls a model, reads a user question for a decision, or
  writes inside a previous batch's root.
"""
import datetime
import hashlib
import json
import os
import re
import stat
import subprocess
import sys

sys.dont_write_bytecode = True  # never drop __pycache__ into the repository

CREDENTIAL_PART = re.compile(r"^(providers(?:\.|$)|\.env(?:\.|$))", re.I)

# Source-input rule of the frozen manifest. Same shape the campaign used from
# HMC350 on (sealed 0361-codrax-hmc350-batch-v1.rb `inputs`), kept as data so
# a reviewer can diff it.
INPUT_PREFIXES = ("internal/", "cmd/", "eval/cases/", "eval/fixtures/", "eval/lib/",
                  "third_party/", "examples/", "config/", "scripts/")
INPUT_SUFFIXES = (".go", ".sh", ".py", ".yaml", ".yml", ".yaml.example", ".yml.example",
                  ".json", ".toml")
INPUT_EXACT = ("Makefile", "go.mod", "go.sum")
INPUT_EXCLUDED_PREFIXES = ("docs/", "eval/results/", ".codrax/")

# Environment that must not leak from the operator shell into a frozen run.
# (sealed HMC350 `clean_env`.) Exact names are run.sh case variables.
CASE_ENV_EXACT = frozenset("""LOG LOG_FILE LOG_FILES HTRACE HTRACE_FILE HTRACE_STDIN_FILE ATRACE
ATRACE_FILE MODE FIXTURE DATA_FIXTURE MULTIREPO MULTIREPO_WRITE_ROOT FOCUS CAP SETTINGS
COMMANDLESS_APPLY ALLOW_UNVERIFIED_APPLY ALLOW_DEGRADED_READ_ANSWER REQUIRED_EXECUTABLES
PLAN_EXPECT_REGEX POST_APPLY_FILE POST_APPLY_FILES MIN_OUTPUT_CHARS EVAL_RESULTS_ROOT BASH_ENV
ENV""".split())
CASE_ENV_PREFIXES = ("CODRAX_", "LIVE_", "EXPECT_", "MAX_", "ADVISORY_MAX_")


class ToolError(Exception):
    """A refused or failed operation; the message is the whole diagnosis."""


def die(message, code=2):
    sys.stderr.write("error: %s\n" % message)
    sys.exit(code)


def now_utc():
    return datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z")


def local_utc_offset_seconds():
    """Offset of the local clock, the one the product logger stamps with."""
    return int(datetime.datetime.now().astimezone().utcoffset().total_seconds())


def credential_path(path):
    return any(CREDENTIAL_PART.match(part) for part in str(path).split(os.sep))


def regular_file(path):
    """lstat-based: a symlink is never a regular file here."""
    try:
        return stat.S_ISREG(os.lstat(path).st_mode)
    except OSError:
        return False


def sha256_file(path, allow_credential_name=False):
    """SHA-256 of a regular, non-symlink, non-credential file."""
    if not allow_credential_name and (credential_path(path) or credential_path(os.path.realpath(path))):
        raise ToolError("credential path forbidden: %s" % path)
    if not regular_file(path):
        raise ToolError("regular non-symlink file required: %s" % path)
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for block in iter(lambda: handle.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def sha256_bytes(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical_json(value):
    """Compact UTF-8 JSON in insertion order.

    Byte-compatible with Ruby ``JSON.generate`` for maps of plain strings, so
    ``source_sha256`` values are comparable with the sealed HMC350-375 receipts.
    """
    return json.dumps(value, separators=(",", ":"), ensure_ascii=False).encode("utf-8")


def load_json(path):
    with open(path, "rb") as handle:
        return json.loads(handle.read().decode("utf-8"))


def write_exclusive(path, raw, mode=0o600):
    """Create ``path`` with ``raw``; refuse to replace anything that exists."""
    if credential_path(path):
        raise ToolError("credential output forbidden: %s" % path)
    try:
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    except FileExistsError:
        raise ToolError("refusing to overwrite existing evidence: %s" % path)
    with os.fdopen(fd, "wb") as handle:
        handle.write(raw)
        handle.flush()
        os.fsync(handle.fileno())


def save_json(path, value, mode=0o600):
    write_exclusive(path, (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8"), mode)


def copy_exclusive(source, target, mode=0o400):
    """Byte copy with exclusive create; returns (sha256, bytes) of the copy."""
    if credential_path(source) or credential_path(os.path.realpath(source)):
        raise ToolError("credential path forbidden: %s" % source)
    if not regular_file(source):
        raise ToolError("regular non-symlink file required: %s" % source)
    try:
        fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        raise ToolError("refusing to overwrite existing evidence: %s" % target)
    digest = hashlib.sha256()
    size = 0
    with os.fdopen(fd, "wb") as out, open(source, "rb") as src:
        for block in iter(lambda: src.read(1 << 20), b""):
            out.write(block)
            digest.update(block)
            size += len(block)
        out.flush()
        os.fsync(out.fileno())
    os.chmod(target, mode)
    return digest.hexdigest(), size


def tree(root, prune=(".git",)):
    """Sorted realpaths of regular files under ``root``.

    Symlinks and credential-named entries are an error, not skipped: an
    archive that silently omits something is worse than one that stops.
    """
    if os.path.islink(root) or not os.path.isdir(root):
        raise ToolError("directory (not a symlink) required: %s" % root)
    found = []
    for base, dirs, files in os.walk(root):
        dirs[:] = sorted(d for d in dirs if d not in prune)
        for name in dirs + files:
            path = os.path.join(base, name)
            if os.path.islink(path):
                raise ToolError("symlink inside evidence tree: %s" % path)
            if credential_path(os.path.relpath(path, root)):
                raise ToolError("credential-named entry inside evidence tree: %s" % path)
        found.extend(os.path.realpath(os.path.join(base, name)) for name in files)
    return sorted(found)


def git(repo, *args):
    done = subprocess.run(["git", "-C", repo] + list(args), stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if done.returncode != 0:
        raise ToolError("git %s failed: %s" % (" ".join(args), done.stderr.decode("utf-8", "replace").strip()))
    return done.stdout.decode("utf-8")


def default_repo():
    """The repository this tool lives in (eval/hmc_tools/), else the CWD's."""
    here = os.path.dirname(os.path.realpath(__file__))
    candidate = os.path.dirname(os.path.dirname(here))
    if os.path.isfile(os.path.join(candidate, "eval", "run.sh")) and os.path.basename(os.path.dirname(here)) == "eval":
        return candidate
    try:
        return git(os.getcwd(), "rev-parse", "--show-toplevel").strip()
    except (ToolError, OSError):
        return os.getcwd()


def resolve_repo(value):
    repo = os.path.realpath(value or default_repo())
    if not os.path.isdir(os.path.join(repo, ".git")) and not os.path.isfile(os.path.join(repo, ".git")):
        raise ToolError("not a git working tree: %s" % repo)
    return repo


def is_source_input(rel):
    if rel.startswith(INPUT_EXCLUDED_PREFIXES):
        return False
    return rel.startswith(INPUT_PREFIXES) or rel.endswith(INPUT_SUFFIXES) or rel in INPUT_EXACT


def source_inputs(repo):
    """Frozen source inputs: ``{repo-relative path: sha256}``, sorted by path.

    Returns ``(inputs, legacy_name_excluded)``. Tracked and untracked-but-not-
    ignored files are candidates (ignored files such as providers.yaml are
    never listed by git here). A *tracked* file is source even when its name
    looks like a credential (internal/config/providers.go); the HMC350-375
    Ruby helper dropped those three files by name and therefore never froze
    them. ``legacy_name_excluded`` lists them so old and new counts reconcile.
    An *untracked* credential-named file is refused.
    """
    tracked = set(git(repo, "ls-files", "-z", "--cached").split("\0")) - {""}
    others = set(git(repo, "ls-files", "-z", "--others", "--exclude-standard").split("\0")) - {""}
    names = tracked | others
    inputs, legacy = {}, []
    for rel in sorted(names):
        if not is_source_input(rel):
            continue
        path = os.path.join(repo, rel)
        if not regular_file(path):
            continue  # deleted in the working tree, a submodule, or a symlink
        named_like_credential = credential_path(rel)
        if named_like_credential and rel not in tracked:
            raise ToolError("untracked credential-named file in the source set: %s" % rel)
        if named_like_credential:
            legacy.append(rel)
        inputs[rel] = sha256_file(path, allow_credential_name=True)
    return inputs, legacy


def source_digest(inputs):
    return sha256_bytes(canonical_json(inputs))


def clean_env(base=None):
    """Operator environment without case variables or CODRAX_/EXPECT_/... names."""
    env = dict(os.environ if base is None else base)
    for key in list(env):
        if key in CASE_ENV_EXACT or key.startswith(CASE_ENV_PREFIXES):
            del env[key]
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    return env


def thousands(number):
    return format(number, ",")
