#!/usr/bin/env python3
"""Drive codrax REPL via pty for e2e observation.

Fixes vs the prior version:
  * Prompt-detection now arms ONCE on first ❯❯ then disarms — no
    re-firing on bubbletea's per-keystroke prompt repaint loop.
  * Completion detection waits for either a persistent idle window
    (no bytes for IDLE_SECS) OR a known terminal marker
    (已撰写最终答案 / Final answer composed / error:).
  * Two-phase /exit: send /exit, then re-arm a short grace-window
    before SIGKILL so the REPL can flush any final lines.
  * Captures raw bytes to LOG and a denoised transcript to LOG.clean
    (ANSI stripped + dedup of repeat prompt lines).

Usage:
    python3 run_repl.py [BINARY] [QUESTION] [TIMEOUT_S] [LOG]
"""
import os, pty, re, select, sys, time

BINARY   = sys.argv[1] if len(sys.argv) > 1 else "./codrax"
QUESTION = sys.argv[2] if len(sys.argv) > 2 else "请用 mermaid flowchart 画一下 codrax 的 4 阶段 read-mode pipeline"
TIMEOUT  = int(sys.argv[3]) if len(sys.argv) > 3 else 240
LOG      = sys.argv[4] if len(sys.argv) > 4 else "/tmp/repl_capture.log"
IDLE_SECS = 25  # No bytes from REPL for this long → assume dispatch finished

argv = [BINARY, "--log-level", "debug"]
print(f"[wrap] argv={argv}  timeout={TIMEOUT}s  log={LOG}")

pid, fd = pty.fork()
if pid == 0:
    os.execvp(argv[0], argv)

buf = b""
phase = "wait-prompt"   # → "dispatching" → "exit-sent" → "done"
deadline = time.time() + TIMEOUT
last_data = time.time()
exit_at = None
completion_markers = ("已撰写最终答案", "Final answer composed", "已生成最终答案",
                      "error:", "(no result)")

with open(LOG, "wb") as fout:
    try:
        while time.time() < deadline:
            rlist, _, _ = select.select([fd], [], [], 0.5)
            if rlist:
                try:
                    data = os.read(fd, 8192)
                except OSError:
                    break
                if not data:
                    break
                buf += data
                fout.write(data); fout.flush()
                last_data = time.time()

            tail = buf[-4096:].decode("utf-8", errors="replace")

            if phase == "wait-prompt":
                if "❯❯" in tail:
                    time.sleep(1.0)
                    print(f"[wrap] sending question")
                    os.write(fd, QUESTION.encode("utf-8"))
                    time.sleep(0.2)
                    os.write(fd, b"\r")
                    phase = "dispatching"
                    last_data = time.time()
                continue

            if phase == "dispatching":
                idle = time.time() - last_data
                hit_marker = any(m in tail for m in completion_markers)
                if hit_marker and idle > 4:
                    print(f"[wrap] completion marker hit, idle={idle:.1f}s, sending /exit")
                    os.write(fd, b"/exit\r")
                    phase = "exit-sent"; exit_at = time.time()
                    continue
                if idle > IDLE_SECS:
                    print(f"[wrap] idle {idle:.1f}s > {IDLE_SECS}s, sending /exit")
                    os.write(fd, b"/exit\r")
                    phase = "exit-sent"; exit_at = time.time()
                    continue

            if phase == "exit-sent" and exit_at and time.time() - exit_at > 6:
                print("[wrap] /exit grace expired, terminating")
                phase = "done"
                break
    except Exception as e:
        sys.stderr.write(f"[wrap] error: {e}\n")

try: os.kill(pid, 9)
except OSError: pass
try: os.waitpid(pid, 0)
except ChildProcessError: pass

# -- post-process the captured stream into a denoised transcript ----
ANSI = re.compile(r"\x1b\[[0-9;?]*[A-Za-z]")
RAW  = open(LOG, "rb").read().decode("utf-8", errors="replace")
clean = ANSI.sub("", RAW)
# Collapse the bubbletea repaint loop: when consecutive lines are
# byte-identical and one of them is a pure repaint of the prompt,
# keep only the LAST occurrence.
lines = [ln.rstrip() for ln in clean.replace("\r", "\n").split("\n")]
denoised = []
prev = None
for ln in lines:
    if ln == prev: continue
    denoised.append(ln); prev = ln
clean_path = LOG + ".clean"
open(clean_path, "w").write("\n".join(denoised) + "\n")
print(f"[wrap] raw={os.path.getsize(LOG)}B  clean={os.path.getsize(clean_path)}B")
