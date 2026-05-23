# External Observation Eval Skeletons

These are intentionally **not** `.case` files yet because the current eval
runner has no first-class web/MCP/connector attachment knobs. They become
executable when those producers can feed typed `ObservationLedger` records.

## MCP JSON Field Exists + Current Code

- Input: fake MCP resource `mcp://docs/spec` with `payload_ref`,
  `row_set_ref`, `json_pointer=/items/0/title`, and one matching field.
- Question: "基于 MCP spec 里的第一个 item，再结合当前源码解释对应实现链路。"
- Must preserve: MCP `resource_uri`, `json_pointer`, `row_set_ref`, plus current
  source citations for implementation only.

## MCP JSON Field Absent

- Input: same fake MCP resource with no `Deprecated API` member.
- Question: "MCP spec 里是否还存在 Deprecated API？说明检索范围。"
- Must preserve: `origin=mcp_resource`, `result_count=0`, `scope`, and no
  current-source citation pressure.

## Web Paragraph Contains + Current Code

- Input: fetched web page with `url`, `fetched_at`, `selector`, `paragraph`.
- Question: "网页里的 API contract 对当前实现有什么影响？"
- Must preserve: web URL/selector/paragraph as external support; current code
  citations only for checkout implementation.

## Connector Row Exists / Absent

- Input: connector response with `connector=jira`, `resource_uri`, `row`.
- Questions:
  - "Jira 里 ISSUE-7 的结论和当前实现是否一致？"
  - "Jira 里是否存在 JIRA-404？说明检索范围。"
- Must preserve: connector resource coordinates and zero-result facts without
  inventing repo citations.
