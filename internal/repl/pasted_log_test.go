package repl

import (
	"strings"
	"testing"
)

func TestSplitPastedLog_DetectsTimestampedBlock(t *testing.T) {
	input := `分析一下这段日志：
2026-04-21T14:54:37.362 INFO [orchestrator] dispatching agent=analyzer skill=analysis-skill
2026-04-21T14:54:56.671 INFO [orchestrator] dispatching agent=explorer skill=explore-skill
2026-04-21T14:56:00.822 INFO [orchestrator] dispatching agent=explorer skill=explore-skill
`
	cleaned, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("expected detection, got empty")
	}
	if !strings.Contains(detected, "14:54:37") || !strings.Contains(detected, "14:56:00") {
		t.Errorf("detected block missing timestamped lines: %q", detected)
	}
	if strings.Contains(cleaned, "14:54:37") {
		t.Errorf("cleaned request still contains log content: %q", cleaned)
	}
	if !strings.Contains(cleaned, "分析一下这段日志") {
		t.Errorf("cleaned request dropped surrounding prose: %q", cleaned)
	}
}

func TestSplitPastedLog_IgnoresSingleFileLineReference(t *testing.T) {
	// Short prose that mentions a file:line once — must NOT trigger.
	input := "我想在 foo.go:42 加一个 assertion"
	cleaned, detected := splitPastedLog(input)
	if detected != "" {
		t.Errorf("single file:line should not trigger detection, got %q", detected)
	}
	if cleaned != input {
		t.Errorf("cleaned should equal input when no detection: got %q", cleaned)
	}
}

func TestSplitPastedLog_RequiresThreeConsecutiveMatches(t *testing.T) {
	// Only 2 log-like lines — should NOT trigger.
	input := `请问下面的问题：
2026-04-21T14:54:37.362 INFO [orchestrator] A
2026-04-21T14:54:56.671 INFO [orchestrator] B
这段代码会 panic 吗？`
	_, detected := splitPastedLog(input)
	if detected != "" {
		t.Errorf("2-line block should not trigger, got %q", detected)
	}
}

func TestSplitPastedLog_DetectsJavaStack(t *testing.T) {
	input := `为什么会抛这个异常？
java.lang.NullPointerException: Cannot invoke foo on null
	at com.example.Bar.baz(Bar.java:42)
	at com.example.Qux.run(Qux.java:17)
	at java.base/java.lang.Thread.run(Thread.java:831)
`
	_, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("Java stack should trigger detection")
	}
	if !strings.Contains(detected, "NullPointerException") {
		t.Errorf("detected missing exception header: %q", detected)
	}
}

func TestSplitPastedLog_DetectsPythonTraceback(t *testing.T) {
	input := `Traceback (most recent call last):
  File "main.py", line 42, in <module>
    result = process(data)
  File "processor.py", line 17, in process
ValueError: invalid input
什么原因？`
	_, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("Python traceback should trigger detection")
	}
	if !strings.Contains(detected, "Traceback") {
		t.Errorf("detected missing traceback header: %q", detected)
	}
}

func TestSplitPastedLog_DetectsGoPanic(t *testing.T) {
	input := `panic: runtime error: index out of range [5] with length 3
goroutine 1 [running]:
main.process(...)
	/home/foo/main.go:42 +0x1a
main.main()
	/home/foo/main.go:10 +0x5f
`
	_, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("Go panic stack should trigger detection")
	}
}

func TestSplitPastedLog_AllLogNoProse_StillDetects(t *testing.T) {
	input := `2026-04-21T14:54:37.362 INFO [orchestrator] a
2026-04-21T14:54:56.671 INFO [orchestrator] b
2026-04-21T14:56:00.822 INFO [orchestrator] c`
	cleaned, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("all-log body should still trigger")
	}
	if strings.TrimSpace(cleaned) != "" {
		// Caller (dispatch) compensates by injecting a placeholder;
		// splitPastedLog itself should cleanly return empty cleaned.
		t.Errorf("cleaned should be empty when body is all log, got %q", cleaned)
	}
}

func TestSplitPastedLog_BelowByteThreshold_DoesNotTrigger(t *testing.T) {
	// Three short log-like lines but cumulative < 50 bytes.
	input := `A
INFO x
INFO y
INFO z`
	_, detected := splitPastedLog(input)
	if detected != "" {
		t.Errorf("sub-threshold block should not trigger, got %q", detected)
	}
}

func TestSplitPastedLog_NormalPromptPassthrough(t *testing.T) {
	input := "对比 explorer.go 和 extractor.go 的 ReAct 循环终止条件有何不同？"
	cleaned, detected := splitPastedLog(input)
	if detected != "" {
		t.Errorf("normal prompt should not trigger: %q", detected)
	}
	if cleaned != input {
		t.Errorf("normal prompt cleaned should be passthrough, got %q", cleaned)
	}
}

