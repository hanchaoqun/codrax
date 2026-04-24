# patch_typo_go — write-mode eval fixture

Source tree used by `eval/cases/patch_go_typo.case` to exercise the
kind=patch end-to-end path with a real LLM.

## What's here

- `main.go` — a 25-line greeting CLI with a deliberate `retrun` typo on
  the line that should produce the `return` statement inside `greet()`.
  Any other content is incidental (imports, trimming, CLI arg loop)
  and only present to make "rewrite the whole file" clearly wrong
  versus "patch one line".

## Not a git repo on disk

The fixture is checked into codrax as plain source files. `run.sh`
copies the tree to a per-run scratch dir under `eval/results/<id>-<ts>/`
and runs `git init` + an initial commit there, because write mode
requires a git worktree. That keeps the committed fixture byte-stable
across runs and avoids nested `.git/` under the codrax repo.

Do **not** commit a fixed version of `main.go` — the typo is the test
input.

## Expected LLM behaviour

Under the current `change-plan-skill` prompt (`skill/defaults.go`), the
planner should prefer `Kind="patch"` for a single-line edit. If it
emits `Kind="modify"` with a full-file rewrite, that is a prompting
signal — not a bug in the machinery — and the case will fail at the
`PLAN_EXPECT_REGEX='"kind":\s*"patch"'` assertion.
