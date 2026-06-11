#ifndef CMDSRV_H
#define CMDSRV_H

#include <stddef.h>
#include <stdint.h>

typedef struct {
    const char *name;
    int (*handler)(int argc, char **argv);
    int arity;
} command_entry;

/* dispatch.c */
int dispatch_command(const char *name, int argc, char **argv);
size_t command_count(void);

/* clock.c — monotonic nanoseconds, platform-forked implementation. */
uint64_t monotonic_now_ns(void);

#endif
