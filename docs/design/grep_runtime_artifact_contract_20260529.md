# Grep Runtime Artifact Contract (2026-05-29)

## Problem

Customer trace/log investigations exposed that the `grep` tool still behaves like
a source-code search helper even when the user is investigating one large runtime
artifact such as a log, systrace, atrace, htrace, or perfetto dump.

The failure mode is not that grep cannot search the file. Exact literal searches
work. The deeper issue is that runtime artifacts are often large, line-oriented,
and format-variable, while the current tool contract exposes only a regex search
surface designed around repository source navigation.

## Code Findings

- `GrepTool.Execute` routes broad repository/directory search through ripgrep,
  GNU/BSD grep, or the Go native walker. Explicit single-file searches may use
  shell `grep -E` when ripgrep is absent.
- `rg`, Go regexp, and GNU/BSD `grep -E` do not share one regex dialect. Common
  model patterns such as `\d`, `\s`, and `\w` are safe in Go regexp / ripgrep
  but not portable in POSIX ERE grep. On single-file logs this can create false
  no-match results.
- `grep` has no literal mode. Logs/traces contain punctuation-heavy event labels
  where regex escaping is unnecessary risk.
- `grep` has no line-window parameters. Once a model knows a rough artifact
  line range, it must either use `read_file` or synthesize shell/awk. A generic
  line window is safer than a format-specific time-window API.
- Shell-backed grep currently collects stdout into memory before compaction.
  This is acceptable for ordinary code searches but risky for a single huge log
  where hundreds of thousands or millions of lines may match.
- The banner word `matching lines` is parsed by downstream code, so context-line
  wording must stay backward compatible.

## Contract

This batch strengthens `grep` as a generic text-search tool without changing
repository source-search defaults:

- Default source-code grep semantics remain unchanged.
- Runtime/log/trace handling is detected only from an explicit single-file path
  or a known attached artifact path/name. No user-prose keyword decision is used
  for hard behavior.
- System hints are advisory. They may teach better search shapes, but must not
  force retry or rewrite.
- Literal, line-window, and regex-compatibility support is generic and useful for
  code/config/log/trace text alike.

## Design

### P0: Regex Dialect Compatibility

When the selected backend is shell `grep`, the target is an explicit single file,
and the pattern contains common non-POSIX shorthand escapes (`\d`, `\s`, `\w`
and their uppercase forms), route through the Go native scanner instead of
`grep -E`. This keeps existing directory/source grep behavior unchanged while
making single-file log/config/source keyholes less dialect-dependent.

### P0: Runtime Artifact No-match and Broad-result Guidance

For explicit runtime artifact files, no-match results must say the regex matched
zero lines, not that the fact is absent. The corrective hint should recommend:

- try one exact literal / timestamp / thread id / event label;
- remember regex order is line-order dependent;
- use `fixed_string=true` for punctuation-heavy literals;
- use `line_start` / `line_end` after discovering a line vicinity.

Broad-result hints must remain runtime-artifact-specific and must not mention
repo relation maps.

### P1: Literal Search

Add `fixed_string: boolean` to `grep`.

- `rg`: use `-F`.
- GNU/BSD grep: use `-F` instead of `-E`.
- native scanner: use `strings.Contains` (case-folded when ignore-case applies).

### P1: Generic Line Window

Add optional `line_start` and `line_end` parameters, 1-based inclusive. They are
valid only with an explicit file path. The implementation uses the native scanner
for portability instead of shell pipes, so it works on Linux, macOS, and Windows.

### P1: Runtime Artifact Streaming Capture

For explicit runtime artifact shell-grep searches, stream stdout instead of
capturing the whole output in memory. Keep a capped in-memory preview and write
full output to a session blob once it grows past a small threshold. Ordinary code
search keeps the existing relevance-aware full-output path.

### P2: Context-line Banner Compatibility

When `context_lines > 0`, keep the legacy substring `matching lines` but append a
clarifier that the count includes context output lines. This avoids breaking
downstream parsers while reducing model confusion.

## Task List

| Task | Status | Notes |
| --- | --- | --- |
| T1 | done | Added `fixed_string`, `line_start`, `line_end` schema and native scanner support. |
| T2 | done | Single-file risky regex shorthand (`\d`, `\s`, `\w`) routes through the native scanner when shell grep would be dialect-unsafe. |
| T3 | done | Runtime artifact no-match and broad-result guidance now stays artifact-specific and does not suggest repo relation maps. |
| T4 | done | Explicit runtime artifact shell grep streams stdout to a temp artifact and keeps only a capped preview in memory. |
| T5 | done | `matching lines` remains in the banner while context output is clarified. |
| T6 | done | Added regression coverage for literal search, line windows, runtime artifact no-match/broad output, native scanner behavior, and streaming blob writes. |

## Validation Plan

- `go test ./internal/tool` — passed 2026-05-29.
- `go test ./internal/agent ./internal/orchestrator` — passed 2026-05-29; this protects downstream grep banner parsers and stage contracts.
- Full `make test` remains the final pre-push validation for this batch.

## Delivered Scope

The implementation is intentionally generic:

- code/config/source searches keep the same default regex behavior;
- literal mode is opt-in and works for any text file, including source and config;
- line windows are file-format agnostic and work on Linux, macOS, and Windows;
- runtime artifact streaming is gated by an explicit single-file artifact path, so
  repo-wide code grep still uses the existing relevance-aware ranking path;
- no new hard gates were introduced. All recovery instructions are advisory tool
  output, not retry or rewrite enforcement.
