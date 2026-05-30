# Answer Document Item Fragment Recovery

## Customer Signal

On 2026-05-30 a finalizer emitted two `emit_answer_document` calls. The first
payload contained a valid ordered-list block, but `blocks[].items[]` also
included stream-corrupted scalar fragments such as brace/array closers between
real item objects. Strict decoding rejected the whole answer with:

`json: cannot unmarshal string into ... blocks.items`.

The old compatibility path only pruned scalar item fragments when the block had
non-empty block-level `text`. In this case the user-visible answer was carried
by valid item objects (`label` + `text`), so the safe recovery condition was too
narrow.

## Red Lines

- Do not rewrite model answer semantics.
- Do not drop standalone prose from `items[]`.
- Do not turn malformed strings into invented answer items.
- Keep full `emit_answer_document` and `emit_answer_document_patch` behavior in
  lock-step because both share the same nested block repair path.

## Fix Design

The recovery path now prunes scalar `items[]` fragments only when:

1. At least one valid item object remains.
2. Visible content is already preserved by block-level `text` or by kept item
   object fields (`text`, `label`, or `cells`).
3. The discarded scalar is non-display structural noise, such as JSON
   punctuation fragments or a `citation_ref:<n>` sidecar.

Meaningful scalar prose still fails strict decoding and asks the model to
re-emit a valid payload. This keeps recovery local to the typed answer carrier
and avoids modifying the original table/list content.

## Task Checklist

- [x] Reproduce full `emit_answer_document` ordered-list item fragment failure.
- [x] Add a negative test for meaningful scalar prose without a visible carrier.
- [x] Generalize the shared item-fragment repair guard.
- [x] Run focused and full tests before push.
