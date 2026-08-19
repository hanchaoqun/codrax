# r713 selected eval manual audit — relation identity and write replay

- baseline: `main@153b49c75`
- sweep: `20260818-195814`
- execution: exactly 2 cases, `PARALLEL=2`, per-case timeout `1800s`
- runner result: `1 PASS / 1 FAIL`
- human result: `1 PARTIAL / 1 FAIL`

| case | runner | human | evidence-backed judgment |
|---|---|---|---|
| `qf_logic_view_read_pipeline` | PASS | PARTIAL | B1130 is production-positive: the finalizer restored two unique one-sided exact relation identity pairs and the prior relation-rejection loop did not recur. The final diagram is legal and preserves analyzer → explorer → extractor → finalizer, BusContext data flow, and the finalizer constructor calls. It nevertheless omits the explicitly requested `Mutable` participant from the diagram, although prose discusses `MutableState`. |
| `github_issue_tokenizers_newline_run_multirepo_py` | FAIL (`plan_not_written apply_not_run`) | FAIL | Exploration completed and the controller correctly reasoned that it should emit `plan_batch`, but it returned prose with no tool call. The protocol retry isolated the next prompt to the correction hint alone, removing the task and typed workflow state that the hint required. The retry therefore emitted `ask_user(reason_code=missing_task_input)` and blocked. |

## Confirmed generalized gaps

### B1131 — write-controller protocol retry deletes its own authority context (P0)

`writeControllerEvaluator.Observe` returns `IsolateNextPrompt=true` for a required-tool no-call. `BaseAgent` then replaces the full message history with only the hint:

> Use only the current typed workflow state …

The typed workflow state is no longer present. Production evidence is exact: the first controller response names `plan_batch` and the implementation target; the second request has only two messages and the model reports `missing_task_input`. The repair must retain the original typed task/workflow/action instruction while replacing the discarded prose with a bounded protocol marker. It must not reconstruct or reuse the discarded model conclusion.

### B1132 — model-invented boundary examples become typed write contracts (P1)

The user and fixture prove only that a five-newline run collapses to one rank token and that ordinary merges remain unchanged. Write analysis additionally minted:

- one isolated newline → one rank token;
- four newlines → two rank tokens.

Both are unsupported, and the second contradicts the requested “consecutive run … collapse to one rank token” behavior. They were accepted into `behavior_contracts` and replayed to the controller as typed context. This is not a newline-specific parser bug: model-authored illustrative boundary cases currently lack a provenance/caliber distinction from user- or test-backed contracts. The generalized fix is to keep unproved examples as planning suggestions until grounded by a request quote, accepted test witness, or tool evidence; they must not acquire verification authority merely by being schema-valid.

### B1133 — requested diagram display identity is lost when source identity is normalized (P1)

The request names `Mutable`; repository navigation finds `MutableState`. Analyzer emitted participant identity/source quote `MutableState`. The exact provenance validator correctly observed that this string is absent from the current request, but it silently dropped the participant while accepting the required diagram. All later participant coverage checks therefore had no obligation to retain `Mutable`, and runner `EXPECT_CONTAINS` passed because prose happened to mention it.

This is a display-identity/source-identity separation gap. A required incident participant with invalid provenance must not disappear behind a warning. The user-authored visible identity must remain the presentation identity; a grounded code symbol may be carried separately as its evidence endpoint/owner. Ambiguous mappings remain unproved. No diagram edge or relation may be created by the system.

## Efficiency and contract audit

- Read used 19 explorer iterations, 5 finalizer iterations and 4 finalizer rejects. B1130 removed the old single-sided identity rejection, but participant repair guidance is still very large and exposes raw internal relation enum labels (`call`, `data_flow`, `precedence`) in the final chart. Reader-facing labels should use the already published business wording; relation enums remain metadata.
- The read answer includes a late “system supplement: output dimension check” that repeats a requirement already satisfied by model prose. It does not alter the conclusion, but remains lower-priority presentation debt.
- Neither case used malformed-JSON recovery or active-stream fixed-age degradation. A live stream still cannot be degraded because no complete answer exists after 4ms or any other fixed cumulative age.

## Frozen order

1. B1131: repair the protocol context contradiction and pin the second request structurally.
2. B1133: preserve exact requested display identities separately from grounded source identities; required participants cannot be silently lost.
3. B1132: add provenance/caliber to write behavior contracts and prevent unproved examples from entering verification authority.
4. Replay the same two cases exactly in parallel, then continue with the next higher-priority read/write/trace pair.

Trace query, explicit-window causal projection, deterministic supplementation, typed on-chain root-cause authority, and model conclusion ownership were not changed by this audit.
