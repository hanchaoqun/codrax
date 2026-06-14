#!/usr/bin/env python3
"""Behavioral oracle for the chrono Duration minimum-bound regression."""
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1]
source_path = root / "src/duration.rs"
tests_path = root / "tests/duration_min.rs"
source = source_path.read_text()
tests = tests_path.read_text() if tests_path.exists() else ""


def strip_comments(text: str) -> str:
    text = re.sub(r"//[^\n]*", "", text)
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
    return text


def extract_function_body(text: str, name: str):
    match = re.search(
        rf"fn\s+{re.escape(name)}\s*\([^)]*\)\s*->\s*(?P<ret>[^\{{]+)\{{",
        text,
        flags=re.S,
    )
    if not match:
        return "", ""
    start = match.end() - 1
    depth = 0
    for index in range(start, len(text)):
        char = text[index]
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return match.group("ret").strip(), text[start + 1:index]
    return match.group("ret").strip(), ""


def test_brace_errors(text: str):
    errors = []
    depth = 0
    for line_no, line in enumerate(text.splitlines(), 1):
        stripped = line.strip()
        if stripped.startswith("#[test]") and depth != 0:
            errors.append(f"#[test] at line {line_no} is nested inside another item")
        depth += line.count("{") - line.count("}")
        if depth < 0:
            errors.append(f"extra closing brace before line {line_no}")
            depth = 0
    if depth != 0:
        errors.append("test file braces are unbalanced")
    return errors


code = strip_comments(source)
test_code = strip_comments(tests)
errors = []

try_return, try_body = extract_function_body(code, "try_milliseconds")
if not try_body:
    errors.append("Duration::try_milliseconds(milliseconds: i64) -> Option<Duration> is required")
elif not re.search(r"Option\s*<\s*Duration\s*>", try_return):
    errors.append("Duration::try_milliseconds must return Option<Duration>, matching chrono's try_* constructor family")

if try_body:
    if not re.search(r"Duration\s*\{(?=[^}]*\bsecs\b)(?=[^}]*\bnanos\b)[^}]*\}", try_body, flags=re.S):
        errors.append("try_milliseconds must construct a candidate Duration from the millisecond input")
    if "None" not in try_body or "Some" not in try_body:
        errors.append("try_milliseconds must return None for out-of-range input and Some for valid input")
    lower_guard = re.search(r"milliseconds\s*<\s*-i64::MAX|MIN_MILLIS\s*:\s*i64\s*=\s*-i64::MAX", code)
    upper_guard = re.search(r"milliseconds\s*>\s*i64::MAX|MAX_MILLIS\s*:\s*i64\s*=\s*i64::MAX", code)
    candidate_guard = re.search(r"candidate\s*<\s*MIN|MIN\s*>\s*candidate|candidate\s*>\s*MAX|MAX\s*<\s*candidate", try_body)
    if not (lower_guard or candidate_guard):
        errors.append("try_milliseconds must enforce the lower bound using -i64::MAX or MIN/MAX")
    if not (upper_guard or candidate_guard):
        errors.append("try_milliseconds must enforce the upper bound using i64::MAX or MIN/MAX")

if re.search(r"const\s+MIN\s*:\s*Duration\s*=\s*Duration::milliseconds", code):
    errors.append("MIN must remain a direct boundary constant, not call Duration::milliseconds recursively")
if re.search(r"const\s+MAX\s*:\s*Duration\s*=\s*Duration::milliseconds", code):
    errors.append("MAX must remain a direct boundary constant, not call Duration::milliseconds recursively")
if re.search(r"MIN_MILLIS\s*:\s*i64\s*=\s*i64::MIN\s*/", code):
    errors.append("constructor lower bound must be -i64::MAX milliseconds, not i64::MIN divided by a scale")

milliseconds_return, milliseconds_body = extract_function_body(code, "milliseconds")
if not milliseconds_body:
    errors.append("Duration::milliseconds(milliseconds: i64) -> Duration is required")
else:
    calls_try = re.search(r"try_milliseconds\s*\(\s*milliseconds\s*\)", milliseconds_body)
    panics = re.search(r"\.expect\s*\(|panic!\s*\(|unwrap\s*\(", milliseconds_body)
    if not calls_try or not panics:
        errors.append("milliseconds must delegate to try_milliseconds and panic/expect on invalid input")
    if re.search(r"Duration\s*\{[^}]*secs\s*:", milliseconds_body, flags=re.S) and not calls_try:
        errors.append("milliseconds must not bypass the fallible constructor")

if "i64::MIN" not in test_code:
    errors.append("regression tests must cover i64::MIN being rejected")
if not re.search(r"try_milliseconds\s*\(\s*i64::MIN\s*\)[^\n;]*(?:is_none|None)", test_code, flags=re.S):
    errors.append("regression tests must assert try_milliseconds(i64::MIN) is rejected")
if "-i64::MAX" not in test_code:
    errors.append("regression tests must keep -i64::MAX as the in-range lower boundary")
if "i64::MAX" not in test_code:
    errors.append("regression tests must keep i64::MAX as the in-range upper boundary")

errors.extend(test_brace_errors(test_code))

if errors:
    for error in errors:
        print(error, file=sys.stderr)
    sys.exit(1)
print("chrono duration minimum-bound checks passed")
