#include <cstring>
#include <iostream>

#include "include/nlohmann/detail/output/serializer.hpp"
#include "single_include/nlohmann/json.hpp"

static int check_header()
{
    char buf[64] = {};
    nlohmann::detail::output::snprintf_float(buf, sizeof(buf), 3, 1.25L);
    return std::strlen(buf) == 0;
}

static int check_single_include()
{
    char buf[64] = {};
    nlohmann_single_include::detail::output::snprintf_float(buf, sizeof(buf), 3, 1.25L);
    return std::strlen(buf) == 0;
}

int main()
{
    if (check_header() || check_single_include()) {
        std::cerr << "long double serialization returned empty output\n";
        return 1;
    }
    return 0;
}
