#pragma once
#include <cstdio>
#include "logx/sink.hpp"

namespace logx {

// Writes to stderr; unbuffered, so flush() stays the base no-op.
class ConsoleSink : public Sink {
 public:
  void write(const std::string& line) override {
    std::fputs(line.c_str(), stderr);
    std::fputc('\n', stderr);
  }
  const char* name() const override { return "console"; }
};

}  // namespace logx
