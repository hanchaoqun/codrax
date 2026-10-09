# Independent oracle: PreferredFrameRate observations

Not supplied to the model. The case question carries no implementation guardrails.

- Closed synthetic SQLite, generated from the fixture SQL. The requested window is [1,2) seconds; default preparation must not change the input.
- `preferred_frame_rate` provides source-owned summary/distribution/timeline. `H:PreferredFrameRate` has a Hz observation protocol; the general process measurement view still does not infer arbitrary units.
- Three independent source/process/filter series: render_service PID100/ipid1/filter10, the same process/filter11, and app.video PID200/ipid2/filter20. Same names or owners do not merge separate filters. No automatic render_service/all-process fallback is allowed.
- Filter10: 120Hz known [1,1.3), 60/120 conflict [1.3,1.4), 60Hz known [1.4,1.5), NULL unknown [1.5,1.6), no interval coverage [1.6,1.7), numeric TEXT `90` unknown [1.7,1.8), finite REAL119.88Hz known [1.8,1.9), no interval coverage [1.9,2). Source intervals beginning before1 carry in; equal 120Hz overlaps count once.
- Filter10 coverage: known500ms/50%, conflict100ms/10%, invalid/missing-value coverage200ms/20%, uncovered200ms/20%. Known distribution uses the full1s denominator: 120Hz300ms/30%,60Hz100ms/10%,119.88Hz100ms/10%. It is not a full-known100% distribution and does not last-write-win conflicts.
- Filter11 independently has75Hz for1000ms/100%; app.video filter20 independently has90Hz for1000ms/100%. Do not add their durations into one1s population or assign either rate to filter10's holes.
- Real zero at1.65 withdur0 has no duration coverage and is not a valid positiveHz measurement;60 at1.95 withNULL duration is only a timestamped observation. The240Hz row exactly at2 is excluded. A NULL timestamp row cannot be assigned to the window; it is separately counted.
- Ten selected raw observations, one unpositioned row, three series. Raw storage/value/interval details remain in the query payload. Tables may bound presentation but must disclose omissions and keep full denominators.
- Preferred rate observations do not prove actual display refresh, frame completion, frame budget, dropped frames, a vote decision or any response root cause. Useful follow-up may request actual display/frame timing; no default60Hz or fabricated dependencies.
- Manually inspect actual query arguments, typed finalizer tables, final answer, and any graph. Machine PASS alone is not acceptance. The answer need not repeat every implementation constraint, but must correctly describe owner/filter-specific values, coverage and limits.
