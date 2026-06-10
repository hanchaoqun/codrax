# Reduced reproduction: iamkun/dayjs#1611 duration parse NaN components

Upstream record: https://github.com/iamkun/dayjs/pull/1611 — parsing an ISO
8601 duration with missing components (e.g. "PT1H") converted undefined
captures via Number() into NaN; the fix defaults missing components to 0.
Upstream pinned `dayjs.duration('PT1H').format(...)` to
`0000-00-00T01:00:00`.

This fixture reduces the plugin to a standalone module + node test. The
eval host has no node/npm, so `make check` runs a python oracle over the
same contract; the node test stays authoritative upstream-shaped content.
