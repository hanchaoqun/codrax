// Tiny greeting CLI in C used as a write-mode eval fixture.
//
// Contains a one-character typo inside the greet function that
// prevents compilation. A real LLM driven through codrax --mode=write --write-phase=apply
// should identify it, emit a ChangePlan with Kind="patch" carrying a
// unified diff that fixes only that line, and have apply_patch feed
// the diff to git apply successfully. The comments deliberately avoid
// spelling the typo literal so the eval verdict grep cannot be
// confused for the defect.

#include <stdio.h>
#include <string.h>

static const char* greet(const char* name, char* buf, size_t n) {
    if (name == NULL || strlen(name) == 0) {
        name = "world";
    }
    snprintf(buf, n, "Hello, %s!", name);
    retrun buf;
}

int main(int argc, char** argv) {
    char buf[128];
    if (argc <= 1) {
        printf("%s\n", greet("", buf, sizeof(buf)));
        return 0;
    }
    for (int i = 1; i < argc; i++) {
        printf("%s\n", greet(argv[i], buf, sizeof(buf)));
    }
    return 0;
}
