# MCP Typed Line Support 2026-05-31

## Problem

MCP output already reaches Codrax through `MCPResponse -> ObservationLedger`
with `origin=mcp_resource`, and `MCPResponse` already has coordinate fields
such as `line_start`, `line_end`, `row`, `json_pointer`, `selector`, and
`resource_uri`. The missing piece is producer-side decoding: stdio MCP tool and
resource results are currently flattened into `Summary`/`PayloadRef`, so a
well-behaved MCP server cannot hand Codrax structured line-backed facts.

Codrax must not solve this by scraping numbers from arbitrary text. That would
turn noisy external prose into hard coordinates and could create false evidence.

## Red Lines

1. Empty `mcp_servers` keeps the current pipeline unchanged.
2. MCP typed coordinates remain external observations. They must not enter the
   current-source citation/ground pool.
3. Ordinary text and ordinary JSON are not parsed for line numbers. Typed line
   support activates only for an explicit Codrax observation envelope or a
   dedicated MIME type.
4. Resource reads still use exact enumerated URIs only. Codrax never constructs,
   resolves, or opens MCP URIs by itself.
5. Invalid typed coordinates degrade to untyped summary output; they do not
   crash the stage or become hard gates.

## Envelope

MCP servers may optionally return a JSON envelope in `tools/call` content or
`resources/read` text/blob content:

```json
{
  "version": "codrax.mcp.observation.v1",
  "summary": "scheduler wakeup found",
  "resource_uri": "mcp://trace/run/attached",
  "line_start": 1139180,
  "line_end": 1139180,
  "selector": "pid=36379 event=sched_wakeup",
  "json_pointer": "/events/42"
}
```

Batch form is also allowed:

```json
{
  "version": "codrax.mcp.observation.v1",
  "summary": "trace window evidence",
  "observations": [
    {
      "summary": "sleep entry",
      "line_start": 1102717
    },
    {
      "summary": "wakeup",
      "line_start": 1139180
    }
  ]
}
```

The preferred MIME type is `application/vnd.codrax.observation+json`. A generic
`application/json` payload is parsed only when the `version` field is present.

## Design

- Add `types.MCPTypedObservation` and `MCPResponse.Observations`.
- Extend `compileMCPResponseObservations`:
  - no `Observations`: preserve the existing single-record behavior;
  - with `Observations`: emit one ledger record per typed observation, using
    parent server/method/blob refs as defaults.
- Extend stdio result decoding:
  - decode text/data/resource content as before for `Summary`;
  - additionally inspect explicit Codrax observation envelopes;
  - replace raw envelope text in `Summary` with compact typed summaries;
  - never infer coordinates from non-envelope text.
- For `resources/read`, inherit the read URI when a typed observation omits
  `resource_uri`.
- Keep large output blob handling in the existing agent layer. If a parent
  `MCPResponse.PayloadRef` is added after decoding, ledger projection inherits
  it for typed observation rows that do not specify their own payload ref.

## Task Checklist

- [x] Add typed observation type and ledger projection.
- [x] Add envelope parser with explicit activation rules.
- [x] Wire parser into `tools/call` and `resources/read` decode paths.
- [x] Add tests for single observation, batch observations, resource URI
      inheritance, ordinary JSON non-activation, invalid coordinate downgrade,
      and ledger external-origin preservation.
- [x] Update MCP docs and sample comments.
