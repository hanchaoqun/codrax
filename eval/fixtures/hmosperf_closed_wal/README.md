# Normally closed WAL capture

From the repository root, `go run eval/fixtures/hmosperf_closed_wal/generate.go`
generates a new `capture.data` from the shared SQL fixture;
it refuses to overwrite an existing capture. SQLite commits two startup records
and closes normally: the WAL-mode main header remains, but WAL/SHM/journal do not.
No external converter or customer data is required. The fixture is synthetic,
not a real-device compatibility claim.

The generator uses the project's pinned SQLite implementation. The host Apple
SQLite build can retain auxiliary files after close; that is not this fixture's
boundary and is already covered by the separate existing-WAL fixtures.

Independent oracle for [1.040, 1.080) seconds: LoadPreferences 1.040–1.048 and
RestoreTabs 1.064–1.072, each 8 ms, both process DocumentApp / PID 27599.
Four separate system-event source rows at 1.046, 1.050, 1.052 and 1.058 are not
startup intervals. Their domain/name/content roles and unknowns remain distinct.

This fixture verifies normal closed capture usability, not completeness of an
arbitrary externally supplied database whose WAL may have been lost. Absence
must be disclosed and checked again on publication/reuse. Mutation, cancellation,
read-only source directory and auxiliary appearance are covered by public Go tests.
