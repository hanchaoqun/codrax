# stub-repo

Sealed minimal repository fixture (F4, ledger §29.55.5).

Trace-only eval cases ("analyze this trace only, do not analyze code")
use this directory as their `--repo` root instead of the codrax repo
itself. It deliberately contains no application source code and no test
files, so repository greps and file reads cannot echo anything back
into the model's context.

Do not add source files, test files, or any evaluation-related strings
here. The empty `docs/` directory (held by `.gitkeep`) exists only so
the seeded scratch repo has a directory entry besides this file.
