# Zod falsy prefault JSON schema fixture

Source record:
- Repository: https://github.com/colinhacks/zod
- Issue: https://github.com/colinhacks/zod/issues/5824
- Fix PR: https://github.com/colinhacks/zod/pull/5893

The upstream fix changed `toJSONSchema` from a truthiness check on
`_prefault` to an existence check, preserving falsy prefault values such as
`false`, `0`, and `""`.

This fixture keeps a minimized TypeScript implementation and test file.
`make check` uses a dependency-free Python validator because Node/npm are not
available on this host.
