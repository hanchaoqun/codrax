# nlohmann/json long double serializer fixture

Source record:
- Repository: https://github.com/nlohmann/json
- Issue context: https://github.com/nlohmann/json/pull/3929
- Fix PR: https://github.com/nlohmann/json/pull/3929

The upstream fix changed the long-double `snprintf` format from `%.*lg` to
`%.*Lg` in both the implementation header and generated single include.

This fixture keeps only the two synchronized headers and a compile-time test.
`make check` builds with `-Wformat -Werror`, so either stale header fails.
