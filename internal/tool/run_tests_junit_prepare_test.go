package tool

import (
	"fmt"
	"strings"
	"testing"
)

func TestB1651JUnitReportArgumentUsesSelectedShellDialect(t *testing.T) {
	for _, path := range []string{`C:\customer repo\.codrax\tmp\junit-invocation-42\ctest.xml`, `/customer's repo/.codrax/tmp/junit-invocation-42/ctest.xml`} {
		for _, shell := range []string{"cmd", "CMD.EXE"} {
			if got := junitReportArgumentForShell(path, shell); got != fmt.Sprintf("%q", path) {
				t.Fatalf("cmd acquired POSIX quoting: shell=%q path=%q got=%q", shell, path, got)
			}
		}
		for _, shell := range []string{"sh", "/bin/sh", "/bin/bash"} {
			if got := junitReportArgumentForShell(path, shell); got != shellQuoteWord(path) {
				t.Fatalf("POSIX shell lost literal path quoting: shell=%q path=%q got=%q", shell, path, got)
			}
		}
	}
	if got := shellQuoteWord("-Dsurefire.reportNameSuffix=codrax-0123abcd"); strings.ContainsAny(got, "'\"") {
		t.Fatalf("producer nonce unnecessarily acquired a shell dialect: %q", got)
	}
}
