# Config-Key Seeder Search Audit (2026-05-31)

## Problem

The analyzer config-key seeder added for phantom config keys is valuable, but
the first implementation had two operational risks:

1. It ran one repository text search per config-key token. A config key can
   contribute up to eight tokens, so a large workspace could pay up to eight
   full scans during analyzer required-file seeding.
2. It passed each token to the generic grep helper as a regular expression.
   Config keys are user/model surfaces, and punctuation such as `+`, `[`, `]`,
   `(`, `)`, or `?` must be treated as literal key text, not regex syntax.

Both problems sit in a soft-navigation path, but they can still hurt customer
experience: slow analyzer startup on large repositories, or noisy/empty schema
home seeds for punctuation-heavy config keys.

## Root Cause

`configKeyGroupSiblingFiles` needed per-token file coverage so it could keep
only files that contain multiple distinct key tokens. It reused `grepFiles` in
a loop because the existing IDF search collapses a file's hit map to one
keyword and cannot report distinct-token coverage.

That reuse was correct for functionality but poor for this narrower seeder:

- `grepFiles` is regex-shaped and guarded per call by `searchTimeout`.
- Repeating it per token multiplies full-repo scan cost.
- Regex interpretation is not part of the config-key seeder's semantics.

## Design

Add a small batch literal search helper in the analyzer keyword-search layer:

- Use one `rg --json --fixed-strings` call when ripgrep is available.
- Preserve per-token file lists by reusing the existing lightweight rg JSON
  path/submatch parsing.
- Use case-insensitive literal matching for config keys, so lower-cased tokens
  still match CamelCase struct fields and YAML tags.
- Fall back to bounded concurrent literal searches when rg is not available:
  - Go-native search uses `NativeGrep` with `FixedString=true`.
  - system grep uses `grep -F -l` instead of regex `grep -E`.

Keep the seeder's typed gate and cap unchanged. The change must not alter
normal code/config analysis except to make this supplemental config-key seed
path faster and more literal-safe.

## Non-Goals

- Do not change repo_map behavior.
- Do not change final hard gates or evidence semantics.
- Do not infer config schema from raw prose.
- Do not seed files for non-config-key requests.

## Task Checklist

- [x] Record the audit finding and solution.
- [x] Add a batch literal file-search helper.
- [x] Route `configKeyGroupSiblingFiles` through the helper.
- [x] Add tests proving punctuation-heavy config keys are treated literally.
- [x] Add tests proving the seeder uses one batch search rather than one search
      per token.
- [x] Run focused and full test suites.
- [x] Commit and push the batch.
