#!/usr/bin/env python3
"""Drive codrax REPL via pty so huh's interactive input sees a TTY."""
import pty, os, select, sys, time

argv = ["./codrax", "--log-level", "debug"]
pid, fd = pty.fork()
if pid == 0:
    os.execvp(argv[0], argv)

buf = b""
sent_request = False
sent_exit = False
deadline = time.time() + 240  # 4 min

try:
    while time.time() < deadline:
        rlist, _, _ = select.select([fd], [], [], 0.5)
        if rlist:
            try:
                data = os.read(fd, 4096)
            except OSError:
                break
            if not data:
                break
            buf += data
            sys.stdout.buffer.write(data)
            sys.stdout.buffer.flush()

        # Heuristic triggers
        text = buf.decode("utf-8", errors="replace")
        if not sent_request and "❯❯" in text:
            time.sleep(0.5)
            os.write(fd, "repomap的作用\n".encode("utf-8"))
            sent_request = True
            buf = b""
        elif sent_request and not sent_exit:
            # Wait for completion marker (error: or final answer rendering done).
            # Detect the banner returning (second ❯❯) or stable 20s idle.
            if "error:" in text or "❯❯" in text:
                time.sleep(1.0)
                os.write(fd, "/exit\n".encode("utf-8"))
                sent_exit = True
except Exception as e:
    sys.stderr.write(f"wrapper: {e}\n")

try:
    os.waitpid(pid, 0)
except ChildProcessError:
    pass
