#include <memory>
#include <string>
#include <utility>

#include "logx/formatter.hpp"
#include "logx/sink.hpp"

namespace logx {

enum class Level { kDebug, kInfo, kWarn, kError };

const char* level_label(Level level) {
  switch (level) {
    case Level::kDebug: return "DEBUG";
    case Level::kInfo: return "INFO";
    case Level::kWarn: return "WARN";
    case Level::kError: return "ERROR";
  }
  return "?";
}

// Front-end: owns one sink and dispatches every record through the
// virtual Sink::write. The concrete backend is chosen at setup time
// by SinkRegistry::create; Logger never names a concrete sink type.
class Logger {
 public:
  explicit Logger(std::unique_ptr<Sink> sink) : sink_(std::move(sink)) {}

  void log(Level level, const std::string& message) {
    if (sink_ == nullptr || level < min_level_) return;
    std::string line;
    line.reserve(message.size() + 16);
    line.append(level_label(level));
    line.append(" ");
    line.append(message);
    sink_->write(line);
    if (level >= Level::kError) {
      sink_->flush();
    }
  }

  void log_latency(const std::string& op, double millis) {
    // double goes through the Formatter<double> specialization:
    // fixed 3-digit precision keeps these lines grep-stable.
    log(Level::kInfo, op + " took " + format_value(millis) + "ms");
  }

  void set_min_level(Level level) { min_level_ = level; }

 private:
  std::unique_ptr<Sink> sink_;
  Level min_level_ = Level::kDebug;
};

}  // namespace logx
