#include <stdio.h>
#include <string.h>

#include "cmdsrv.h"

/* Line-oriented REPL: `<command> [arg]` per line; `quit` exits. */
int main(void) {
    char line[256];
    while (fgets(line, sizeof line, stdin) != NULL) {
        line[strcspn(line, "\n")] = '\0';
        char *cmd = strtok(line, " ");
        if (cmd == NULL) continue;
        char *arg = strtok(NULL, " ");
        char *argv[1] = {arg};
        int argc = arg != NULL ? 1 : 0;
        int rc = dispatch_command(cmd, argc, argv);
        if (rc == 1) break;
        if (rc == -1) fprintf(stderr, "unknown command: %s\n", cmd);
        if (rc == -2) fprintf(stderr, "wrong arity for: %s\n", cmd);
    }
    return 0;
}
