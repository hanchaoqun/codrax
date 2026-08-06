# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T11:50:15Z
- sweep_start_ts: 20260806-045014
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260806-045015 | write_apply,write_patch_oracle,answer_contains | none | 90s | 20 | read=2,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exactly one `main.go` line changed (`retrun` → `return`). The isolated post-apply report ran `go test -json ./...`, covered `main.go` with `project_runner/target_behavior`, passed `TestGreet`, and closed `verified`. No plan/schema/finalizer rejection, JSON recovery, unrelated edit, or main-worktree mutation. |
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260806-045015 | typed_inventory_rowset,dimension_substring,answer_contains | none | 256s | 24 | read=8,repo_map=2,list=0,trace=0,source_lens=2 | midloop=6,inv=1/0,fin_reject=5,unavail=0,prune=0 | fail | Evidence and final table contain the correct 2 extend, 2 foreign-func and 8 public-class rows with all paths/packages. The answer nevertheless says `public class 共 10 个` and duplicates the full roster in three required prose sections plus one table. Five deterministic finalizer rejects expose two contract gaps: mixed-family blocks reject prompt-visible non-global `extend` row IDs, and comparison section requirements compete with the principal-row carrier teaching. The runner's `got20` is partly a duplicated-surface symptom, but human correctness is still fail because the visible count is wrong and the answer is unnecessarily repetitive. B176-S2 is healthy (`blocks_string_recovery=0`); B176-S1/S3 are only partial on mixed-family partitions. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Adjudication

- `EVAL-B176-MIXEDROW1=P1/red-line-contract-conflict`: `Principal Enumeration Rows` exposes partition IDs for `extend String/Cart`, but a mixed-family principal table is validated only against the synthetic global source-inventory roster. Copying the visible IDs is rejected; removing them is accepted. Mixed blocks must admit every typed prompt-row alias while explicit family blocks remain partition-closed.
- `EVAL-B176-BUCKETCARRIER1=P1/high-churn-contract-conflict`: the comparison contract requires three `section` blocks, while the principal-row teaching says rows must be in `ordered_list/bullet_list/table`; the repair recipe separately says `section` can carry rows. This encourages prose roster duplication plus an extra table and produced five retries. One typed rule must say that each required bucket section may carry its own `items[]` rows and thereby satisfy enumeration coverage; no second global table is required.
- `EVAL-B176-COUNT1=P1`: typed aggregate rows prove 8 public-class members, but model prose says 10. Fix through typed aggregate-value guidance/neutral validator metadata, not output-text keyword or number scanning. First remove the conflicting duplicate carrier; replay before adding a new hard gate.
- `EVAL-B175-CITREF1=partial`: the new unique-display-row binder did not run for `extend` in the mixed block because its row universe inherited the same global-only restriction. The final accepted draft still logs an `extend Cart` citation advisory. Reuse the corrected mixed typed-row universe; do not add a Cangjie-specific matcher.
