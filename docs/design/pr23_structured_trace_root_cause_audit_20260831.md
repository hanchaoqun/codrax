# PR #23 structured Trace root-cause sidecar audit

Date: 2026-08-31  
PR: `https://github.com/hanchaoqun/codrax/pull/23`  
Original commit: `ea525ad937f73c32aed197b5050a8faac1717816`  
Original author: `keruoya <keruoya@users.noreply.github.com>`  
Merge commit: `80b6e0425` (two-parent, non-squash merge)

## 1. Scope and conclusion

The change adds a structured Trace root-cause sidecar, typed candidate compilation,
final-answer artifact plumbing, and `.root-causes.json` retention. The direction is
useful: the compact machine-readable result is separate from the full Markdown/HTML
answer, and output cleanup understands the sibling file.

The PR could not be enabled unchanged. Four architecture gaps survived review:

| ID | Severity | Finding | Resolution |
|---|---|---|---|
| PR23-A | P0 | A missing or semantically invalid `trace_root_causes` field rejected the whole `emit_answer_document` transaction, so an optional sidecar could erase a useful full answer. | Sidecar is optional. Absence is accepted; sidecar-only decode/binding failure is logged and ignored while the model answer remains eligible. Patch failure preserves the last accepted sidecar. |
| PR23-B | P0 | The model could freely author category, thread/resource/phase identity, positive impact and evidence prose without binding any typed candidate or evidence receipt. A background or fabricated number could therefore become a structured root cause. | Input now accepts only an ordered list of exact candidate IDs from a schema enum. Runtime binds every semantic field, magnitude and evidence from the frozen contract. Only typed `on_chain` candidates with representable per-thread wall-clock duration are selectable. |
| PR23-C | P0 | `BuildDeterministicFinding` selected the first eligible ranked candidate as `PrimaryCause` after model completion. That made system ranking a conclusion authority. | Automatic primary-cause selection was removed from finalizer execution. The legacy builder is fail-closed unresolved; model candidate choice and order own the conclusion. |
| PR23-D | P1 | The new atomic artifact setter duplicated the answer-success epilogue and omitted cleanup of the pending patch base and relation-repair lease. Three existing tests reproduced stale retry state after a successful emit. | Full and multi-artifact setters now share one locked success epilogue that clears all retry-local state. |
| PR23-E | P1 | The PR added a `TraceFindingRequired` field and orchestrator setter described as a CLI switch, but no CLI or other caller invoked it and the default activation path did not read it. The eight dead lines also failed the orchestrator hot-file ratchet. | Removed the unused pseudo-configuration surface instead of raising the ratchet budget. Runtime activation remains derived from typed Trace request shape and candidate availability. |

## 2. Preserved behavior

- The full answer remains model-authored and is never replaced, summarized, or
  rewritten by the sidecar path.
- Explicit-window Trace causal projection and system evidence supplementation are
  unchanged.
- Root-cause admission reads typed projection fields only. It does not scan user
  input, model reasoning, final prose, Markdown, or Mermaid labels.
- Adjacent/background rows remain available to the long answer as support, but are
  absent from the root selector.
- Model owns how many candidates to select and their strongest-to-weakest order.
  The system owns only exact fact binding and stable serialization.
- Count, composite-score, and cross-thread CPU-ms values are not relabeled as
  `impact_seconds`.

## 3. Output contract after remediation

The optional model input is deliberately small:

```json
{
  "schema_version": 2,
  "root_causes": [
    {"candidate_id": "candidate-..."}
  ]
}
```

Each candidate ID is exposed by the per-dispatch schema from the exact typed
on-chain roster. The persisted `.root-causes.json` retains the PR's public v2 form:
rank, category, subject/resource/phase identity, impact seconds, stable summary and
evidence. Internal candidate IDs are cleared before persistence.

## 4. Remaining bounded gaps

1. The public category vocabulary still folds semantic CPU-work families without a
   dedicated category (for example class verification, runtime compilation and
   texture upload) into `phase_high_load`. The long answer and Trace projection keep
   the exact semantic class, so no evidence is lost, but the compact sidecar taxonomy
   is less specific. Extending the public schema requires a versioned compatibility
   decision rather than silently changing v2 labels.
2. Cross-thread compute-delivery aggregates are deliberately excluded because their
   unit is CPU-ms rather than wall-clock seconds. Per-thread low-frequency wall-clock
   candidates remain representable. A future aggregate field needs an explicit unit,
   not conversion into `impact_seconds`.
3. Syntactically invalid JSON for the entire tool call is handled by the existing
   answer-document recovery path. This batch makes any syntactically valid but
   malformed sidecar shape non-blocking; it cannot recover bytes the provider never
   delivered as a parseable tool call.

## 5. Verification obligations

- candidate compiler: exact on-chain admission; background/adjacent negative arms;
- selector binder: model semantic-field spoofing is ignored; candidate ID/order are
  the only model-owned values;
- full emit: invalid optional selector does not reject or alter the full answer;
- success epilogue: accepted full and patch emits clear relation lease and staged
  patch base;
- hot-file ratchet: no dead CLI switch or budget increase;
- schema: sidecar property is optional and candidate IDs are enum-bound;
- repository-wide Go tests and build must pass before push.

Verification result: `go test ./... -count=1` passed; the focused finalizer
prompt tests passed after the compact roster projection; `make` passed.

## 6. History preservation

The PR was merged with a regular two-parent merge. Commit `ea525ad93` remains
reachable unchanged with the original author identity; remediation is recorded as a
separate descendant commit so review can distinguish imported work from audit fixes.
