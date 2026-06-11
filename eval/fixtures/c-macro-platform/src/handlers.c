#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "cmdsrv.h"

int cmd_ping(int argc, char **argv) {
    (void)argc;
    (void)argv;
    puts("pong");
    return 0;
}

int cmd_echo(int argc, char **argv) {
    if (argc < 1) return -2;
    puts(argv[0]);
    return 0;
}

int cmd_stat(int argc, char **argv) {
    (void)argc;
    (void)argv;
    printf("commands=%zu\n", command_count());
    return 0;
}

/* The only handler that touches the platform clock: measures the
 * busy-wait it performs so callers can verify timer resolution. */
int cmd_sleep(int argc, char **argv) {
    if (argc < 1) return -2;
    long ms = strtol(argv[0], NULL, 10);
    uint64_t start = monotonic_now_ns();
    uint64_t target = start + (uint64_t)ms * 1000000ull;
    while (monotonic_now_ns() < target) {
        /* spin */
    }
    printf("slept_ns=%llu\n",
           (unsigned long long)(monotonic_now_ns() - start));
    return 0;
}

int cmd_quit(int argc, char **argv) {
    (void)argc;
    (void)argv;
    return 1; /* signals the loop to exit */
}
