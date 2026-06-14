from pathlib import Path
import ast
import sys


src = Path("memoclaw/client.py").read_text()
tree = ast.parse(src)


def fail(message: str) -> None:
    print(message, file=sys.stderr)
    raise SystemExit(1)


if "/v1/memories/search" in src:
    fail("stale legacy search path is still present")
if "urlencode" in src:
    fail("text_search should send a JSON body, not query parameters")
if "/v1/search" not in src:
    fail("text_search must call /v1/search")

methods = {}
for node in ast.walk(tree):
    if isinstance(node, ast.ClassDef) and node.name in {"MemoryClient", "AsyncMemoryClient"}:
        for item in node.body:
            if isinstance(item, (ast.FunctionDef, ast.AsyncFunctionDef)) and item.name == "text_search":
                methods[node.name] = item

for class_name in ("MemoryClient", "AsyncMemoryClient"):
    fn = methods.get(class_name)
    if fn is None:
        fail(f"{class_name}.text_search missing")
    body_src = ast.get_source_segment(src, fn) or ""
    if '"POST"' not in body_src and "'POST'" not in body_src:
        fail(f"{class_name}.text_search must use POST")
    if "json=" not in body_src:
        fail(f"{class_name}.text_search must pass a json body")
    for field in ("query", "limit"):
        if field not in body_src:
            fail(f"{class_name}.text_search missing body field {field}")

print("python text search contract ok")
