#!/usr/bin/env python3
"""Behavioral oracle without node: asserts the parse mapping no longer
sends missing ISO components through bare Number(), and that the node
test pins the upstream PT1H regression."""
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1]
source = (root / "src/duration.js").read_text()
tests = (root / "tests/duration.test.js").read_text()

# Strip // line comments so prose cannot satisfy code checks.
code = re.sub(r"//[^\n]*", "", source)

errors = []

bare = re.search(r"map\s*\(\s*\(?value\)?\s*=>\s*Number\s*\(\s*value\s*\)\s*\)", code)
guarded = re.search(r"(!=\s*null|!==\s*undefined|===\s*undefined|\?\?|\|\|\s*0|value\s*==\s*null)", code)
numeric_map = re.search(r"map\s*\([^)]*=>[^;\n]*Number\s*\(", code)
if bare:
    errors.append("missing ISO components still run through bare Number(value) and become NaN")
if not guarded:
    errors.append("parseIso must default missing components to 0 instead of producing NaN")
if not numeric_map:
    errors.append("parseIso must keep converting present ISO components to numbers after applying the missing-component default")
if "PT1H" not in tests:
    errors.append("regression test for a partial duration (e.g. PT1H) is required")
if "0000-00-00T01:00:00" not in tests:
    errors.append("regression must pin the zeroed rendering of PT1H")
if "deepStrictEqual(partial" not in tests:
    errors.append("regression must assert parseIso('PT1H') numeric fields, not only formatted output")

if errors:
    for error in errors:
        print(error, file=sys.stderr)
    sys.exit(1)
print("duration missing-component checks passed")
