# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T19:22:24Z
- sweep_start_ts: 20260828-122223
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260828-122224 | write_plan,write_patch_oracle | none | 46s | 26 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan-only write lane stayed bounded to `main.py:20`, emitted one `kind=patch` unified diff plus one exact replace edit (`retrun` → `return`), and supplied a bounded import/`greet("Test")` verification probe. No unrelated path, JSON repair, replan, or contract rejection appeared. |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-122224 | answer_regex,answer_contains | none | 148s | 31 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B1398/B1399 are production-positive: the final answer contains neither the fact-free requested-dimension supplement nor the false generic inconsistency caveat. Runtime fell from r899's 251s/24 reads/8 completion attempts/1 finalizer reject to 148s/8 reads/2 attempts/0 rejects. However the principal summary falsely says the setting is parsed through “Viper/Cobra”; the repo has Cobra plus `gopkg.in/yaml.v3`, and the real parser is `LoadRuntimeSettings` → `yaml.NewDecoder(...).KnownFields(true)` → `Decode`. The explorer never read that body. Its dimension-1 ownership was falsely satisfied by root.go assignment/CLI operations carrying `[1,3]`. Finalizer still received 82 evidence rows and cited only 3. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed gaps and disposition

- `B1397-POSTEMITADVISORYTAIL1/P1`: structural head-placement tests remain green, but this replay did not naturally fire the advisory because the first evidence batch already put operation rows under both explanation indices. Record production no-regression, not natural-fire confirmation.
- `B1398-REQUESTDIMENSIONSOURCEQUOTEBACKFILL1/P1`: production-positive; no system-authored requested-dimension appendix survives.
- `B1399-RESOLVEDPREFLIGHTCONSISTENCYCAVEAT1/P1`: production-positive; resolved pre-completion telemetry no longer becomes a user-visible contradiction caveat.
- `B1400-DIMENSIONFILESCOPEOWNERSHIP1/P1`: the typed dimension-ownership gate knows that a row is an operation and knows its requested dimension index, but not which source responsibility/file must prove that dimension. Consequently unrelated merge/CLI operations can self-declare ownership of the parsing-mechanism dimension. The generalized repair is typed dimension-to-required-file ownership plus dimension-local operation closure; it must not inspect or rewrite final prose.
- `B1396-SAMEFILEFINALIZERCONTEXT1/P2`: still open and independently confirmed at 82 collected evidence rows / 3 cited rows. Selection must use typed accepted ownership and load-bearing rows, not file-name or prose similarity.
