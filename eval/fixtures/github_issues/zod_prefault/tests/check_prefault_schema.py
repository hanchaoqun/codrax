#!/usr/bin/env python3
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1]
source = (root / "packages/zod/src/v4/core/to-json-schema.ts").read_text()
tests = (root / "packages/zod/src/v4/classic/tests/to-json-schema.test.ts").read_text()

errors = []

bare_prefault_truthiness = re.compile(
    r"ctx\.io\s*===\s*['\"]input['\"]\s*&&\s*result\.schema\._prefault\s*(?:[)\n{;]|$)"
)

if bare_prefault_truthiness.search(source):
    errors.append("implementation still uses truthiness for _prefault")

has_existence_check = (
    '"_prefault" in result.schema' in source
    or "'_prefault' in result.schema" in source
    or 'hasOwnProperty.call(result.schema, "_prefault")' in source
    or "result.schema._prefault !== undefined" in source
)
if not has_existence_check:
    errors.append("implementation must use a typed existence check for _prefault")

for snippet in ["prefault(false)", "prefault(0)", 'prefault("")']:
    if snippet not in tests:
        errors.append(f"missing regression test snippet: {snippet}")

for snippet in ["default: false", "default: 0", 'default: ""']:
    if snippet not in tests:
        errors.append(f"missing expected default snippet: {snippet}")

if errors:
    for error in errors:
        print(error, file=sys.stderr)
    sys.exit(1)

print("Zod falsy prefault regression checks passed")
