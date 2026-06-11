#pragma once
#include <string>
#include "logx/file_sink.hpp"

namespace logx {

// Second-level subclass: rotates the underlying file when max_bytes
// is exceeded. Inherits the FileSink buffer/flush machinery and only
// overrides write() to add the size check.
class RotatingSink : public FileSink {
 public:
  RotatingSink(std::string path, std::size_t max_bytes)
      : FileSink(std::move(path)), max_bytes_(max_bytes) {}

  void write(const std::string& line) override {
    written_ += line.size() + 1;
    if (written_ > max_bytes_) {
      rotate();
      written_ = line.size() + 1;
    }
    FileSink::write(line);
  }

  const char* name() const override { return "rotating"; }

 private:
  void rotate() {
    if (handle_ != nullptr) {
      std::fclose(handle_);
    }
    const std::string old = path_ + ".1";
    std::rename(path_.c_str(), old.c_str());
    handle_ = std::fopen(path_.c_str(), "a");
  }

  std::size_t max_bytes_;
  std::size_t written_ = 0;
};

}  // namespace logx
