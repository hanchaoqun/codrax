# REPL Output Artifact Request Expansion

## Problem

Interactive REPL paste folding is a UI feature: long pasted input can be
shown as a compact placeholder such as `[Pasted text #0 ...]` while the
pipeline receives the expanded text. The final-answer artifacts under
`.codrax/output` are not UI chrome; their `# 问题` section must contain
the expanded current request so the saved Markdown, standalone HTML, and
browser preview remain auditable.

## Root Cause

The REPL already tracks both forms:

- `line`: placeholder-expanded text dispatched to the pipeline.
- `display`: placeholder-folded text persisted to memory to keep future
  prior-conversation context compact.

Pipeline output dumping previously reconstructed the request from the
orchestrator objective with `StripConversationPrefix`. That is indirect:
the objective may contain prior-conversation scaffolding, and callers
outside the REPL boundary do not encode whether the text came from the
expanded request or the folded display.

## Contract

The REPL now passes the expanded current request to the runner through a
small optional `SetOutputTranscriptRequest` capability. The orchestrator
uses it only for final-answer Markdown/HTML transcript generation and
falls back to the historical objective-derived request when the setter is
not used.

This preserves all existing behavior outside the artifact path:

- model context still receives the effective pipeline request;
- memory still stores the folded display with expanded
  `RequestForSummary`;
- local/chitchat replies already write output artifacts from the expanded
  request;
- the HTTP preview serves the same Markdown artifact, so it inherits the
  expanded `# 问题` section automatically.

