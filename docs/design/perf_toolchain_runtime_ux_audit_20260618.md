# Perf Toolchain + Runtime Artifact UX Audit

## Summary

This audit closes the follow-up after raw perf.data fallback support: Codrax
should keep the two-engine perf strategy visible to operators, preserve
trace+perf handoff as one bundle, and make explicit runtime artifact paths
transparent in terminal status plus markdown/html dumps.

The trace_query running root-cause lane is already mostly closed in code:
`EventPerfSample`, `WindowStats.PerfSamples`, `RootCauseRankItem.PerfContext`,
role-aware `PerfContexts`, and frame bundle fields such as
`target_running_perf`, `on_chain_perf`, `binder_peer_perf`, and
`same_cpu_competitor_perf` are implemented. Perf samples are consumed as
code-execution support for running/runnable/compute-supply/on-chain/binder-peer
candidates; they do not create an independent scheduling root cause.

The relative-path Donghu regression noted during eval is closed. The final
post-fix run (`perf_runtime_ux_donghu_path_after_claimform_fix_20260618`) passed
both relative and absolute path cases with `flagged: 0 / 2`, `read=0`,
`repo_map=0`, and `list_files=0`. Both outputs preserved the
CookieMonsterCl -> com.baidu.tieba wakeup/runnable relationship, and the facet
planner kept `current_code_path` optional rather than hard-required.

## Official Sources

- OpenHarmony `developtools_hiperf` describes hiperf as the official
  OpenHarmony command-line performance tool, records to `perf.data` by default,
  requires Python 3.7+, builds host binaries such as `hiperf_host`, and provides
  report/dump/script flows plus symbol collection helpers:
  https://gitee.com/openharmony/developtools_hiperf
- OpenHarmony profiler trace headers identify embedded standalone data types,
  including `HIPERF_DATA`, inside htrace-like containers:
  https://gitee.com/openharmony/developtools_profiler
- Android simpleperf `report_sample.py` and `simpleperf_report_lib.py` expose
  sample pid/tid/thread name/time/cpu/period/event/symbol/callchain data used by
  the official Android adapter path:
  https://android.googlesource.com/platform/system/extras/+/refs/heads/main/simpleperf/scripts/

## Current Code Flow

- `codrax trace convert` accepts `--hiperf-host`, `--hiperf-symbol-dir`,
  `--simpleperf-report-sample`, `--simpleperf-python`, `--simpleperf-symfs`,
  `--simpleperf-kallsyms`, `--perf-parser=auto|official|raw`,
  `--no-perftrace`, and `--perf-tools-status`.
- `--perf-parser=auto` prefers official hiperf/simpleperf adapters, then falls
  back to Codrax raw perf.data parsing when supported.
- Raw perf.data fallback now consumes official OpenHarmony hiperf feature
  sections where their layouts are stable: `HOSTNAME`, `OSRELEASE`, `VERSION`,
  `ARCH`, `NRCPUS`, `CPUDESC`, `CPUID`, `TOTAL_MEM`, `CMDLINE`, `EVENT_DESC`,
  `HIPERF_WORKLOAD_CMD`, `HIPERF_RECORD_TIME`, `HIPERF_CPU_OFF`,
  `HIPERF_HM_DEVHOST`, `HIPERF_FILES_SYMBOL`, and a bounded summary of
  `HIPERF_FILES_UNISTACK_TABLE`.
- `EVENT_DESC` upgrades raw sample labels from `config:0x...` to official event
  names and maps sample ids when present. `HIPERF_CPU_OFF` plus
  `sched:sched_switch` marks raw samples as `sample_kind=off_cpu`; other raw
  samples remain conservative rather than forcing `on_cpu`.
- `HIPERF_FILES_UNISTACK_TABLE` is exposed as table/node/pid provenance only.
  Full deduplicated stack-id expansion remains an official hiperf report
  responsibility because it depends on sample stack ids plus the full
  `UniqueStackTable` flow.
- Converter outputs can include `.systrace`, `.perf.data`, `.perftrace`, and
  `.tracebundle.json`; trace_query accepts the bundle directly and can merge
  systrace+perftrace evidence.
- CLI and REPL attachment paths already print runtime artifact status for
  attached logs/traces, and markdown/html dumps already render a Runtime
  Artifacts table for attachments and tracebundle metadata.
- Analyzer/explorer prompt guidance already teaches that explicit runtime
  artifact paths should go through the runtime-artifact lane and that trace_query
  can consume `.tracebundle.json` directly.
- `emit_analysis` now preserves path-shaped runtime artifact tokens from the
  current request as typed runtime artifact hints when the analyzer declared
  external artifact citations, even if the model omitted `required_files` or the
  path is outside the active repo.
