# Native resource stack fixture

Constructed SQLite input, not a device trace. The SQL uses the reference repository's `native_hook` / `native_hook_frame` / `data_dict` shapes and exercises Codrax's default attached-binary preparation path.

Build a new file using `sqlite3 capture.data < capture.sql`. The source has five resource events and eight frame rows, without any CPU Running witness. The query `[10.000, 10.050)` seconds for thread 101 contains exactly three events: allocation at 10.005, free at 10.025, allocation at 10.040. The 10.015 event belongs to thread 202; the 10.050 event is at the excluded right boundary.

Independent audit oracle:

- Callchain 7: three recorded frames, depths 0/1/2, symbols `malloc`, `ImageCache::reserve`, `UIFrame::render`; libraries and source fields survive. IP -1 retains its unsigned all-one bit pattern without becoming a missing-value sentinel.
- Callchain 8: two recorded frames at depths 0/2, missing depth 1, unresolved symbol at depth 2. The unknown fields are not zeros. No source claim proves the capture collected the complete stack.
- Callchain 9: three recorded frame rows, depth 0 duplicated; depth 1 references duplicate dictionary ID 300 and must not choose `ambiguous_A` or `ambiguous_B`. A unique complete call order is not established.
- Stack keys are local to the capture. Resource owner is established by its native event, not by the frame table. No fixed depth is declared the business leaf. Resource lifetime is not function duration; resource facts do not establish CPU execution, leaks, waits or a response root cause.

The live question intentionally does not expose this oracle or implementation constraints to the model.
