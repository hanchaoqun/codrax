# Confirmed scheduler concurrency fixture

Authored systrace, not a binary-conversion claim. Query `[1.000000,1.010000)` seconds. All positive thread IDs in this physical source participate; process membership and CPU capacity are not inferred.

Runnable: render remains ready from 1.001 to 1.006 (the 1.003 repeated wake must not reset or double it), decode from 1.002 to 1.004, upload from 1.006 to 1.007. These closed intervals give peak 2 threads, area 8 thread·ms, union 6 ms and full-window mean 0.8 threads. Simultaneous render admission and upload wake at 1.006 must not create a zero-duration peak. The unfinished wake at 1.009 is not closed; do not invent a confirmed wait to the end or call uncovered intervals measured idle.

Running: render's carry-in interval clips to 1.000–1.001, decode runs 1.004–1.005, render runs 1.006–1.008 and upload runs 1.007–1.008. Peak 2, area 5 thread·ms, union 4 ms, full-window mean 0.5. Running and Runnable are separate resource measures, not quantities to add into an actual running parallelism or target response delay. No dependency target, priority competition or frequency evidence establishes a root cause.

Manual review must bind numbers to state/source/window, distinguish exact admitted-interval arithmetic from capture completeness, inspect tool→context delivery, and check any graph uses truthful relations and time spans. Presence regexes are only smoke checks.
