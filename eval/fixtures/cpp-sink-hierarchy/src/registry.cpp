#include <memory>
#include <string>

#include "logx/console_sink.hpp"
#include "logx/file_sink.hpp"
#include "logx/rotating_sink.hpp"

namespace logx {

// Factory: maps a config-provided backend name to a concrete sink.
// Unknown names return nullptr so callers fail loud at setup time
// instead of silently logging nowhere.
class SinkRegistry {
 public:
  static std::unique_ptr<Sink> create(const std::string& kind,
                                      const std::string& path) {
    if (kind == "console") {
      return std::make_unique<ConsoleSink>();
    }
    if (kind == "file") {
      return std::make_unique<FileSink>(path);
    }
    if (kind == "rotating") {
      return std::make_unique<RotatingSink>(path, 1 << 20);
    }
    return nullptr;
  }
};

std::unique_ptr<Sink> make_sink(const std::string& kind,
                                const std::string& path) {
  return SinkRegistry::create(kind, path);
}

}  // namespace logx
