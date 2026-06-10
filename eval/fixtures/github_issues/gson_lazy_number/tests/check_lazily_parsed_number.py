#!/usr/bin/env python3
"""Source-level oracle (host has no Java runtime, mirroring the
commons-lang fixture): the production class must gain value-based
equals/hashCode without losing the existing surface."""
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1]
source = (root / "src/main/java/com/google/gson/internal/LazilyParsedNumber.java").read_text()

errors = []

equals_match = re.search(r"public\s+boolean\s+equals\s*\(", source)
hash_match = re.search(r"public\s+int\s+hashCode\s*\(", source)
if not equals_match:
    errors.append("LazilyParsedNumber must override equals(Object)")
if not hash_match:
    errors.append("LazilyParsedNumber must override hashCode()")

def method_window(match):
    # Bounded body window: enough for any reasonable implementation,
    # short enough not to bleed into later methods' uses of `value`.
    start = match.end()
    return source[start:start + 420]

if equals_match and "value" not in method_window(equals_match):
    errors.append("equals must compare the wrapped value")
if hash_match and "value" not in method_window(hash_match):
    errors.append("hashCode must derive from the wrapped value")
if "intValue" not in source or "toString" not in source:
    errors.append("existing Number surface must be preserved")

if errors:
    for error in errors:
        print(error, file=sys.stderr)
    sys.exit(1)
print("LazilyParsedNumber value-semantics checks passed")
