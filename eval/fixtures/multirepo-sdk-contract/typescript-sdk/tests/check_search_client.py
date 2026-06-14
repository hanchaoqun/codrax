from pathlib import Path
import re
import sys


src = Path("src/client.ts").read_text()


def fail(message: str) -> None:
    print(message, file=sys.stderr)
    raise SystemExit(1)


if "/v1/memories/search" in src:
    fail("stale legacy search path is still present")
if re.search(r'method\s*:\s*["\']GET["\']', src):
    fail("textSearch still uses GET")
if "/v1/search" not in src:
    fail("textSearch must call /v1/search")
if not re.search(r'method\s*:\s*["\']POST["\']', src):
    fail("textSearch must use POST")
if "URLSearchParams" in src:
    fail("textSearch should send a JSON body, not query parameters")
if "JSON.stringify" not in src:
    fail("textSearch must serialize a JSON request body")
for field in ("query", "limit"):
    if field not in src:
        fail(f"missing body field {field}")

print("typescript text search contract ok")
