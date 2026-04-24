# Minimal Makefile for the C write-mode eval fixture. Pre-apply the
# build fails (typo in main.c); post-apply the binary links and runs
# a smoke test. codrax's run_tests auto-detects `make test` when a
# Makefile is present and classifies exit-code pass/fail.

CC ?= gcc
CFLAGS ?= -Wall -O0

main: main.c
	$(CC) $(CFLAGS) -o main main.c

test: main
	./main > /dev/null && ./main codrax > /dev/null && echo PASS

clean:
	rm -f main

.PHONY: test clean
