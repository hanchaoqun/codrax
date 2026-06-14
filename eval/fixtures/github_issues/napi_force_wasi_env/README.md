# napi-rs force-wasi environment fixture

This fixture reconstructs the generated native loader symptom behind
napi-rs PR #3236.

Observed behavior:

- downstream users set `NAPI_RS_FORCE_WASI=false` to disable the WASI fallback;
- the generated loader still treats the non-empty string as truthy and attempts
  to load the `.wasi.cjs` bundle;
- `NAPI_RS_FORCE_WASI=true` and `NAPI_RS_FORCE_WASI=error` must continue to force
  the WASI path.

The regression contract is encoded in `tests/check_force_wasi.py`.
