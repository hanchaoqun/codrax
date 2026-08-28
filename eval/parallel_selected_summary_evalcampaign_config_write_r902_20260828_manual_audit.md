# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T20:16:33Z
- sweep_start_ts: 20260828-131633
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260828-131633 | write_apply,write_patch_oracle,answer_contains | none | 93s | 26 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One-line `kind=patch` changed only `main.go:25`; apply, same-package verification probe, current-plan report, and final workflow verdict all passed. No replan or JSON repair occurred. The probe's nested `go build ./...` created an untracked `patch_typo_fixture` binary; the system correctly excluded it and disclosed retention, but the predictable build side effect is a separate P2 cleanup/usability gap. |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-131633 | answer_regex,answer_contains | none | 177s | 27 | read=21,repo_map=1,list=0,trace=0,source_lens=1 | midloop=10,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail | B1401 fired: the first completion could not close explanation seats with definitions and the model added grounded `SetMaxSteps` call plus CLI `Changed` condition. It still never read `LoadRuntimeSettings`/`yaml.NewDecoder(...).KnownFields(true)`/`Decode`; final prose falsely called YAML a Go standard-library parser. An early ungrounded definition-shaped `pipeline-max-steps` row was stamped as a durable evidence-subject denial despite a grounded `IntVar` call on the same source line, so L3 emitted `ViolDeniedTokenUndeclared`; the shared consistency family then appended the false generic “答案前后某些表述存在不完全一致” note. B1402 stayed positive: no enumeration-label supplement. Finalizer collected 97 evidence rows and cited 4. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed gaps and disposition

- `B1401-DEFINITIONMECHANISMOPERATION1`: production-positive for the exact bypass it fixed. Definitions no longer close operation seats; however an executable anchor from the wrong responsibility can still close a semantically different explanation dimension, tracked separately as B1405.
- `B1402-ENUMSUPPLEMENTFALSEDOUBT1`: production-positive. No false enum-label supplement was emitted for grounded config/CLI labels.
- `B1403-GENERICCONTRADICTIONTELEMETRY1`: unit-positive for unpaired `ViolSelfContradiction`, but the user-visible symptom is not globally closed. `ViolDeniedTokenUndeclared` shares `CaveatFamilyConsistency`, so a stale denial still emitted the same generic wording through a distinct typed family member.
- `B1404-STALEEVIDENCESUBJECTDENIAL1/P1`: `stampUngroundedEvidenceDenials` turns each final ungrounded row into append-only global negative knowledge without reconciling later/current grounded source support. One failed claim shape therefore censors a real config literal and produces a false user caveat. Repair must narrow the claim, not scan answer/request prose.
- `B1405-DIMENSIONRESPONSIBILITYOPERATION1/P1`: operation authority is now executable, but the parsing-mechanism dimension is not bound to its required responsibility/source operation. `SetMaxSteps`/merge guards cannot prove the YAML decoder implementation. Use analyzer-owned typed dimension responsibility plus exact executable evidence; do not hard-code YAML or this config key.
- `B1396-SAMEFILEFINALIZERCONTEXT1/P2`: independently reproduced at 97 collected rows / 4 citations. The finalizer received 70 additional shared rows, including structurally derived `Decode`, without load-bearing selection.
- `B1406-VERIFICATIONBUILDSIDEEFFECT1/P2`: a same-package probe that runs `go build ./...` for a main package predictably leaves a binary in the retained worktree. Current exclusion/disclosure is safe; a future generalized fix should isolate or explicitly own predictable probe outputs rather than auto-delete unknown files.
