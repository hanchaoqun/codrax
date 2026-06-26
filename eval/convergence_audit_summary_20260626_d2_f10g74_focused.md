# D2-F10g74 Focused Post-Fix Eval Audit

Date: 2026-06-26
Baseline: `0656d4b65` plus local D1-F10g.74 analyzer prescan cutover before commit.

## Result

| case | result | wall(s) | audit |
|---|---:|---:|---|
| `harmony/cangjie_repomap` | PASS | 254 | D1-G118 focused correctness fixed. The final answer used grounded `.cj` source rows; source_inventory rows were supporting coverage, not an unrelated principal universe. |
| `patch_go_typo` | PASS | 191 | D1-G119 focused write verify fixed. The patch changed only `retrun` to `return`; authoritative `go test -json ./...` passed. |

## Manual Findings

- D1-G118 is closed for the focused Cangjie repro: synthetic source-inventory principal row-set projection no longer overrides a disjoint grounded source family.
- D1-G119 is closed for the focused write smoke: probe-authoring/import-boundary issues no longer prevent the project suite from becoming the authoritative verify verdict.
- Residual D1-G67 remains a commercial smoothness gap: Cangjie correctness passed, but the run still spent four analyzer iterations and early grep/list work on support-code string literals before typed source inventory recovered the intended `.cj` source universe. D1-F10g.74 addresses this by allowing bounded analyzer source-inventory navigation and immediately closing prescan on the typed navigation observation.

## Next Validation

Run the next representative batch with six cases, two parallel:

- `qf_relation_subagent_registry`
- `harmony/arkts_repomap`
- `harmony/cangjie_repomap`
- `harmony/trace_query_openharmony_bytrace_thread`
- `read_combo_log_current_source_explanation`
- `patch_go_typo`

Audit dimensions: correctness, repo_map/trace_query first-hop health, excessive grep/list_files, repeated completion/finalizer repair, wall time, context size, and whether handoff preserves evidence without turning navigation rows into hard answer obligations.
