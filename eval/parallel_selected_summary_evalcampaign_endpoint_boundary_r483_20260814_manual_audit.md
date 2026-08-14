# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T11:48:10Z
- sweep_start_ts: 20260814-044808
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | s8a | PASS | eval/results/s8a-20260814-044810 | answer_regex,answer_contains | none | 221s | 29 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=5/0,fin_reject=3,unavail=0,prune=0 | partial | B789 production-positive: endpoint principal block contains only buildAnalysisIR→gate.RunWith@2722 and gate.Run→gate.RunWith@135; all 12 local calls survive in a separate support block. Reject 1 caught the mixed facet, reject 2 followed an ineffective model patch that changed only `sink`, reject 3 corrected definition-line 134 to call-line 135; no contradictory contract. Residual prose says “并行收敛”, although three typed prompt lanes explicitly say a shared callee proves neither parallelism nor convergence. Record as model-adherence variance; no raw-answer hard gate. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-044810 | answer_regex,answer_contains | none | 319s | 35 | read=5,repo_map=2,list=1,trace=0,source_lens=0 | midloop=9,inv=6/1,fin_reject=1,unavail=0,prune=0 | pass | Correct no-directed-path conclusion, exact two-edge shared-callee boundary, separate local-support roster, and two-edge sequence diagram. One reject exposed B790: First-Pass taught display endpoint `agent.buildAnalysisIR` without its typed selectors, while the validator-aligned repair carrier required `from_identity=buildAnalysisIR`; the model copied the system seed and was rejected. Fixed by making First-Pass reuse the same copy-ready typed carrier and sibling `edge_anchors_json` as repair. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2 PASS. Human: 1 pass, 1 partial.
- B789 is production-positive and preserves supporting facts outside the exact principal facet.
- B790 is a precise system teaching/validation self-conflict, not model fluctuation. The initial and repair diagram lanes now consume one typed carrier; visible labels remain model-authored.
- `s8a`'s unsupported concurrency wording is already contradicted by precise prompt evidence. It remains a soft/model-adherence observation because scanning or rewriting final prose would violate the answer-ownership and noisy-signal hard-gate red lines.
- No malformed-JSON recovery, empty answer, stale-draft fallback, or active-stream fixed-age degradation was observed.
