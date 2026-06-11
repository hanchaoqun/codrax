#include <string.h>

#include "cmdsrv.h"

/* First expansion: forward-declare every handler named in cmds.def. */
#define CMD(name, handler, arity) int handler(int argc, char **argv);
#include "cmds.def"
#undef CMD

/* Second expansion: build the command table from the same list. The
 * two expansions cannot drift apart because both read cmds.def. */
#define CMD(name, handler, arity) {#name, handler, arity},
static const command_entry command_table[] = {
#include "cmds.def"
};
#undef CMD

size_t command_count(void) {
    return sizeof(command_table) / sizeof(command_table[0]);
}

int dispatch_command(const char *name, int argc, char **argv) {
    for (size_t i = 0; i < command_count(); i++) {
        const command_entry *entry = &command_table[i];
        if (strcmp(entry->name, name) != 0) {
            continue;
        }
        if (argc != entry->arity) {
            return -2; /* arity mismatch */
        }
        return entry->handler(argc, argv);
    }
    return -1; /* unknown command */
}
