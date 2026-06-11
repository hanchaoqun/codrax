# cpp-sink-hierarchy — C++ virtual-dispatch hierarchy fixture

Blueprint shapes: spdlog sink hierarchy; fmt formatter specialization.

Design facts (cases assert exactly these):
- Abstract base `Sink` (include/logx/sink.hpp) has TWO direct concrete
  subclasses: ConsoleSink and FileSink. RotatingSink derives from
  FileSink (second-level inheritance), so it is a Sink transitively.
- Virtual dispatch chain: Logger::log (src/logger.cpp) -> formats via
  format_value -> sink_->write(...) virtual call -> concrete sink.
- SinkRegistry::create (src/registry.cpp) maps "console"->ConsoleSink,
  "file"->FileSink, "rotating"->RotatingSink. Unknown names return nullptr.
- Formatter<double> (include/logx/formatter.hpp) is an explicit template
  specialization: fixed 3-digit precision, unlike the generic Formatter<T>.