// ── Generalization tests: new runtimes added after the first-pass
// audit. Each asserts the corresponding abstract signal (timestamp /
// stack frame / error header) is detected without a language-
// specific pattern.

func TestSplitPastedLog_DetectsNodeJSStack(t *testing.T) {
	input := `为什么报这个错？
TypeError: Cannot read property 'foo' of undefined
    at Object.handler (/app/src/routes.js:42:13)
    at Router.handle (/app/node_modules/express/lib/router/index.js:174:12)
    at processTicksAndRejections (node:internal/process/task_queues:95:5)
`
	_, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("Node.js stack should trigger detection")
	}
	if !strings.Contains(detected, "TypeError") {
		t.Errorf("detected missing TypeError header: %q", detected)
	}
}

func TestSplitPastedLog_DetectsRustPanic(t *testing.T) {
	// RUST_BACKTRACE=full output: header + several frames with hex.
	// The RUST_BACKTRACE=1 short form interleaves entries without
	// hex addresses, which is intentionally NOT matched — a generic
	// `\d+:\s+\w+` pattern would false-positive on `grep -n` output.
	input := `thread 'main' panicked at 'index out of bounds: the len is 3 but the index is 5', src/main.rs:42:5
stack backtrace:
   0: 0x5609abcd1000 - std::backtrace_rs::backtrace::...
   1: 0x5609abcd2234 - core::panicking::panic_bounds_check
   2: 0x5609abcd5678 - main::compute
what's wrong?`
	_, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("Rust panic block should trigger detection")
	}
	if !strings.Contains(detected, "panicked") {
		t.Errorf("detected missing panic header: %q", detected)
	}
}

func TestSplitPastedLog_DetectsSyslog(t *testing.T) {
	input := `这段 journal 里有错吗？
Apr 21 14:54:37 host sshd[12345]: Accepted publickey for alice
Apr 21 14:54:38 host sshd[12345]: session opened for user alice
Apr 21 14:54:39 host systemd[1]: Started Session 42 of user alice
`
	_, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("syslog timestamp block should trigger detection")
	}
}

func TestSplitPastedLog_DetectsLowercaseLogLevels(t *testing.T) {
	// zap / slog / logrus typically print lowercase levels.
	input := `zap 打出来是这样：
info  server start addr=:8080 ts=2026-04-21T14:54:37.362Z
debug route register method=GET path=/healthz
warn  deprecated flag used flag=--legacy
`
	_, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("lowercase-level block should trigger detection")
	}
}

func TestSplitPastedLog_DetectsJavaCausedBy(t *testing.T) {
	input := `为什么链式报错？
java.lang.RuntimeException: outer
	at com.example.Outer.run(Outer.java:10)
Caused by: java.sql.SQLException: inner
	at com.example.Inner.query(Inner.java:42)
`
	_, detected := splitPastedLog(input)
	if detected == "" {
		t.Fatalf("Java Caused-by chain should trigger detection")
	}
}

// ── Negative tests: generalization MUST NOT produce false positives
// on code / markdown / normal prose. Each of these has tokens that
// look log-ish in isolation but must not clear the 3-match bar.

func TestSplitPastedLog_DoesNotMatchMarkdownBullets(t *testing.T) {
	input := `帮我 review：
- INFO: the config is loaded from disk
- WARN: legacy flag still ships
- TRACE: we rely on default timezone
这段 review 意见靠谱吗？`
	_, detected := splitPastedLog(input)
	// Three markdown bullets that start with log-level keywords — the
	// prefix `- ` is not in [\s\[]*, so the level pattern is anchored
	// at the `I`/`W`/`T`, after non-level chars. Should NOT match.
	if detected != "" {
		t.Errorf("markdown bullets should not be misread as logs: %q", detected)
	}
}

func TestSplitPastedLog_DoesNotMatchGoSourceCode(t *testing.T) {
	input := `帮我看看这段代码对不对：
func Run() error {
	if err := bootstrap(); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	return nil
}`
	_, detected := splitPastedLog(input)
	if detected != "" {
		t.Errorf("Go source code should not be misread as logs: %q", detected)
	}
}

func TestSplitPastedLog_DoesNotMatchExceptionMentionInProse(t *testing.T) {
	// "NullPointerException" appears mid-sentence, not as a line
	// header. The Exception/Error/Panic pattern is anchored at
	// column 0, so a prose mention must not trigger.
	input := `请问为什么会抛出 NullPointerException 而不是 IllegalArgumentException？
之前 review 过这段代码，但是没看出来。
现在想问一下排查的入口在哪？`
	_, detected := splitPastedLog(input)
	if detected != "" {
		t.Errorf("prose mention of exception names should not trigger: %q", detected)
	}
}
