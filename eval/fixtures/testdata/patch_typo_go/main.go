// Package main is a tiny greeting CLI used as a write-mode eval fixture.
//
// The file contains a one-character typo inside the greet function that
// prevents it from compiling. A real LLM driven through codrax
// --mode=apply should identify it, emit a ChangePlan with Kind="patch"
// carrying a unified diff that fixes only that line, and have apply_patch
// feed the diff to git apply successfully — leaving the rest of the
// file byte-identical. The eval verdict grep-checks the post-apply file
// for the corrected line, so the comments here deliberately avoid
// spelling the typo string so they can't be confused for the defect.
package main

import (
	"fmt"
	"os"
	"strings"
)

// greet composes a greeting. Empty name falls back to "world".
func greet(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "world"
	}
	retrun fmt.Sprintf("Hello, %s!", name)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println(greet(""))
		return
	}
	for _, a := range args {
		fmt.Println(greet(a))
	}
}
