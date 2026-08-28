# Selected Eval Manual Audit

- date: 2026-08-28T18:10:38Z
- sweep_start_ts: 20260828-111037
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- binary revision: `2b6ce9c89423`
- results_root: eval/results

| # | case | runner | result_dir | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|--------|------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | log_path_question_multi_runtime_files | PASS | eval/results/log_path_question_multi_runtime_files-20260828-111038 | 194s | 29 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0 | pass | B1392 is measurement-positive: folded co-occurrence recognizes the same structured multi-paragraph answer without forcing facts onto one line. The answer preserves both observed errors and all four stack frames/locations. This sample does not add receiver ownership, downstream slowness, network, or timeout-policy mechanisms; paired with r896, that confirms the remaining mechanism expansion is model variance rather than a missing system fact that should be hardened through prose scanning. |
| 2 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-111038 | 188s | 27 | read=11,repo_map=1,list=0,trace=0,source_lens=1 | midloop=8,inv=8/0,fin_reject=0 | partial | The model's principal explanation and precedence table cover code default 50, YAML overlay, and explicit CLI precedence. Two independent gaps remain. First, system-owned source supplementation appended six rows; only PipelineMaxSteps/pipeline-max-steps were relevant, while rootPreRun, enforceStdinExclusivity, cliRuntimeAnalysisKickoffLines, and rsMCPServers were unrelated same-file evidence. The trigger treated a visible `cmd/root.go` path as authority for all accepted rows in that file. B1393 narrows this to exact typed identities/surface terms/load-bearing summaries. Second, B1390 remains confirmed: the model made several accepted completion calls without `requested_dimension_indices`, then tried aggregate facts even after the typed remedy explicitly asked for evidence metadata; it needed 23 explorer iterations and 11 reads before emitting two indexed operation rows. The final prose also says an explicit CLI value becomes `mergedMaxSteps`, although the source retains it in `flagMaxSteps`; treat this as model imprecision, not permission for system answer rewriting. |

## Decision

- `B1392`: production measurement-positive/core-closed.
- `B1393-SAMEFILESOURCESUPPLEMENT1`: P1, confirmed and implemented; exact symbol/surface authority only, no file-wide expansion.
- `B1390-DIMENSIONOWNERSHIPFIRSTEMISSION1`: still P2/high-ROI process debt; solve through typed roster/schema teaching before the first completion attempt, not request/final-answer keyword gates.
- No production code in this sweep modified Trace selection, projection, supplementation, ranking, or stream completion.
