# REPL Transient Frame Rendering Fix

Date: 2026-05-27

Status: implemented and validated.

## Problem

In interactive REPL mode, transient UI rows can leave ghost text on the
terminal. Reproduced examples:

- Type `/` so the slash-command suggestion panel appears, then delete `/`.
  Some suggestion rows remain on screen.
- Run `/repos` after the slash suggestion panel was visible. Parts of the old
  transient panel can visually mix with the persistent `/repos` output.

This is not a `/` or `/repos` special case. Any feature that draws temporary
input-area rows and then prints persistent output can hit the same class of
bug.

## Code Findings

- Native interactive input is rendered by `internal/repl/native_input.go`.
- `nativeLineInput.render()` draws a frame made of the prompt line plus optional
  slash suggestions, then moves the cursor back to the prompt line.
- `nativeLineInput.clearRendered()` stores only `renderedRows` and assumes the
  cursor is on the last row of the previous frame. That invariant is false:
  after `render()` the cursor is intentionally on the first row, at the input
  cursor column.
- The row counter is logical line count, not terminal physical row count. It
  does not account for ANSI styling, CJK/wide characters, terminal wrapping, or
  terminal width changes.
- `/repos` prints persistent rows through `REPL.info()` after input submission.
  It is only implicated because stale transient rows were not fully cleared
  before persistent output started.
- Bubble Tea fallback owns its frame diffing through Bubble Tea and is lower
  risk. The primary defect is in the native input path.

## Root Cause

The native renderer lacks a first-class "transient frame" abstraction. It tracks
only how many logical rows were printed, but not where the terminal cursor is
inside that frame when the next clear operation begins. Clearing from the wrong
cursor row erases rows above the prompt and leaves rows below it untouched.

The same missing abstraction also makes future transient surfaces fragile:
slash suggestions, paste placeholders, help panels, confirmation prompts, and
command-discovery hints all need the same lifecycle:

1. compute the full transient frame;
2. draw it;
3. remember physical height and cursor row inside the frame;
4. clear exactly that frame before shrinking it or before printing persistent
   output.

## Design

Add a native-only transient frame state:

- `rows`: terminal physical rows occupied by the last transient frame;
- `cursorRow`: zero-based row inside that frame where the cursor currently sits.

`render()` will:

- clear the previous frame first;
- render the new frame;
- compute physical height using display width, not byte/rune count;
- move the cursor from the bottom of the new frame to the input cursor row;
- store `rows` and `cursorRow`.

`clearRendered()` will:

- start from the actual cursor row;
- move up only `cursorRow` rows to reach the top of the previous frame;
- erase each physical row with `ESC[2K`;
- return to the top of the cleared frame;
- reset the frame state.

Terminal row accounting will use existing display-width machinery already
available in the REPL stack (`lipgloss.Width` / `runewidth`) rather than adding
a new dependency. Transient input chrome may keep a one-column safety margin to
avoid terminal auto-wrap edge cases; this does not affect final answers,
tables, diagrams, or CLI/scripted mode.

## Boundaries

- Do not special-case `/`, `/repos`, or any single command.
- Do not change non-interactive CLI/scripted output.
- Do not alter final-answer diagram/table no-wrap behavior.
- Do not route persistent command output through the transient renderer.
- Do not introduce a new terminal UI dependency.

## Task List

| Task | Status |
| --- | --- |
| T1 Document issue, root cause, boundaries, and generic design. | Done |
| T2 Replace `renderedRows` with native transient frame state. | Done |
| T3 Make `clearRendered()` cursor-row aware. | Done |
| T4 Use display-width-based physical row accounting. | Done |
| T5 Keep transient prompt/suggestion rows inside terminal width. | Done |
| T6 Add regression tests for slash panel shrink, cursor-row clearing, ANSI/CJK width, and submit cleanup. | Done |
| T7 Run focused REPL tests and package tests. | Done |

## Verification Plan

- `go test ./internal/repl`
- Focused unit tests that capture the ANSI sequence emitted by native input
  rendering, proving that shrinking a multi-row transient frame erases the old
  rows below the prompt rather than rows above it.
- Regression coverage for styled/CJK suggestion rows so the physical-row
  accounting is not byte-based.

## Completion Notes

- Native input now stores a `nativeRenderedFrame` instead of a bare logical row
  count.
- Clearing starts from the remembered cursor row inside the previous frame, so
  shrinking a slash suggestion panel clears the rows below the prompt instead of
  erasing unrelated rows above it.
- Frame height is computed from display width with ANSI/CJK awareness.
- Transient prompt/suggestion rows keep a one-column safety margin to reduce
  terminal auto-wrap ambiguity.
- Validation: `go test ./internal/repl ./internal/render`.
