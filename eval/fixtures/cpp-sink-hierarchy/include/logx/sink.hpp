#pragma once
#include <string>

namespace logx {

// Abstract sink: every output backend implements write(); flush() has
// a default no-op so buffered sinks override it selectively.
class Sink {
 public:
  virtual ~Sink() = default;
  virtual void write(const std::string& line) = 0;
  virtual void flush() {}
  virtual const char* name() const = 0;
};

}  // namespace logx
