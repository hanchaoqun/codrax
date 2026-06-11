#pragma once
#include <cstdio>
#include <string>
#include "logx/sink.hpp"

namespace logx {

// Appends to a single file with libc buffering; flush() forces the
// buffer out so crashes lose at most one line.
class FileSink : public Sink {
 public:
  explicit FileSink(std::string path) : path_(std::move(path)) {
    handle_ = std::fopen(path_.c_str(), "a");
  }
  ~FileSink() override {
    if (handle_ != nullptr) std::fclose(handle_);
  }

  void write(const std::string& line) override {
    if (handle_ == nullptr) return;
    std::fputs(line.c_str(), handle_);
    std::fputc('\n', handle_);
  }

  void flush() override {
    if (handle_ != nullptr) std::fflush(handle_);
  }

  const char* name() const override { return "file"; }

 protected:
  std::string path_;
  std::FILE* handle_ = nullptr;
};

}  // namespace logx
