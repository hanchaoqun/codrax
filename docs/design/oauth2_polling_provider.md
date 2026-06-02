# OAuth2 Polling Provider Integration

## Problem

Some internal deployments expose OpenAI-compatible chat completions through a
company OAuth2 flow rather than a static API key. A representative deployment
has these differences from the current Codrax provider path:

- authorization-code-like polling with a generated `client_code`;
- token response fields such as `access_token`, `refresh_token`,
  `expires_in`, `token_type`, `scope`, and user identity metadata;
- request auth through a provider-specific token header rather than
  `Authorization: Bearer`;
- required request headers such as application id and per-request trace id;
- custom chat and model-list paths;
- request body extras such as queueing or provider-side streaming switches.

The chat payload and SSE chunks remain OpenAI-compatible enough that duplicating
the existing `OpenAIAdapter` would create avoidable risk. The safest design is
to keep the adapter's message/tool/SSE logic and add a provider-auth and
request-decoration layer.

## Non-goals

- Do not add model-facing tools or tool parameters. This is provider
  configuration only, so it must not affect tool JSON compatibility logic.
- Do not store tokens in `providers.yaml`.
- Do not auto-connect to a public endpoint when required fields are missing.
- Do not duplicate the OpenAI-compatible SSE/tool-call parser.
- Do not change repo, trace, MCP, or source-evidence behavior.

## Design

### Configuration

`llm.default` and every `llm.agents.<name>` entry may add:

- `chat_completions_path`: default `/chat/completions`.
- `models_path`: optional model-list endpoint.
- `auth`: optional auth block.
- `headers`: static/generated request headers. `@uuid_v4` generates a fresh
  UUID per request.
- `request_extra`: JSON-compatible top-level chat body extras. Reserved fields
  (`model`, `messages`, `tools`, `tool_choice`, `stream`, `max_tokens`,
  `thinking`) cannot be overridden.

Generic example:

```yaml
llm:
  default:
    provider: openai
    base_url: https://your-model-gateway.example.com/api
    chat_completions_path: /chat/completions
    models_path: /models

    auth:
      mode: oauth2_polling
      auth_base_url: https://your-sso.example.com/oauth
      client_id: your-oauth-client-id
      scope: "your-scope"
      response_type: code
      scope_resource: "your-scope-resource"
      authorize_path: /oauth2/authorize
      callback_path: /oauth/callback
      token_path: /oauth/getToken
      access_token_header: X-Auth-Token
      access_token_format: "{token}"
      refresh_before_seconds: 300

    headers:
      app-id: "your-app-id"
      x-snap-traceid: "@uuid_v4"

    request_extra:
      queue: true
      tool_stream: true

    # If model is omitted, Codrax fetches models_path once and selects the
    # first model, printing the model list and selected model to the UI/log.
```

### Token lifecycle

`TokenManager.Get(ctx)` returns a reusable token:

1. Use in-memory token if `now < expires_at - refresh_before`.
2. Load the secure cache file and reuse if valid.
3. Otherwise generate `client_code`, print an authorization URL, and poll the
   token endpoint.
4. Save token cache with `0600` file permissions under a `0700` directory.
5. Parse `expires_in` as seconds, accepting both JSON strings and numbers.
6. If a chat/model request returns `401` or `403`, invalidate the token and retry
   once after acquiring a fresh token.

The cache fingerprint binds the token to the provider identity:
`base_url`, `auth_base_url`, `client_id`, `scope`, `scope_resource`, and token
path. A config change cannot silently reuse an incompatible token.

### Model selection

Rules:

1. If a resolved provider has `model`, use it.
2. If `model` is empty and `models_path` is set, request the model list with the
   same auth/header decoration, select the first non-empty model, and show the
   list plus the selected model.
3. If `model` is empty and model-list discovery fails or is not configured,
   startup fails with an actionable message.

This deliberately avoids complex ranking. Operators who need stable production
routing should configure `model` explicitly.

### UX

Keep output concise and consistent with existing provider/TLS startup messages:

- OAuth prompt is printed to stderr/log only when auth is required, and follows
  the configured UI language.
- Model-list auto-selection is printed once per provider identity, and follows
  the configured UI language.
- Tokens and refresh tokens are never printed.
- Final Markdown stdout remains clean in CLI mode.

### Safety

- HTTPS is expected for OAuth endpoints. `tls_insecure_skip_verify` remains an
  explicit operator escape hatch and keeps the existing warning.
- Token cache path expands `~` and is created with restrictive permissions.
- Logs redact token-like fields.
- Header names used for auth are configured in `auth`, not in arbitrary
  `headers`, to avoid accidental token leakage or duplicate auth headers.

## Task Checklist

- [x] Document design and rollout.
- [x] Add provider config fields and inheritance tests.
- [x] Add OAuth token manager with secure cache tests.
- [x] Add request decoration for custom path, auth header, static/generated
      headers, and request body extras.
- [x] Add model-list discovery and first-model default selection.
- [x] Add fake auth/chat/model-list integration tests.
- [x] Refresh `providers.yaml.example`, user guide Markdown, and HTML.
