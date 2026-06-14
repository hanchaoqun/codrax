#include "../include/tmfmt.hpp"

#include <climits>
#include <cstdio>

static int expect_year(const char* name, int tm_year, const char* want) {
    std::tm value = {};
    value.tm_year = tm_year;
    std::string got = mini_fmt::format_tm_year(value);
    if (got != want) {
        std::fprintf(stderr, "%s: got %s, want %s\n", name, got.c_str(), want);
        return 1;
    }
    return 0;
}

int main() {
    int failed = 0;
    failed |= expect_year("ordinary year", 121, "2021");
    failed |= expect_year("large positive year", INT_MAX, "2147485547");
    if (failed == 0) {
        std::puts("tm year formatting checks passed");
    }
    return failed;
}
