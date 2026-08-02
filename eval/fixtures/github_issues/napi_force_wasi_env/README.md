# napi-rs force-wasi environment fixture

This fixture reconstructs the generated native loader symptom behind
napi-rs PR #3236.

Observed behavior:

- downstream users set `NAPI_RS_FORCE_WASI=false` to stop forcing the WASI path
  while still retaining the loader's normal fallback when the native binding is
  actually unavailable;
- the generated loader incorrectly treats that non-empty string as a force flag
  and attempts to load the `.wasi.cjs` bundle even when the native binding loaded;
- `NAPI_RS_FORCE_WASI=true` and `NAPI_RS_FORCE_WASI=error` must continue to force
  the WASI path.

The regression contract is encoded in `tests/check_force_wasi.py`, including a
generated-artifact scope check so a helper declared only outside the returned
loader template cannot produce an undefined identifier at runtime.
