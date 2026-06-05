# Process Event Sink for CLI and REPL

## Problem

REPL renders rich process cards for code, trace, operation, and data lanes.
Single-shot CLI intentionally keeps stdout clean so the final Markdown answer
can be piped, redirected, or checked by eval. That split is correct, but the
data lane exposed a gap: REPL showed plan/runner/workflow events while CLI
only exposed final stdout plus debug logs. In eval, `run-*.out` captures
stdout and stderr together, so users saw model thinking but not the same
low-noise workflow markers they expected from REPL.

## Design Principles

- stdout remains final-answer-only for single-shot CLI.
- CLI process events go to stderr.
- REPL process events stay in the interactive renderer.
- Process events are advisory UX and audit telemetry; they are never used as
  hard gates.
- Events are typed by lane/kind/round/segments. They must not be inferred from
  user prose or model prose.
- Detailed artifacts, large outputs, scripts, and full JSON stay in audit
  files/logs. The event stream shows compact summaries only.
- The design is lane-neutral: data, command operation, future external skills,
  and future provider workflows can emit the same shapes.

## Event Shape

Conceptual event:

```text
lane: data | operation | provider | write | trace | ...
kind: plan | execute | result | repair | evaluate | continue | blocked
round: optional integer
label: short localized label
segments: compact facts such as status, risk, required material count,
          contribution count, reconcile status, approval mode, provider name
body_ref: optional audit/payload path
```

REPL rendering:

```text
◇ 数据计划 · 就绪 · 输入 3 · 输出 单行文本 · 必需材料 3
◇ 数据工作流 · 结果第 2 批 · 决策记录 3 · 贡献记录 3 · 对账 pass
```

CLI stderr rendering:

```text
◇ 数据计划 · 就绪 · 输入 3 · 输出 单行文本 · 必需材料 3
◇ 数据工作流 · 执行第 1 批 · 未读源码
◇ 数据工作流 · 结果第 2 批 · 未读源码 · 贡献记录 3 · 对账 pass
```

CLI stdout still contains only:

```text
<final answer markdown>
```

## Current Implementation Batch

Data single-shot CLI now emits low-noise process events to the configured
progress writer, normally stderr. It reuses the same summary builders used by
the REPL data route:

- `dataTaskPlanAuditSummary`
- `dataTaskWorkflowResultSegment`
- `dataTaskWorkflowErrorSegment`

Operation single-shot CLI already had a progress writer on stderr. This batch
does not rewrite operation rendering; it documents the shared event surface and
keeps the operation path stable.

## Non-goals

- Do not move final answers to stderr.
- Do not print full scripts, full command output, or full result JSON to the
  process stream.
- Do not colorize CLI by default. `--color=always` remains an explicit
  user-facing rendering choice where supported.
- Do not use process events to decide user intent.

## Task Checklist

- [x] Document the shared process event shape and stdout/stderr split.
- [x] Add CLI data progress events for plan, execute, repair, result,
      evaluate, and continue.
- [x] Route single-shot data progress to stderr from the CLI entrypoint.
- [x] Add CLI data workflow test coverage proving progress events are emitted
      while final answer stays strict.
- [ ] Gradually converge operation CLI progress rendering onto the typed event
      helper without changing existing behavior.
- [ ] Add provider/external skill workflow events once those providers use the
      same typed process surface.

