# HiSys original-row identity fixture

Only `capture.data` is attached. The SQL and this oracle are not model context.
Rebuild into an absent database with `sqlite3 capture.data < capture.sql`.

In [2.000, 2.080) seconds there are six events. Four separate rows have identical
business fields and timestamp 2.010, with hidden rowids -3, 0, 9007199254740993,
9223372036854775807. The declared `id` is 7 for all four and is not their unique
row identity. The two completion events are at 2.040 (rowid 23, session 17) and
2.060 (rowid 11, session 18). The rows at 1.900 and 2.100 are outside the window.

Manual acceptance: preserve all six records and exact integer identifiers;
equal-time rows may use signed rowid for stable display, but that order does not
prove execution order or causality. An original rowid identifies only a row in
this source generation's `hisys_all_event`, not a global event or thread identity.
The binary source must remain unchanged. Capture completeness is not established.
