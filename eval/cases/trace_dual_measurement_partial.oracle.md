# Independent oracle: one unavailable capture

This oracle is not a model input. The adjacent question contains only the sources, independent windows and requested result.

- Baseline `empty.htrace` is a zero-byte existing file, not a capture with a measured zero or an executed empty measurement population. Its [1,2) second request must report preparation/input failure, without invented rows, successful acquisition or zeros.
- Current `current.data` is the unchanged fixture described in `trace_dual_measurement_records.oracle.md`. Its [4,4.5) window has 8 retained rows in 4 series. Reuse that exact row oracle, including carry-in, two independent same-name filters, NULL, TEXT "0", numeric zero, exact integer 9007199254740995, unknown/invalid duration and the excluded right edge.
- The current side must be really prepared and queried despite baseline failure. A candidate probe, a successful tool-call attempt, an adjacent SQL source file, a status-only result or a model-created table is not a native acquisition receipt.
- Audit original-path/window binding, analyzer typed comparison, actual calls/results, per-side status, finalizer context, accepted answer and rendered native tables. A machine successful-result-header match remains only a minimum check; manually inspect the actual native result.
- No device/workload/unit/clock/state semantics are known. No delta or causal conclusion is requested. Missing baseline must not suppress current facts or be interpreted as zero for a difference.
- Input bytes must remain unchanged. Preparation paths are not new user-authorized inputs. One side failing does not authorize directory discovery or other sources; global cancellation remains global.

This supplements rather than replaces the historical both-success case and its retained failures. Reverse order, both failures, no independent binding, same dependent bundle, stale generations, source-window swap and cancellation are deterministic public-path tests, not extra live cases in this batch.
