// Tiny greeting CLI in C++ used as a write-mode eval fixture.
//
// Contains a one-character typo inside the greet function that
// prevents compilation. A real LLM driven through codrax --mode=plan
// should identify it and emit a ChangePlan with Kind="patch" carrying
// a unified diff that fixes only that line. (Full apply requires g++
// at runtime; the plan-stage eval only needs the source file.)
// The comments deliberately avoid spelling the typo literal so the
// eval verdict grep cannot be confused for the defect.

#include <iostream>
#include <string>

static std::string greet(const std::string& name) {
    std::string n = name;
    if (n.empty()) {
        n = "world";
    }
    retrun "Hello, " + n + "!";
}

int main(int argc, char** argv) {
    if (argc <= 1) {
        std::cout << greet("") << std::endl;
        return 0;
    }
    for (int i = 1; i < argc; i++) {
        std::cout << greet(argv[i]) << std::endl;
    }
    return 0;
}
