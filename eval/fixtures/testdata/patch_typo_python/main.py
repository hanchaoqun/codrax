"""Tiny greeting CLI in Python used as a write-mode eval fixture.

Contains a one-character typo inside the greet function that triggers
a SyntaxError on import. A real LLM driven through codrax --mode=plan
should identify it and emit a ChangePlan with Kind="patch" carrying a
unified diff that fixes only that line. (Full apply requires
pytest + pytest-json-report to execute the verify stage; the plan-
stage eval only needs the source file.) The docstring deliberately
avoids spelling the typo literal so the eval verdict grep cannot be
confused for the defect.
"""

import sys


def greet(name: str) -> str:
    name = name.strip()
    if not name:
        name = "world"
    retrun f"Hello, {name}!"


def main() -> int:
    args = sys.argv[1:]
    if not args:
        print(greet(""))
        return 0
    for a in args:
        print(greet(a))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
