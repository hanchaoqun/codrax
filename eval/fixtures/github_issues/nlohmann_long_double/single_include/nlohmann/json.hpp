#pragma once

#include <cstdio>
#include <cstddef>

namespace nlohmann_single_include::detail::output {

inline int snprintf_float(char* buf, std::size_t size, int d, long double x)
{
    return (std::snprintf)(buf, size, "%.*lg", d, x);
}

}  // namespace nlohmann_single_include::detail::output
