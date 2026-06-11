#include "cmdsrv.h"

/* Monotonic clock, one implementation per platform family. Exactly
 * three branches; the POSIX fallback is last so new platforms get a
 * standards-based default instead of a silent zero. */

#if defined(_WIN32)

#include <windows.h>

uint64_t monotonic_now_ns(void) {
    static LARGE_INTEGER freq;
    LARGE_INTEGER counter;
    if (freq.QuadPart == 0) {
        QueryPerformanceFrequency(&freq);
    }
    QueryPerformanceCounter(&counter);
    return (uint64_t)(counter.QuadPart * 1000000000ull / freq.QuadPart);
}

#elif defined(__APPLE__)

#include <mach/mach_time.h>

uint64_t monotonic_now_ns(void) {
    static mach_timebase_info_data_t info;
    if (info.denom == 0) {
        mach_timebase_info(&info);
    }
    return mach_absolute_time() * info.numer / info.denom;
}

#else /* POSIX fallback */

#include <time.h>

uint64_t monotonic_now_ns(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t)ts.tv_sec * 1000000000ull + (uint64_t)ts.tv_nsec;
}

#endif
