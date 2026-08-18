# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T07:53:10Z
- sweep_start_ts: 20260818-005308
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-005310 | answer_regex,answer_contains | none | 98s | 25 | read=3,repo_map=2,list=1,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Functional answer is correct, but all three platform mechanism rows cite only definition entries while claiming body APIs/conversions. The source bodies were fully read; the typed handoff still marks those refs `definition_site_only/executable_body=unproven`. This confirms B1055, a general selected-mechanism operation-evidence gap rather than a C-specific answer error. |
| 2 | github_issue_nlohmann_long_double_symptom | PASS | eval/results/github_issue_nlohmann_long_double_symptom-20260818-005310 | write_apply,answer_regex | none | 181s | 25 | read=5,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | pass | Both distribution headers changed `%.*lg` to `%.*Lg`; no tests were modified, `make check` passed, changed-path coverage is complete, and the isolated worktree finished cleanly as verified. Two unavailable-tool attempts and repeated localization are efficiency debt, not a correctness failure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### C platform mechanism read

- The final answer correctly distinguishes Windows `QueryPerformanceFrequency`/`QueryPerformanceCounter`, macOS `mach_timebase_info`/`mach_absolute_time`, and POSIX `clock_gettime(CLOCK_MONOTONIC)`, and correctly identifies `cmd_sleep` as the only command handler caller with three calls.
- Explorer read all of `src/clock.c` and saw the exact operations at lines 15/17/18, 28/30, and 39/40. It nevertheless emitted three model-selected `mechanism` rows anchored only at function-definition starts 11/25/37. Completion then copied those same definition refs into the platform member set. Finalizer was explicitly told that `definition_site_only` cannot prove body behavior, but it had no operation-level rows for the platform APIs and still rendered the correct model summary with weak citations.
- The JSON `blocks` payload arrived as a JSON-encoded string and was recovered once. The full answer survived, so the bounded JSON self-healing path worked and is not the source of the evidence gap.
- B1055 fixes the shared producer seam: a typed `mechanism` request with a required `function_or_purpose` dimension may auto-pair exact parser-owned calls from a model-selected, already-read defining callable. Related/absence/illustrative definitions are excluded. The producer supplies facts only; it cannot select members, relations, diagrams, or conclusions and reads no user/model/final prose.

### C++ write mode

- The applied tree changes exactly `include/nlohmann/detail/output/serializer.hpp` and `single_include/nlohmann/json.hpp`, keeping the generated/single-header copy synchronized. Both replacements are `%.*lg` -> `%.*Lg`; `make check` passes and verification signs both paths.
- The workflow spent 11 controller dispatches and made two unavailable-tool attempts. Analyzer, Explorer, and Planner also repeated exact localization that prior typed handoff had already established. This is a general write-efficiency observation, but it did not cause replan churn, unsafe edits, false-green verification, or user-visible failure, so it remains below B1055.
- One model rationale described printf conversion arguments as pointer types. The patch and native verification are still correct; this is model reasoning imprecision, not authority for a system rewrite or a prose hard gate.

### Invariants

- No runtime Trace code changed. Explicit-window causal diagnosis, Trace causal projection, deterministic supplement, typed on-chain-only root causes, and support-only adjacent/background evidence remain unchanged.
- No active-stream fixed-age degradation occurred. Continued bytes do not authorize a 4ms fallback; only caller cancellation/deadline, no-first-byte, byte stall, transport failure, or decode failure may end/recover a stream with explicit disclosure.
