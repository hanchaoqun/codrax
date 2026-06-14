# SDK Contract Mirror

The Python SDK must mirror the Memory API text-search contract:

- Method: `POST`
- URL: `/v1/search`
- Body fields: `query`, `limit`, optional `namespace`.

`GET /v1/memories/search?q=...` is a stale route from an older API version.
