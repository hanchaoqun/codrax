#!/usr/bin/env python3
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1]
source = (root / "src/main/java/org/apache/commons/lang3/RandomStringUtils.java").read_text()
tests = (root / "src/test/java/org/apache/commons/lang3/RandomStringUtilsTest.java").read_text()

errors = []

if not re.search(r"end\s*<=\s*0x7[fF]", source):
    errors.append("ASCII optimizations must be gated by end <= 0x7f")

lines = source.splitlines()
for idx, line in enumerate(lines):
    if "ALPHANUMERICAL_CHARS" in line and "return new String" in line:
        condition = lines[idx - 1] if idx > 0 else ""
        if re.search(r"end\s*>\s*0x7[fF]", condition):
            errors.append("ASCII fast path is gated with end > 0x7f instead of end <= 0x7f")
        if not re.search(r"end\s*<=\s*0x7[fF]", condition):
            errors.append("ASCII fast path must explicitly require end <= 0x7f")

if not re.search(r"0x(?:4e00|370|400)", tests, re.IGNORECASE):
    errors.append("missing non-ASCII letter regression test")

if not re.search(r"0x0?66[0a]", tests, re.IGNORECASE):
    errors.append("missing non-ASCII digit regression test")

if errors:
    for error in errors:
        print(error, file=sys.stderr)
    sys.exit(1)

print("RandomStringUtils non-ASCII regression checks passed")
