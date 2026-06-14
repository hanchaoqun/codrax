# fmt tm year overflow symptom fixture

This fixture is reduced from fmtlib/fmt PR #2564. Ordinary calendar years render correctly, but extreme `tm_year` values must not overflow before formatting.
