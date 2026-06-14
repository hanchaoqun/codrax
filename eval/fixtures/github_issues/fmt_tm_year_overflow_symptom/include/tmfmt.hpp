#pragma once

#include <ctime>
#include <string>

namespace mini_fmt {

struct tm_parts {
    int year_offset;
    int month;
    int day;
};

inline tm_parts extract_parts(const std::tm& value) {
    return tm_parts{value.tm_year, value.tm_mon + 1, value.tm_mday};
}

inline std::string render_year(int calendar_year) {
    return std::to_string(calendar_year);
}

inline std::string format_tm_year(const std::tm& value) {
    tm_parts parts = extract_parts(value);
    int calendar_year = parts.year_offset + 1900;
    return render_year(calendar_year);
}

} // namespace mini_fmt
