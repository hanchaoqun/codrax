# multirepo-basic — multi-repo eval fixture

Three sub-repos under one parent. Each is `git init`-ed at runtime by
`eval/run.sh`'s multi-repo bootstrap path (the seed checks in zero
`.git/` dirs into the codrax repo itself).

Layout:

- `repo-greet-go/`   — Go service exposing `UserService` interface +
  `GreetServiceImpl` implementer. Exercises cross-repo Implementers
  fan-out (Sc 4 of 2026-05-08 multi-repo architecture).
- `repo-tools-py/`   — Python tooling with `process_request`
  identifier. Exercises cross-repo keyword search (Sc 5).
- `repo-stub-rust/`  — Rust stub with one unrelated function.
  Sentinel for "should NOT appear in answer" — used by single-focus
  case to verify no cross-repo confabulation.

How tests use it: a case sets `MULTIREPO=multirepo-basic` and the
runner copies this tree to a scratch dir, `git init` + commits each
immediate child, then dispatches codrax with `--repo <scratch>` (the
parent dir). Topology discovery (BFS depth=4) auto-detects the three
sub-repos and the analyzer routes per question.
