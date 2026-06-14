#!/usr/bin/env python3
"""Behavioral oracle for the PyO3 iterator skip overflow regression."""
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1]
source_paths = [
    root / "src/types/list.rs",
    root / "src/types/tuple.rs",
]
test_path = root / "tests/iterators.rs"


def strip_comments(text: str) -> str:
    text = re.sub(r"//[^\n]*", "", text)
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
    return text


def extract_method_body(text: str, name: str) -> str:
    match = re.search(rf"fn\s+{re.escape(name)}\s*\([^)]*\)\s*->[^\{{]+\{{", text, flags=re.S)
    if not match:
        return ""
    start = match.end() - 1
    depth = 0
    for index in range(start, len(text)):
        char = text[index]
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return text[start + 1:index]
    return ""


errors = []

for source_path in source_paths:
    raw = source_path.read_text()
    code = strip_comments(raw)
    label = source_path.name
    nth_body = extract_method_body(code, "nth")
    nth_back_body = extract_method_body(code, "nth_back")

    if not nth_body:
        errors.append(f"{label}: nth implementation is required")
    else:
        if "checked_add" not in nth_body:
            errors.append(f"{label}: nth must use checked addition for skip arithmetic")
        if re.search(r"checked_add\s*\(\s*n\s*\)\s*\?", nth_body):
            errors.append(f"{label}: nth must exhaust the iterator when checked_add overflows, not return early with ?")
        exhausts_forward = (
            re.search(r"self\.index\s*=\s*(?:current_length|self\.length|min_length)", nth_body)
            or re.search(r"self\.length\s*=\s*self\.index", nth_body)
        )
        if not exhausts_forward:
            errors.append(f"{label}: nth must exhaust the iterator after a past-end skip")

    if not nth_back_body:
        errors.append(f"{label}: nth_back implementation is required")
    else:
        if "checked_sub" not in nth_back_body:
            errors.append(f"{label}: nth_back must use checked subtraction for reverse skip arithmetic")
        if re.search(r"checked_sub\s*\(\s*(?:n\s*\+\s*1|1\s*\+\s*n)\s*\)", nth_back_body):
            errors.append(f"{label}: nth_back must not compute n + 1 before checked_sub; n can be usize::MAX")
        if re.search(r"current_length\s*-\s*n\s*-\s*1", nth_back_body):
            errors.append(f"{label}: nth_back must not use raw current_length - n - 1 subtraction")
        if not re.search(r"self\.length\s*=\s*self\.index", nth_back_body):
            errors.append(f"{label}: nth_back must exhaust the reverse iterator after a past-end skip")

tests = strip_comments(test_path.read_text() if test_path.exists() else "")
if "usize::MAX" not in tests:
    errors.append("regression tests must include usize::MAX")
if "nth_back" not in tests:
    errors.append("regression tests must cover nth_back")
if not (re.search(r"next\s*\(\s*\)\s*\.\s*is_none\s*\(", tests) or re.search(r"assert_eq!\s*\([^;\n]*next\s*\(\s*\)[^;\n]*None", tests)):
    errors.append("regression tests must assert next() is exhausted after a past-end nth")
if not (re.search(r"next_back\s*\(\s*\)\s*\.\s*is_none\s*\(", tests) or re.search(r"assert_eq!\s*\([^;\n]*next_back\s*\(\s*\)[^;\n]*None", tests)):
    errors.append("regression tests must assert next_back() is exhausted after a past-end nth_back")
if re.search(r"nth_back\s*\(\s*0\s*\).*?next\s*\(\s*\)[^;\n]*None", tests, flags=re.S):
    errors.append("regression tests must not claim nth_back(0) exhausts remaining forward elements")
if re.search(r"nth\s*\(\s*0\s*\).*?next_back\s*\(\s*\)[^;\n]*None", tests, flags=re.S):
    errors.append("regression tests must not claim nth(0) exhausts remaining reverse elements")

if errors:
    for error in errors:
        print(error, file=sys.stderr)
    sys.exit(1)

print("PyO3 iterator skip regression checks passed")
