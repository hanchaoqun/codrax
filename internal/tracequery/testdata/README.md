# Trace Query External Fixtures

Small public trace fixtures used to keep the runtime trace parser honest against
real ftrace/bytrace shapes. These are intentionally tiny excerpts, not full
customer traces.

- `android_perfetto_sched_blocked.systrace`
  - Source: https://github.com/google/perfetto/blob/main/test/trace_processor/diff_tests/parser/parsing/sched_blocked_systrace.systrace
  - License: Apache-2.0, Google Perfetto project.
  - Covers Android/Perfetto-style systrace rows with `(-----)` TGID and
    `sched_blocked_reason`.
- `android_perfetto_cpu_frequency_limits.systrace`
  - Source: https://github.com/google/perfetto/blob/main/test/trace_processor/diff_tests/parser/sched/cpu_frequency_limits.systrace
  - License: Apache-2.0, Google Perfetto project.
  - Covers CPU frequency limit event names and repeated same-CPU state rows.
- `harmony_openharmony_bytrace_thread.txt`
  - Source: https://gitee.com/openharmony-sig/smartperf/raw/master/host/trace_streamer/test/resource/ut_bytrace_input_thread.txt
  - License: Apache-2.0, OpenHarmony-SIG SmartPerf project.
  - Covers Harmony/OpenHarmony bytrace/hitrace style scheduling rows with
    binder-like thread names, `sched_waking`, `sched_wakeup`, and
    `sched_switch` events.
