# Reduced reproduction: fmtlib/fmt#2564 tm formatter year overflow

Upstream record: https://github.com/fmtlib/fmt/pull/2564 — the chrono tm
formatter did year arithmetic in int; tm_year near INT_MAX overflowed
before widening. Upstream switched the year carriers to long long and
pinned `tm_year = INT_MAX → "2147485547"` in chrono-test.cc.

This fixture reduces the bug to one inline function. The Makefile compiles
with -fwrapv so the pre-fix wrap is deterministic across compilers; the
correct fix performs the addition in a wider type.
