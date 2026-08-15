# Selected Eval Manual Audit

- date: 2026-08-15T21:46:47Z
- sweep_start_ts: 20260815-144646
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | runner | human | result_dir | audit |
|--:|------|--------|-------|------------|-------|
| 1 | sr_java_call_chain | FAIL | partial | eval/results/sr_java_call_chain-20260815-144647 | B855 is production-positive: the system copy-ready sequence now starts at `VisitController.create`, keeps the three `VisitService.schedule` siblings in grounded source-line order, and ends at `AuditLog.record`; no relation was created, removed, or reversed. The model independently renumbered the same true graph and preserved the readable order, but attached all `edge_anchors` to the ordered-list block rather than the diagram block. The first tool submission surfaced only missing answer dimensions; the second patch then exposed the already-present diagram-anchor violation and stopped with no repair turn left. The final answer is useful but still calls stdout “审计落库/完成审计记录” without the required durability-negative boundary, so the runner failure remains legitimate. |
| 2 | github_issue_chrono_duration_min | FAIL | partial | eval/results/github_issue_chrono_duration_min-20260815-144647 | The implementation is correct and applied in one plan: `try_milliseconds`, the panicking wrapper, direct MIN/MAX constants, and regression tests are present. Only one verifier generation ran. B856 is production-positive: no cumulative-review batch or repeated static verifier was minted; finish was normalized directly to honest `unverified:production_verification_source_static_only`. Wall time fell from 388s in r530 to 235s. The fixture still has no native Rust/Cargo execution proof, so human status remains partial and the verification bar must not be lowered. |

## Gap classification

- `B855-SEQUENCEEVIDENCESLICEORDER1`: production closed in r531.
- `B856-SOURCESTATICVERIFYREPEAT1`: production closed in r531.
- `B857-ANSWERDOCVALIDATIONWATERFALL1` (P1): one draft contained both a requested-dimension omission and diagram-local anchor ownership error, but feedback was serialized across turns. The model also received a general block schema that permits `edge_anchors` on a non-diagram block while the semantic contract requires diagram-local ownership. The repair must aggregate typed violations or make block ownership unambiguous; it must not inspect answer prose, move fields on the model's behalf, delete the graph, or synthesize relations.
- Java stdout durability wording remains a model-authoring partial under already-supplied exact source evidence. Do not add a prose keyword hard gate or system-authored correction for this single oracle.