- `ClaimFormOf` treats evidence whose source is a runtime artifact path as
  `external_observation`, so trace/log rows cannot satisfy `definition_fact` or
  `current_code_path` lanes by accident.

## Residual Gaps

1. Convert handoff can accidentally split trace and perf.
   When a bundle exists, the next-step hint should prefer attaching the
   `.tracebundle.json`, not only the generated `.systrace`, so CPU samples and
   raw-perf provenance stay available to trace_query.

2. Convert artifact rows are too terse.
   Operators need converter, bytes, data type, plugin, source offset/size, and
   caveats directly in CLI/REPL output, not only inside JSON bundle metadata.

3. Explicit path-only requests need output transparency.
   If the user writes a trace/log/perf path in the question instead of attaching
   it with `--htrace`/`/htrace`, final markdown/html dumps should still show the
   referenced runtime artifact paths. This must remain a transparency feature,
   not an intent-classification shortcut.

4. Tool discovery/install guidance is present but not prominent enough.
   `--perf-tools-status` exists; command help and REPL convert usage should make
   it visible and explain official-first/raw-fallback behavior.

5. Running+perf docs were stale.
   The code already implements role-aware perf contexts, but the older plan file
   still reads like the implementation is pending. Documentation needs to mark
   current coverage and keep any remaining UX work separate from core
   root-cause logic.

6. Zero-impact state_churn snapshots can add noise.
   Runtime answer mutation previously materialized any structured `state_churn`
   row into the final answer. When the bounded root cause is a clear runnable
   CPU competitor and a secondary `state_churn` row has `impact=0`, that snapshot
   can confuse the answer without adding diagnostic value.

7. Perf quality source fields can be dropped in final prose.
   Raw fallback evidence carries critical quality fields such as
   `source=raw_perfdata_fallback`, `symbolization_status=unsymbolized`,
   `clock_confidence=assumed`, and `callchain_status=ip_only`. These fields were
   present in trace_query typed observations but could be omitted by the model's
   final prose, weakening the answer's caveat precision.

8. Path-only runtime traces could leak into source facets.
   A model may put a trace path in `required_files`, omit the path from
   `external_observation_policy.source_quotes`, or emit trace line evidence
   through `emit_evidence`. Without a typed path projection and source-based
   claim-form guard, the planner can ask for `current_code_path` and the
   finalizer can misread trace rows as source definitions. This is fixed by
   preserving runtime path hints and projecting runtime artifact sources to
   `external_observation`.

## Design

- Preserve the structural boundary: explicit path recognition is used to expose
  tools/status/dump rows and seed runtime-artifact handling; it must not hard-code
  whether the user wants trace-only, code-only, trace-vs-code comparison, or
  code-vs-trace comparison.
- For path-only UX, scan request text for path-shaped tokens and use canonical
  runtime path helpers or content sniffing for explicit locators. Suffixless
  files are accepted only when readable content looks like runtime trace/perf
  data.
- Merge request-path artifacts with attached artifacts before status/dump output,
  de-duplicating by kind+source.
- Project path-shaped runtime artifact tokens from the current request into the
  typed artifact hint lane only after the analyzer has declared external
  artifact citation handling. This preserves artifact identity without
  classifying user intent from suffixes or prose.
- Treat runtime artifact paths as observation sources in claim-form projection,
  preventing trace/log rows from satisfying source-code facets.
- Prefer `.tracebundle.json` in convert next-step hints whenever the converter
  produced one.
- Keep perf samples as explanatory support for existing scheduler/resource
  candidates; ranking remains based on overlap, chain relevance, cumulative
  impact, CPU/core/frequency/affinity, runnable pressure, D/io_wait, IO, and
  supply evidence.
- Keep raw fallback official-feature consumption as provenance and bounded
  sample enrichment. Do not treat unexpanded unistack tables as recovered
  callchains, and do not promote perf-only samples to standalone scheduler root
  causes.

## Official Format Delta After Latest Comparison

- OpenHarmony profiler/ftrace protos include useful systrace event families
  already relevant to Codrax analysis: binder, sched, sched_blocked_reason,
  filemap, f2fs, mmc, irq/softirq/ipi, workqueue, clock_set_rate,
  cpu_frequency, cpu_frequency_limits, and related trace marker events. Existing
  text systrace parsing consumes many of these once rendered as stable text
  rows; remaining converter work should add explicit renderers per event family,
  not generic unknown-event body dumping.
