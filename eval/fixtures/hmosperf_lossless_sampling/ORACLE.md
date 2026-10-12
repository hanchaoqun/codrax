# Independent audit oracle — not attached to the model

The requested window is [1, 2) seconds and native sample TID is 11. Six recorded samples exist, five are in-window; the 3.0-second sample (weight 900) is outside. No scheduler duration or causal dependency is established by this fixture.

## Populations and paths

| Producer / event | In-window samples | Full weight | Exact-stack weight | Unavailable-stack weight |
| --- | ---: | ---: | ---: | ---: |
| raw_perfdata_fallback / cycles | 3 | 35 | 30 | 5 |
| raw_perfdata_fallback / instructions | 1 | 100 | 100 | 0 |
| simpleperf_report_sample / cycles | 1 | 7 | 7 | 0 |

Each exact stack has two frames, root to leaf. `sys;root` is one literal symbol in `/system/lib.so` at `0x1`; the semicolon does not split it. The two `work` children retain distinct libraries and addresses: `/app/A.so` at `0x2`, `/app/B.so` at `0x3`. Frames retain source references and original depth. These are sampled prefix relationships, not source-code calls or measured invocation order.

For raw cycles, `sys;root` inclusive weight 30 / full cohort 35 = 85.714286%, self 0. A child inclusive/self 10 / 35 = 28.571429%; B child inclusive/self 20 / 35 = 57.142857%, the larger observed branch. The unavailable legacy stack contributes 5 / 35 = 14.285714%; do not normalize visible stacks to 30. Its delimiter-only text is not sufficient to recover exact frame boundaries. No hidden node is invented.

Instructions and the independent simpleperf cycles each have one observed stack; root and A child inclusive shares are 100% in their own cohorts. Do not combine events/producers, infer CPU time, multiply shares by the window to create milliseconds, or claim statistically representative workload hotness from these few samples. No zero weight replaces unknown frame information. Expansion, folding, node/reference limits must disclose omissions without changing full-population denominators or deleting system frames.

## Acceptance

Inspect actual preparation/query arguments and full payloads, finalizer handoff, principal answer, complete output and diagrams. A machine pass alone does not establish tree-tool use, correct cohort denominators, retained source identity or complete-answer quality. Missing direct tree invocation or misleading prose is recorded separately from deterministic implementation tests. Exact generated node IDs are not pinned; they should not replace reader-facing function/library labels.
