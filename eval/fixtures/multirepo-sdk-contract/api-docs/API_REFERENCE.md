# Memory API Reference

## Text search

Text search accepts a JSON request body.

- Method: `POST`
- URL: `/v1/search`
- Body fields:
  - `query`: string
  - `limit`: number
  - `namespace`: optional string

Legacy `/v1/memories/search?q=...` routes are not served by this API version.
