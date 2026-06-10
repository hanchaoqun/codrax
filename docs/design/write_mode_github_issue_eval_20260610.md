# Write Mode GitHub Issue Eval Evidence Ledger - 2026-06-10

## Scope

This ledger records a real-world write-mode evaluation using non-Go GitHub issue fixes. The goal is not to tune for these cases, but to expose system-level gaps in the controller DAG, typed verification, approval policy, durable artifacts, and handoff.

`<think>` text rendered in user-visible logs is expected transparency and is not treated as a defect.

## Upstream Sources

| Case | Language | Upstream record | Local fixture | Shape |
| --- | --- | --- | --- | --- |
| `github_issue_libgit2_foreach_worktree` | C | [libgit2 issue #7216](https://github.com/libgit2/libgit2/issues/7216), [PR #7231](https://github.com/libgit2/libgit2/pull/7231) | `eval/fixtures/github_issues/libgit2_foreach_worktree` | single source file |
| `github_issue_nlohmann_long_double` | C++ | [nlohmann/json PR #3929](https://github.com/nlohmann/json/pull/3929) | `eval/fixtures/github_issues/nlohmann_long_double` | synchronized two-header edit |
| `github_issue_commons_lang_random_ascii` | Java | [apache/commons-lang PR #1273](https://github.com/apache/commons-lang/pull/1273) | `eval/fixtures/github_issues/commons_lang_random_ascii` | production + regression tests |
| `github_issue_zod_prefault` | TypeScript | [Zod issue #5824](https://github.com/colinhacks/zod/issues/5824), [PR #5893](https://github.com/colinhacks/zod/pull/5893) | `eval/fixtures/github_issues/zod_prefault` | production + regression tests |

## Local Eval Additions

- Added `eval/cases/github_issue_*.case` for four real issue fixtures.
- Added `eval/fixtures/github_issues/*` with reduced, runnable reproductions and upstream source links.
- Added `make eval-github-issues` to run every GitHub issue case.
- Hardened `eval/run.sh` so apply-mode evals fail when plan generation fails or apply is skipped.
- Hardened `eval/run.sh` post-apply inspection to support git worktrees whose `.git` is a file.
- Narrowed the Zod fixture checker so `result.schema._prefault !== undefined` is accepted while bare truthiness is still rejected.

## Commands Run

```bash
make eval-github-issues SAMPLES=1
bash eval/run.sh eval/cases/github_issue_nlohmann_long_double.case 1
bash eval/run.sh eval/cases/github_issue_commons_lang_random_ascii.case 1
bash eval/run.sh eval/cases/github_issue_zod_prefault.case 1
bash -n eval/run.sh && make eval-runner-test
cd eval/fixtures/github_issues/zod_prefault && make check
cd eval/fixtures/github_issues/commons_lang_random_ascii && make check
```

The fixture `make check` commands are expected to fail on the original buggy fixtures.

## Trusted Results

| Case | Latest trusted result | Result dir | Main evidence |
| --- | --- | --- | --- |
| C libgit2 | PASS | `eval/results/github_issue_libgit2_foreach_worktree-20260610-175959` | Worktree contains both precedence fixes and verifies. |
| C++ nlohmann | FAIL | `eval/results/github_issue_nlohmann_long_double-20260610-181221` | Valid tiny two-file plan blocked as high risk due `affects_public_api`. |
| Java commons-lang | FAIL | `eval/results/github_issue_commons_lang_random_ascii-20260610-181635` | First plan applied, verifier selected Maven instead of Makefile, retry found precise failure but did not emit a repair plan. |
| TS Zod | FAIL | `eval/results/github_issue_zod_prefault-20260610-181635` | First plan applied a correct-looking fix, verifier selected Python zero-tests path, no durable report JSON was persisted, retry did not converge. |

Trusted pass rate after runner fixes: 1 / 4.

The earlier run `eval/results/github_issue_nlohmann_long_double-20260610-180337` was a false positive before the harness fix: ModePlan produced no plan because the controller emitted `apply_plan` in plan mode, and the harness did not yet fail `MODE=apply` when apply was skipped.

## Evidence Highlights

- `libgit2`: the system completed a single-file C bugfix and preserved negative return values with `if ((error = cb_result) != 0)` and `if ((error = lookup_result) < 0)`.
- `nlohmann`: `run-1.plan.json` contains a minimal `%.*lg` to `%.*Lg` patch in both headers, but approval is `risk_level=high`, `action=manual_approval`, reason `affects_public_api`, despite the analyzer also marking overall risk low.
- `commons-lang`: `plan-1781086754245026000-91179.report.json` records `runner_missing` because verifier selected `java` and tried `mvn`. Earlier exploration and planner dry-run had already observed `Makefile` and `make check`.
- `commons-lang`: after `make check` dry-run exposed `ASCII fast path must explicitly require end <= 0x7f`, the planner continued searching and ended with `no change plan was produced this round`.
- `Zod`: `run-1.plan.json` shows both raw patches applied and risk auto-executed as medium. The result dir has no `*.report.json`, leaving the failed verify state without a durable typed report.
- `Zod`: verifier read `Makefile` and `tests/check_prefault_schema.py`, then ran `runner=python`; run_tests returned `NoTestsRunners` instead of executing `make check`.
- Across C, C++, Java, and TS, structured edits repeatedly failed with `old_text mismatch`; C++ and older Zod logs also showed `invalid line range N-0`.

## System Gaps

### P0 - Typed Verification Surface

Verifier must consume a typed `TestSurface` selected before post-apply verify. The surface should include manifest observations, runnable commands, command ids, working directories, suite selectors, expected parser, and confidence. `Makefile -> make check` must outrank language-default runners when it is the only runnable project contract. A zero-test result must not be treated as useful verification when a runnable Makefile/script surface exists.

### P0 - Verify Result State Machine

Controller must distinguish planner dry-run from post-apply verify. Only the latest typed post-apply `ChangeReport` for the active batch attempt may drive `finish`, `replan`, or `block`. Batch state should not simultaneously read as `ready_to_plan` and `verify_failed`; model-facing state should be derived from one canonical attempt state.

### P0 - Durable Reports And Worktree Evidence

Every apply and verify attempt needs durable artifacts: plan snapshot, apply record, report JSON, command transcript digest, selected test surface, and failure evidence refs. Failure paths should either preserve the worktree until eval/report capture finishes or persist a patch/archive of the worktree diff. Missing `*.report.json` on Zod made the audit chain incomplete.

### P0 - Approval Risk Calibration

`affects_public_api` from write analysis is too broad for hard approval. The hard gate should derive API impact from typed diff analysis: exported signature changes, ABI-visible declaration changes, public config/build manifest edits, persistence/security surfaces, and external path or secret policy. Body-only literal bugfixes inside a public header should usually be medium and auto-executable, not high approval.

### P0 - Verify Failure Handoff

P2 context must carry structured verify failure evidence to replan: command id, runner, cwd, exit code, failure kind, stderr excerpt, failing assertion, touched path, and suggested next test surface. Replan should start from that P2 evidence and produce a small repair patch, not restart broad exploration or infer failure from prose.

### P1 - Structured Edit Builder Reliability

The structured edit path is too brittle. It should default `end_line=start_line` for single-line replace or make `end_line` schema-required. `old_text` comparison should expose the exact current bytes and normalize only documented read-file gutters/line endings. A model should be able to request "replace current line N with content" without copying stale `old_text`.

### P1 - ModePlan Action Mask

ModePlan should never present or accept `apply_plan`. Plan mode terminal semantics are simple: once a bounded plan is persisted, finish. The controller action schema should be masked by mode and workflow state before the model sees it.

### P1 - Planner Dry-Run Tooling

Planner currently uses `run_tests(dry_run=true)` but the runner abstraction still maps Python scripts to pytest zero-tests and Java to Maven. Dry-run should execute the typed test surface command in a non-mutating lane and return a `DryRunReport`, not a language default guess.

### P1 - Complexity-Aware Short Path

Small exact replacements still spend many rounds in analyze/explore/controller/planner. Literal or single-line fixes with known files should get a bounded fast path: confirm anchors, build patch, apply, verify. This is an optimization over the same typed DAG, not a separate legacy path.

### P1 - Context Pack Dedupe And Priority

Context packs currently include useful observations but do not consistently prevent repeated exploration. Dedupe should key by `(priority, source_stage, file, line, symbol, evidence_kind)` and render consumer-specific Top-N views. P2 verify failures must outrank P1 code facts on retries.

### P2 - Apply/Report Wording And Timestamps

Apply logs mention normalized raw patch and git apply details, but report status can still feel inconsistent across retries. `ChangeReport.generated_at` was `0001-01-01T00:00:00Z` in the Java report and should be populated.

### P2 - Eval Fixture And Harness Hygiene

The harness fixes landed in this eval pass and should stay covered: apply mode must fail if no plan is written or apply is skipped, and post-apply all-file matching must work for git worktrees. Zod checker broadness was fixed to avoid false negatives for `!== undefined`.

## Executable Follow-Up Tasks

1. Add `TestSurface` typed artifact and selector: manifest scan, runnable command inventory, command priority, parser choice, and no-tests policy.
2. Change verifier to execute the selected `TestSurface` exactly once and persist a `ChangeReport` for pass, fail, runner missing, timeout, no-tests, and parser errors.
3. Refactor controller state to consume only latest active batch post-apply reports; remove contradictory batch status rendering.
4. Make `ModePlan` action schema exclude apply/verify actions and finish immediately after plan persistence.
5. Replace hard approval use of analyzer `affects_public_api` with typed diff risk analysis.
6. Improve structured edit builder defaults and diagnostics; add regression tests for single-line replace, missing end_line, read-file gutter text, and exact-byte mismatch reporting.
7. Persist worktree diff/report evidence on failure before cleanup; update eval runner to prefer durable artifacts over live worktree when present.
8. Promote P2 verify failures to first-class context pack items consumed by replan, with Top-N dedupe.
9. Add commercial eval regression target for these four GitHub issue fixtures and mark expected trusted outcomes separately from current observed outcomes.
10. Add hygiene tests that controller prompts do not route on model prose or user-keyword matching; all routing remains typed action enum plus validated artifacts.

## Progress Ledger

- 2026-06-10: selected four non-Go upstream issue fixes and created reduced fixtures.
- 2026-06-10: added GitHub issue eval cases and `make eval-github-issues`.
- 2026-06-10: fixed apply-mode false positive in `eval/run.sh`.
- 2026-06-10: fixed git worktree post-apply inspection in `eval/run.sh`.
- 2026-06-10: ran live write-mode evals; trusted pass rate is 1 / 4.
- 2026-06-10: fixed Zod fixture checker broadness and verified original fixture still fails.
- 2026-06-10: recorded P0/P1/P2 system gaps and executable follow-up tasks in this ledger.

## Expected Trusted Outcomes After Systemic Hardening

Observed outcomes above are the 2026-06-10 pre-hardening evidence (trusted
pass rate 1/4). The systemic remediation lives in
`docs/design/write_mode_github_issue_p0_systemic_hardening_20260610.md`
(typed TestSurface + dead-end escalation, canonical attempt state + finish
gate, durable failure evidence, risk decomposition, verify-failure handoff,
structured edit hardening). Expected trusted outcomes for this suite after
that delivery:

| Case | Expected | Mechanism |
| --- | --- | --- |
| C libgit2 | PASS | unchanged happy path |
| C++ nlohmann | PASS | body-only public-header patch grades medium and auto-executes |
| Java commons-lang | PASS | missing `mvn` escalates to the Makefile check candidate; failure evidence feeds replan |
| TS Zod | PASS | zero-test python outcome escalates to `make check`; report JSON always persisted |

Re-run with `make eval-github-issues SAMPLES=1` and record fresh observed
results here next to (not over) the pre-hardening rows.

## Live Run Ledger — 2026-06-10 Evening (post-hardening, this clone)

### Round 1: original four cases (binary at `a89ce192`)

| Case | Verdict | Evidence |
| --- | --- | --- |
| C libgit2 | PASS | unchanged happy path |
| C++ nlohmann | FAIL (harness) | plan auto-executed at `risk=medium` (approval decomposition WORKED — last round this blocked as high); both headers patched with the exact `%.*Lg` fix; verify `make check` passed; verdict regex missed only because the ported `run.sh` content reader resolved `git ls-files` paths against the wrong CWD |
| Java commons-lang | FAIL (budget) | verifier picked `make` directly off the rendered test surface (no Maven detour); verify failed on a missing regression assertion; the run died on `global step budget exhausted` mid-recovery |
| TS Zod | FAIL (budget) | same budget shape: after verify #2 failed, the controller spent ~8 exploration rounds (read-only lane rejecting `exec_command`, emit retries), then the third plan — which added the missing regression tests — was APPLIED but the budget died before its verify; blocked → worktree discarded → applied fix lost. `attempt-1.diff` + report persisted (Batch 3 worked); `needs_replan cause=tests_failed` rendered (Batch 2 worked); planner replan opened with the typed failure section both rounds (Batch 5 worked) |

### Round 1 systemic findings → fixes shipped in this clone

1. **F1 budget completion lane (P0)**: an applied-but-unverified batch now
   gets one bounded verify dispatch past the step/turn ceiling before the
   budget verdict (`runBudgetCompletionVerify`, typed attempt-record
   condition). Green verdict completes the run; failure persists evidence
   and blocks as before. Without this, a correct fix applied with the last
   steps was discarded unverdicted.
2. **F2 controller pacing guidance (soft)**: needs_replan + recorded failure
   evidence → prefer replan_batch; exploration costs the same budget the
   remaining verify needs.
3. **F3 live plan mirror (P1)**: `--plan-file` runs mirror every accepted
   plan (and its status/worktree updates) back to the imported file, and
   re-anchor PlanPath there, so the operator-visible artifact and the
   preserve-on-success worktree path follow the LIVE plan across replan
   rounds instead of freezing at the first snapshot.
4. **F4 ExecutedCommand exit codes (P2)**: successful executions recorded
   `exit_code=-1` (extractExitCode(nil)); now 0.
5. **F5 harness reader (P0, eval-only)**: `git -C <wt> ls-files | xargs cat`
   resolved repo-relative paths against run.sh's CWD; restored the
   subshell `cd` so post-apply content is read from the worktree.

### Round 2: four NEW cases from fresh upstream records

| Case | Language / shape | Upstream record | Verdict |
| --- | --- | --- | --- |
| `github_issue_gson_lazy_number` | Java, additive methods (prod+oracle) | google/gson LazilyParsedNumber equals/hashCode gap (historical issue 627) | PASS — `runner=java` unavailable host; model chose make directly |
| `github_issue_dayjs_duration_nan` | JS, prod fix + regression test, package.json manifest | iamkun/dayjs#1611 | PASS — **live runner-missing escalation**: `node exit=127 runner_missing → make (runner_missing_escalation) executed` |
| `github_issue_fmt_tm_year_overflow` | C++, single header, real compile+run | fmtlib/fmt#2564 | PASS — deterministic `-fwrapv` reduction; widening fix verified by execution |
| `github_issue_dateutil_relativedelta_float` | Python, bare dir, stdlib unittest | dateutil gh-411/gh-553 | FAIL→spec — fix correct (`value != int(value)` normalization), `python -m unittest discover` (framework=unittest lane) PASSED, worktree preserved; the case spec had pinned the `is_integer` spelling. Spec broadened to `(is_integer|int[(])`. |

### Confirmed-working mechanisms (live evidence)

- Typed test surface drives runner choice (commons-lang/zod chose make
  immediately; no Maven/pytest detours).
- Runner-missing dead-end escalation end-to-end (dayjs: node→make).
- Python unittest framework selection end-to-end (dateutil).
- Approval decomposition: nlohmann two-header body-only patch auto-executed
  at medium (previously hard-blocked high).
- Canonical needs_replan state + verify-failure handoff lead section render
  in live controller/planner prompts; attempt diffs + reports persist on
  failure.

## Remote Main Recheck - 2026-06-10 (pre-`3a6aef83` observation)

After updating local `main` to `origin/main@a89ce192`, the stashed local
GitHub issue cases and the pre-hardening gap ledger were already present on
main. The current main copy is strictly newer than the stash for this ledger
because it also contains the expected post-hardening outcomes section above.

One local post-update recheck was run before the "do not run eval" instruction.
It should be interpreted as a residual-gap audit, not as a replacement for the
trusted pre-hardening evidence table.

| Case | Local summary result | Deeper typed evidence | Residual gap |
| --- | --- | --- | --- |
| C libgit2 | PASS | Single-file operator-precedence fix applied and verified. | Functional pass, but the generated C formatting compressed `return` onto the `if` line; edit rendering quality still needs tightening. |
| C++ nlohmann | Summary FAIL | `plan-1781105516804877000-99870.report.json` shows `passed=true`, selected `make@.`, and approval was `medium/auto_execute`. | Eval runner read the older `run-1.plan.json`/worktree view instead of the final dynamic-DAG success artifact. |
| Java commons-lang | FAIL | `run-1.plan.json` stayed `pending_approval` even though `affects_public_api=false` and the plan was a two-file source+test bugfix. | Approval policy still over-blocks full-file `modify` plans; low/medium auto-exec intent is not fully honored. |
| TS Zod | Summary FAIL | A later replan `plan-1781105831216715000-5355` passed, selected a real Makefile surface, and the controller emitted `finish/all_verified`. | Eval runner does not follow the final replan artifact; repair also satisfied the fixture by adding marker comments, which is acceptable for the fixture but not an ideal product-quality edit. |

Residual follow-ups observed at `a89ce192`, with the later live-ledger status:

1. Teach the eval runner to resolve the final workflow artifact rather than
   only `run-1.plan.json`. It should follow the latest applied/verified
   plan id, durable report, and preserved worktree ref emitted by the
   controller DAG. Later status: covered by F3 live plan mirror and F5 harness
   reader in `3a6aef83`.
2. Recalibrate approval for source+test plans where typed diff risk is
   medium but the change representation is full-file `modify`; current Java
   case showed this could land in `pending_approval` at `a89ce192`. Later
   status: the live ledger should be treated as the newer source of truth.
3. Add formatting-quality checks for structured/raw edit output so functional
   passes do not collapse adjacent statements onto one line.
4. Keep the typed TestSurface and failure-handoff work: the C++ and Zod
   traces show those mechanisms are now active and useful.

No additional eval runs should be started unless explicitly requested.
