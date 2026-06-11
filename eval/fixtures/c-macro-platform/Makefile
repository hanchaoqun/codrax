CC ?= cc
CFLAGS ?= -Wall -Wextra -Iinclude
OBJS = src/dispatch.o src/handlers.o src/clock.o src/main.o

cmdsrv: $(OBJS)
	$(CC) -o $@ $(OBJS)

clean:
	rm -f $(OBJS) cmdsrv
