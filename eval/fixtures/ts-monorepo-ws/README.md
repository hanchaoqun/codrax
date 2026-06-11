# ts-monorepo-ws — TypeScript workspace monorepo fixture

Blueprint shapes: turborepo / pnpm-workspace package layout; zod-style
barrel re-exports; axios-style transport/retry split.

Design facts (cases assert exactly these):
- RetryPolicy (packages/core/src/retry.ts) has EXACTLY TWO implementers:
  ExponentialBackoff and FixedDelay. JitterHelper is a plain helper class,
  NOT an implementer.
- Cross-package call chain: cli run() -> @app/client ApiClient.fetchUser()
  -> @app/core HttpTransport.send() -> RetryPolicy.nextDelay().
- The @app/core / @app/client path aliases are defined ONLY in
  tsconfig.base.json ("paths"); package tsconfigs extend it.
- packages/core/src/index.ts is a barrel: it re-exports retry.ts and
  transport.ts; ApiClient imports through the package name, not deep paths.
