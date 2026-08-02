#!/usr/bin/env python3
"""Behavioral oracle for the napi-rs generated loader force-wasi regression."""
from pathlib import Path
import re
import sys

root = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else Path(__file__).resolve().parents[1]
source_path = root / "cli/src/api/templates/js-binding.ts"
tests_path = root / "tests/js-binding.test.ts"


def strip_comments(text: str) -> str:
    text = re.sub(r"//[^\n]*", "", text)
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
    return text


source = strip_comments(source_path.read_text())
tests = strip_comments(tests_path.read_text() if tests_path.exists() else "")
errors = []

template_match = re.search(r"\breturn\s*`(?P<body>.*)`\s*}\s*$", source, flags=re.S)
if template_match:
    rendered_loader = template_match.group("body")
else:
    rendered_loader = ""
    errors.append("renderNativeBinding must return a generated loader template")

if not re.search(r"NAPI_RS_FORCE_WASI\s*===\s*['\"]true['\"]", rendered_loader):
    errors.append("force-wasi handling must explicitly accept the true value")
if not re.search(r"NAPI_RS_FORCE_WASI\s*===\s*['\"]error['\"]", rendered_loader):
    errors.append("force-wasi handling must explicitly accept the error value")

truthy_env_condition = re.compile(
    r"if\s*\((?=[^)]*process\.env\.NAPI_RS_FORCE_WASI)(?![^)]*===)[^)]*\)",
    flags=re.S,
)
if truthy_env_condition.search(rendered_loader):
    errors.append("loader branches must not use raw truthiness of NAPI_RS_FORCE_WASI")

force_branch = re.search(
    r"if\s*\(\s*!nativeBinding\s*\|\|\s*(?P<force>[^)]+)\)",
    rendered_loader,
    flags=re.S,
)
if not force_branch:
    errors.append("generated loader must retain native-missing fallback plus explicit force-wasi branching")
else:
    force_expr = force_branch.group("force").strip()
    inline_force = (
        "NAPI_RS_FORCE_WASI" in force_expr
        and re.search(r"===\s*['\"]true['\"]", force_expr)
        and re.search(r"===\s*['\"]error['\"]", force_expr)
    )
    if "NAPI_RS_FORCE_WASI" in force_expr and not inline_force:
        errors.append("generated force-wasi branch must compare the environment value to true/error")
    elif not inline_force:
        force_name = re.fullmatch(r"[A-Za-z_$][A-Za-z0-9_$]*", force_expr)
        declaration = None
        if force_name:
            declaration = re.search(
                rf"(?:const|let)\s+{re.escape(force_name.group(0))}\s*=\s*(?P<value>[^\n;]+)",
                rendered_loader,
            )
        if not declaration:
            errors.append(
                "force-wasi branch references state that is not declared inside the generated loader"
            )
        else:
            value_expr = declaration.group("value")
            if not (
                re.search(r"NAPI_RS_FORCE_WASI\s*===\s*['\"]true['\"]", value_expr)
                and re.search(r"NAPI_RS_FORCE_WASI\s*===\s*['\"]error['\"]", value_expr)
            ):
                errors.append("generated force-wasi state must accept exactly true/error forcing values")
            if declaration.start() > force_branch.start():
                errors.append("generated force-wasi state must be declared before it is used")

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

print("napi-rs generated-loader force-wasi scope and behavior checks passed")
