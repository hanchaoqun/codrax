# Apache Commons Lang RandomStringUtils ASCII optimization fixture

Source record:
- Repository: https://github.com/apache/commons-lang
- Fix PR: https://github.com/apache/commons-lang/pull/1273

The upstream fix restricted ASCII-only random-string optimizations so
non-ASCII letter and digit ranges are not clamped to ASCII.

This fixture keeps a minimized Java source and test file. Because this host
does not have a Java runtime, `make check` runs a Python validator that checks
the source-level invariants and regression tests expected from the PR.
