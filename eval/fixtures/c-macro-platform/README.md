# c-macro-platform — X-macro command table + platform forks fixture

Blueprint shapes: redis command table; libgit2 #ifdef platform forks.

Design facts (cases assert exactly these):
- include/cmds.def is the X-macro list with EXACTLY FIVE commands:
  ping, echo, stat, sleep, quit (each CMD(name, handler, arity)).
- src/dispatch.c expands the list twice: once for the handler
  declarations and once for the command_table[] entries; lookup is a
  linear name scan over that table.
- src/clock.c forks on platform: _WIN32 uses QueryPerformanceCounter,
  __APPLE__ uses mach_absolute_time, the POSIX fallback uses
  clock_gettime(CLOCK_MONOTONIC). Exactly three branches.
- cmd_sleep (src/handlers.c) is the only handler that calls
  monotonic_now_ns (twice: before/after the busy wait).
