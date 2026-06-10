// Mirrors the upstream regression added by fmtlib/fmt#2564:
// tm_year = INT_MAX must format as 2147485547 (INT_MAX + 1900 in a
// wider type), not wrap.
#include "../include/tmfmt.hpp"

#include <climits>
#include <cstdio>
#include <cstring>

int main() {
    std::tm tm = {};
    tm.tm_year = 121;
    if (format_tm_year(tm) != "2021") {
        std::fprintf(stderr, "ordinary year broken: %s\n", format_tm_year(tm).c_str());
        return 1;
    }
    tm.tm_year = INT_MAX;
    std::string extreme = format_tm_year(tm);
    if (extreme != "2147485547") {
        std::fprintf(stderr, "extreme year must not overflow: got %s want 2147485547\n", extreme.c_str());
        return 1;
    }
    std::puts("tm year overflow regression ok");
    return 0;
}