- OpenHarmony hiperf official perf.data feature layouts confirmed useful
  fallback fields beyond samples: device/kernel/arch/cpu/memory provenance,
  command line and workload command, record time, event descriptions, Harmony
  devhost pid, CPU-off mode, saved symbols, and deduplicated stack-table
  presence. The low-risk subset is now parsed into `.perftrace` parser caveats
  and trace_query quality context.
- Larger future work: expand `PERF_RECORD_HIPERF_CALLSTACK` / deduplicated
  stack ids only if the raw parser can faithfully reproduce official
  `UniqueStackTable` expansion. Until then the commercial path remains
  official-first with raw fallback providing transparent degraded evidence.

## Task List

### Batch A: UX Transparency

- [x] Add request-path runtime artifact rows to CLI status and markdown/html dump
  inputs.
- [x] Preserve suffixless trace/perf path transparency through content sniffing
  for explicit locators.
- [x] De-duplicate attachment and request-path artifact rows.
- [x] Prefer tracebundle in `trace convert` next-step hints.
- [x] Include converter/provenance/caveats on CLI and REPL convert artifact rows.
- [x] Suppress final-answer `state_churn` metric snapshots when the structured
  churn impact is zero; keep non-zero fragmented-state evidence visible.
- [x] Materialize final-answer `Perf 证据质量` rows from structured trace_query
  `perf_quality` notes so raw fallback/source/symbolization/clock/callchain
  limits survive model summarization.
- [x] Preserve runtime artifact paths emitted through `required_files` even when
  they are outside the active repo; do not warn them as missing current-source
  files.
- [x] Project current-request runtime artifact path tokens into typed artifact
  hints when the analyzer declares external artifact citations but omits the
  path from structured fields.
- [x] Force evidence sourced from runtime artifact paths to
  `external_observation` claim form so systrace/log rows cannot become
  `current_code_path` support.

### Batch B: Tooling Guidance

- [x] Update `trace convert` long help with official-first/raw-fallback strategy.
- [x] Update REPL `/htrace convert` usage to point at `--perf-tools-status`.
- [ ] Add a concise user-facing install/discovery section to docs once the
  current UX batch is verified.

### Batch C: Tests and Evals

- [x] Unit-test request-path runtime artifact dump rows, including suffixless
  content-sniffed traces and tracebundle expansion.
- [x] Unit-test trace convert bundle-preferred next-step hints and artifact
  provenance details.
- [x] Unit-test zero-impact state_churn snapshot suppression.
- [x] Unit-test raw fallback perf quality materialization.
- [x] Unit-test official hiperf raw feature parsing: EVENT_DESC event names,
  device/cpu/memory/workload/record-time metadata, CPU-off sample kind, saved
  symbol provenance, and unistack summary caveats.
- [x] Unit-test runtime artifact path preservation in `required_files` repair.
- [x] Unit-test current-request runtime artifact path projection into typed
  hints.
- [x] Unit-test runtime artifact source claim-form projection.
- [x] Run focused Go tests for `cmd`, `internal/outputdump`, `internal/repl`,
  `internal/orchestrator`, `internal/hitraceconv`, and `internal/tracequery`.
- [x] Run full `go test ./...` and `make`.
- [x] Run perf/running and runtime-path eval cases in batches of two.

### Batch D: Documentation Closure

- [x] Update the older running+perf root-cause plan to reflect implemented code
  coverage and point remaining work to this UX/toolchain audit.
- [x] Push Batch A-D after verification.

## Validation

- `go test ./internal/types ./internal/tool`
- `go test ./cmd ./internal/outputdump ./internal/repl ./internal/orchestrator ./internal/hitraceconv ./internal/tracequery ./internal/tool ./internal/types`
- `go test ./...`
- `make`
- Eval batch `perf_runtime_ux_batch1_after_quality_20260618`: 2/2 PASS,
  `flagged: 0 / 2`.
- Eval batch `perf_runtime_ux_batch2_after_quality_20260618`: 2/2 PASS.
- Eval batch `perf_runtime_ux_donghu_path_after_claimform_fix_20260618`: 2/2
  PASS, `flagged: 0 / 2`, `read=0`, `repo_map=0`, `list_files=0`.

## Acceptance Criteria

- A user can provide runtime artifacts as attachments or as explicit paths in the
  question and still see what Codrax consumed in terminal status plus markdown
  and HTML reports.
- Converted htrace bundles do not lose perf context through a systrace-only
  next-step suggestion.
- Official hiperf/simpleperf and raw fallback are both visible as deliberate
  parser choices.
- Running root-cause answers can use perf context as code-execution support
  without promoting samples to standalone scheduling causes.
- No logic matches user intent or model prose by keywords; hard behavior is
  driven by typed request fields, explicit paths, readable artifact content, and
  structured trace/perf data.
