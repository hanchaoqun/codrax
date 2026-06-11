#pragma once
#include <sstream>
#include <string>

namespace logx {

// Generic formatter: stream the value as-is.
template <typename T>
struct Formatter {
  static std::string format(const T& value) {
    std::ostringstream out;
    out << value;
    return out.str();
  }
};

// Explicit specialization for double: fixed 3-digit precision so log
// lines stay diff-friendly across platforms (the generic path would
// print locale/width-dependent digits).
template <>
struct Formatter<double> {
  static std::string format(const double& value) {
    std::ostringstream out;
    out.setf(std::ios::fixed);
    out.precision(3);
    out << value;
    return out.str();
  }
};

template <typename T>
std::string format_value(const T& value) {
  return Formatter<T>::format(value);
}

}  // namespace logx
