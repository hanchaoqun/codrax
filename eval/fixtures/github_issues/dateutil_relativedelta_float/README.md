# Reduced reproduction: dateutil relativedelta whole-float months/years

Upstream record: dateutil changelog 2.7.0 — "Fixed issue where passing
whole number floats to the months or years arguments of the relativedelta
constructor would lead to errors during addition." (gh pr #411, fixed by
gh pr #553).

This fixture reduces relativedelta to the month/year arithmetic core. The
test suite is standard-library unittest (no pytest config on purpose);
there is deliberately no Makefile or manifest — the repo is a bare Python
directory.
