#!/usr/bin/env python3
"""Behavioral oracle for the napi-rs generated loader force-wasi regression."""
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1]
source_path = root / "cli/src/api/templates/js-binding.ts"
tests_path = root / "tests/js-binding.test.ts"


def strip_comments(text: str) -> str:
    text = re.sub(r"//[^\n]*", "", text)
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
    return text


source = strip_comments(source_path.read_text())
tests = strip_comments(tests_path.read_text() if tests_path.exists() else "")
errors = []

if not re.search(r"NAPI_RS_FORCE_WASI\s*===\s*['\"]true['\"]", source):
    errors.append("force-wasi handling must explicitly accept the true value")
if not re.search(r"NAPI_RS_FORCE_WASI\s*===\s*['\"]error['\"]", source):
    errors.append("force-wasi handling must explicitly accept the error value")
if not re.search(r"(?:const|let)\s+\w*forceWasi\w*\s*=", source, flags=re.I):
    errors.append("force-wasi state should be computed once as a boolean before loader branching")

truthy_env_condition = re.compile(
    r"if\s*\((?=[^)]*process\.env\.NAPI_RS_FORCE_WASI)(?![^)]*===)[^)]*\)",
    flags=re.S,
)
if truthy_env_condition.search(source):
    errors.append("loader branches must not use raw truthiness of NAPI_RS_FORCE_WASI")

for value in ["'false'", "'0'", "'true'", "'error'"]:
    if value not in tests and value.replace("'", '"') not in tests:
        errors.append(f"regression tests must cover NAPI_RS_FORCE_WASI={value}")

if not re.search(r"(?:doesNotMatch|strictEqual|equal)[^;\n]*(?:false|0)", tests):
    errors.append("regression tests must assert false/0 do not force the WASI branch")
if not re.search(r"(?:match|strictEqual|equal)[^;\n]*(?:true|error)", tests):
    errors.append("regression tests must assert true/error keep forcing the WASI branch")

if errors:
    for error in errors:
        print(error, file=sys.stderr)
    sys.exit(1)

print("napi-rs force-wasi environment checks passed")
