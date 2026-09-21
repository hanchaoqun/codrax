#!/usr/bin/env python3
"""Read the final renderer-owned audit receipt; never infer ownership by titles."""
import hashlib
import json
from pathlib import Path
import re
import sys


LOG_PREFIX = r"^\d{4}-\d\d-\d\dT\S+\s+INFO \[output_dump\] "
MARKDOWN_WRITE = re.compile(LOG_PREFIX + r"wrote (?!explicit report )(.+\.md) \((\d+) bytes\)$")
SURFACE_RECEIPT = re.compile(LOG_PREFIX + r"answer surfaces path=(.+) sha256=([0-9a-f]{64})$")
DEFAULT_WRITE_FAILURE = re.compile(
    r"^\d{4}-\d\d-\d\dT\S+\s+WARN \[output_dump\] "
    r"(?:mkdir (?!.* for explicit report ).+|write (?!explicit report ).+\.md) failed: .*$")


def load(log, prefix):
    lines = Path(log).read_text(encoding="utf-8", errors="replace").splitlines()
    writes = [(i, m.groups()) for i, line in enumerate(lines) if (m := MARKDOWN_WRITE.match(line))]
    last_write_index = writes[-1][0] if writes else -1
    if any(DEFAULT_WRITE_FAILURE.match(line) for line in lines[last_write_index + 1:]):
        # The latest finalization may fail before a Markdown success event
        # exists at all. Its precise default-output warning is still a barrier
        # against reviving an earlier successful answer's ownership receipt.
        raise ValueError("transcript_write_failed")
    if not writes:
        # A receipt is not itself proof that its transcript reached the write
        # boundary. Explicit report copies have no ownership receipt of their
        # own; HTML and root-cause artifacts are separate delivery surfaces.
        if any(SURFACE_RECEIPT.match(line) for line in lines):
            raise ValueError("transcript_write_missing")
        raise ValueError("receipt_missing")
    write_index, (markdown_path, markdown_size) = writes[-1]
    # writeDefaultDump logs successful Markdown before attempting its receipt.
    # A newer Markdown with a failed/missing receipt must never borrow an older
    # success, including when the same millisecond path has been overwritten.
    receipts = [m.groups() for line in lines[write_index + 1:] if (m := SURFACE_RECEIPT.match(line))]
    if not receipts:
        raise ValueError("receipt_missing")
    path, expected_digest = receipts[-1]
    path = Path(path)
    if not str(path).endswith(".answer-surfaces.json"):
        raise ValueError("receipt_path_invalid")
    if str(path) != markdown_path[:-len(".md")] + ".answer-surfaces.json":
        raise ValueError("receipt_binding_invalid")
    raw = path.read_bytes()
    if hashlib.sha256(raw).hexdigest() != expected_digest:
        raise ValueError("receipt_mismatch")
    obj = json.loads(raw)
    if not isinstance(obj, dict) or type(obj.get("schema_version")) is not int or obj["schema_version"] != 1:
        raise ValueError("receipt_schema_invalid")
    if obj.get("status") != "available":
        raise ValueError("ownership_unavailable")
    transcript = Path(markdown_path).read_bytes()
    if len(transcript) != int(markdown_size) or hashlib.sha256(transcript).hexdigest() != obj.get("markdown_sha256"):
        raise ValueError("transcript_mismatch")
    if not isinstance(obj.get("answer_sha256"), str) or not re.fullmatch(r"[0-9a-f]{64}", obj["answer_sha256"]):
        raise ValueError("answer_binding_invalid")
    for name in ("primary", "principal"):
        if not isinstance(obj.get(name), str):
            raise ValueError("surface_invalid")
    # Persist the verified receipt and its transcript for post-run human audit.
    Path(prefix + ".answer-surfaces.json").write_bytes(raw)
    Path(prefix + ".answer-transcript.md").write_bytes(transcript)
    for name in ("primary", "principal"):
        Path(prefix + "." + name + ".md").write_text(obj[name], encoding="utf-8")


if __name__ == "__main__":
    try:
        load(sys.argv[1], sys.argv[2])
    except (OSError, ValueError, TypeError, KeyError, IndexError) as exc:
        reason = str(exc) if isinstance(exc, ValueError) and str(exc) in {
            "receipt_missing", "receipt_path_invalid", "receipt_schema_invalid", "ownership_unavailable",
            "transcript_mismatch", "receipt_mismatch", "answer_binding_invalid", "surface_invalid",
            "transcript_write_missing", "receipt_binding_invalid",
            "transcript_write_failed",
        } else "receipt_unreadable"
        print("answer_surface_" + reason)
        sys.exit(1)
