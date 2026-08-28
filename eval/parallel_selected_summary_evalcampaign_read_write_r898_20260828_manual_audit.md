# Selected Eval Manual Audit

- date: 2026-08-28T18:33:49Z
- sweep_start_ts: 20260828-113348
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- binary revision: `770e62ed7901`
- results_root: eval/results

| # | case | runner | result_dir | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|--------|------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-113349 | 235s | 28 | read=13,repo_map=1,list=0,trace=0,source_lens=1 | midloop=11,inv=5/0,fin_reject=0 | partial | B1393 is production-positive: the final answer contains only the five relevant config anchors and no system appendix containing rootPreRun/enforceStdinExclusivity/cliRuntimeAnalysisKickoffLines/rsMCPServers. B1390 is only partially positive. The typed ownership guide was present before first completion, but the first evidence batch attached indices `[1,3]` to the PipelineMaxSteps definition and omitted indices on the actual YAML/CLI condition operations; completion still had to request metadata repair. Calls/iterations improved from r897's 8/23 to 5/20, while reads rose from 11 to 13. A precise post-emit advisory can flag this typed mismatch before completion without rejecting or rewriting evidence. The principal answer correctly explains default 50, YAML overlay, explicit CLI precedence, and SetMaxSteps. Two system-owned appendices are false: a generic precedence comment was accepted as uncertainty-boundary evidence and produced “已核查范围与未确认边界”, while an earlier config-precedence facet softening remained sticky even though the final table declares that facet. The finalizer prompt still carried dozens of unrelated same-file concrete-value rows; output filtering is fixed, context selection remains open. |
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260828-113349 | 69s | 26 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0 | pass | The plan reads the real Python owner, emits exactly one `main.py` patch, changes only `retrun` to `return`, produces an applicable unified diff, and carries bounded import/greet/main verification. There are no JSON recovery events, plan rejects, replans, finalizer rewrites, or unrelated files. B1390 does not activate for this single write-plan dimension. |

## Decision

- `B1393-SAMEFILESOURCESUPPLEMENT1`: production-positive/core-closed on the user-visible supplement path.
- `B1390-DIMENSIONOWNERSHIPFIRSTEMISSION1`: partial production-positive; prompt arrival is proven, but first operation-row ownership is not. Open `B1394-DIMENSIONOWNERSHIPPOSTEMITADVISORY1` as a precise soft advisory after evidence emission, never a hard reject or text scan.
- `B1395-UNCERTAINTYFACETAUTHORITY1`: P1. Narrow uncertainty-boundary candidate binding to typed absence/external/drift/conditional authority; an ordinary text-reference comment cannot mint a user-visible missing-boundary caveat. Resolve sticky facet-softening disclosure against the outgoing document's typed facet declarations.
- `B1396-SAMEFILEFINALIZERCONTEXT1`: P2. B1393 fixed visible output, but the finalizer still received unrelated same-file evidence; narrow context selection separately without deleting accepted evidence or using answer prose.
- The write-mode control is clean. No change in this sweep modified Trace explicit-window selection, causal projection, automatic supplementation, on-chain ranking, business clues, or actual/eliminable ledgers.
