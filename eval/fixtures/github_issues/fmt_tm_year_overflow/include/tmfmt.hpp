// Reduced from fmtlib/fmt include/fmt/chrono.h (pre-#2564).
//
// The tm formatter computed the calendar year from tm_year with plain
// int arithmetic. For extreme inputs (tm_year near INT_MAX) the
// addition overflows before the value is widened, so the rendered year
// is garbage. Upstream fixed it by carrying the year in long long.
#pragma once

#include <ctime>
#include <string>

inline std::string format_tm_year(const std::tm& tm) {
    int year = tm.tm_year + 1900;
    return std::to_string(year);
}
