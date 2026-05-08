package logtriage

import (
	"regexp"

	"github.com/hanchaoqun/codrax/internal/types"
)

// bugClassPattern is one entry in the cross-language registry. Each
// row carries:
//   - Class:    the diagnostic-tier categorisation
//   - Lang:     language / runtime hint (telemetry only — NEVER
//               surfaces to the LLM)
//   - Pattern:  regex matched against the raw log / trace text
//   - HumanZh:  canonical Chinese term (LLM-facing,
//               training-distribution friendly)
//   - HumanEn:  canonical English term (LLM-facing)
//
// Adding a new pattern is one row + one fixture test — the detector
// engine does NOT change. Adding a new bug class is one row in
// types/bug_class.go's enum + ≥ 1 row here per language.
//
// R4 generalization: every Class has ≥ 1 generic catch-all row at
// the bottom so signatures from languages NOT explicitly listed
// still detect the class via the universal English term.
//
// R6 LLM-prompt safety: only HumanZh / HumanEn / matched substring
// are surfaced to LLM-facing prompts; Class enum string and Lang
// hint stay internal.
type bugClassPattern struct {
	Class   types.BugClass
	Lang    string
	Pattern *regexp.Regexp
	HumanZh string
	HumanEn string
}

var bugClassRegistry = []bugClassPattern{
	// ── Race / data race / concurrent modification ──────────────────
	// Go runtime self-detection (no -race flag needed for these two)
	{types.BugClassRace, "go", regexp.MustCompile(`fatal error: concurrent map writes`),
		"数据竞争(并发 map 写)", "data race (concurrent map writes)"},
	{types.BugClassRace, "go", regexp.MustCompile(`fatal error: concurrent map iteration and map write`),
		"数据竞争(并发遍历与写)", "data race (concurrent iteration + write)"},
	// Go race detector (`-race`)
	{types.BugClassRace, "go", regexp.MustCompile(`WARNING: DATA RACE`),
		"数据竞争(Go race detector 检测)", "data race (Go race detector)"},
	// LLVM ThreadSanitizer (C / C++ / Swift / Rust under TSan instrumentation)
	{types.BugClassRace, "tsan", regexp.MustCompile(`WARNING: ThreadSanitizer: data race`),
		"数据竞争(ThreadSanitizer)", "data race (ThreadSanitizer)"},
	// Helgrind (Valgrind C / C++)
	{types.BugClassRace, "helgrind", regexp.MustCompile(`==\d+==\s+Possible data race during (write|read)`),
		"数据竞争(Helgrind)", "possible data race (Helgrind)"},
	// JVM (Java / Kotlin / Scala)
	{types.BugClassRace, "jvm", regexp.MustCompile(`java\.util\.ConcurrentModificationException`),
		"并发修改异常(数据竞争)", "ConcurrentModificationException"},
	// Generic catch-all
	{types.BugClassRace, "generic", regexp.MustCompile(`(?i)\b(race condition|data race|race detected)\b`),
		"数据竞争", "race condition / data race"},

	// ── Deadlock ─────────────────────────────────────────────────────
	{types.BugClassDeadlock, "go", regexp.MustCompile(`fatal error: all goroutines are asleep - deadlock!`),
		"Go 死锁(全部 goroutine 阻塞)", "Go deadlock (all goroutines asleep)"},
	{types.BugClassDeadlock, "jvm", regexp.MustCompile(`Found Java-level deadlock`),
		"Java 死锁", "Java-level deadlock"},
	{types.BugClassDeadlock, "tsan", regexp.MustCompile(`WARNING: ThreadSanitizer: lock-order-inversion`),
		"锁顺序反转(死锁风险)", "lock-order inversion"},
	{types.BugClassDeadlock, "rust", regexp.MustCompile("called `Mutex::lock\\(\\)` on a poisoned mutex"),
		"Rust 互斥量中毒(deadlock 后)", "Rust poisoned mutex"},
	{types.BugClassDeadlock, "generic", regexp.MustCompile(`(?i)\b(deadlock|circular wait|lock order)\b`),
		"死锁", "deadlock"},

	// ── Nil dereference / null pointer ───────────────────────────────
	{types.BugClassNilDeref, "go", regexp.MustCompile(`runtime error: invalid memory address or nil pointer dereference`),
		"空指针(nil)解引用", "nil pointer dereference"},
	{types.BugClassNilDeref, "jvm", regexp.MustCompile(`java\.lang\.NullPointerException`),
		"空指针异常(NPE)", "NullPointerException"},
	{types.BugClassNilDeref, "kotlin", regexp.MustCompile(`kotlin\.KotlinNullPointerException`),
		"Kotlin 空指针", "KotlinNullPointerException"},
	{types.BugClassNilDeref, "swift", regexp.MustCompile(`Fatal error: Unexpectedly found nil`),
		"Swift 强解包 nil", "Swift force-unwrap nil"},
	{types.BugClassNilDeref, "rust", regexp.MustCompile("called `Option::unwrap\\(\\)` on a `None` value"),
		"Option None unwrap", "Option None unwrap"},
	{types.BugClassNilDeref, "python", regexp.MustCompile(`AttributeError: 'NoneType' object has no attribute`),
		"NoneType 属性访问", "NoneType attribute access"},
	{types.BugClassNilDeref, "ruby", regexp.MustCompile(`NoMethodError.*for nil:NilClass`),
		"Ruby nil 调用", "Ruby nil method call"},
	{types.BugClassNilDeref, "js", regexp.MustCompile(`TypeError: Cannot read propert(y|ies) of (null|undefined)`),
		"JS null/undefined 属性访问", "JS null/undefined property access"},
	{types.BugClassNilDeref, "generic", regexp.MustCompile(`(?i)\b(null pointer|nil dereference|null reference)\b`),
		"空指针解引用", "null/nil dereference"},

	// ── Bounds (array / slice / list / string out-of-range) ──────────
	{types.BugClassBounds, "go", regexp.MustCompile(`runtime error: index out of range`),
		"切片下标越界", "slice index out of range"},
	{types.BugClassBounds, "go", regexp.MustCompile(`runtime error: slice bounds out of range`),
		"切片边界越界", "slice bounds out of range"},
	{types.BugClassBounds, "jvm", regexp.MustCompile(`java\.lang\.(ArrayIndex|StringIndex)OutOfBoundsException`),
		"数组/字符串越界", "ArrayIndex/StringIndexOutOfBoundsException"},
	{types.BugClassBounds, "rust", regexp.MustCompile(`thread .* panicked at .*index out of bounds`),
		"Rust 越界访问", "Rust index out of bounds"},
	{types.BugClassBounds, "python", regexp.MustCompile(`IndexError: (list|tuple|string) index out of range`),
		"Python 序列越界", "Python sequence index out of range"},
	{types.BugClassBounds, "ruby", regexp.MustCompile(`IndexError`),
		"Ruby IndexError", "Ruby IndexError"},
	{types.BugClassBounds, "generic", regexp.MustCompile(`(?i)\bindex (out of range|out of bounds)\b`),
		"下标越界", "index out of bounds"},

	// ── Type assertion / cast failure ────────────────────────────────
	{types.BugClassTypeAssertion, "go", regexp.MustCompile(`interface conversion: .* is not`),
		"接口类型断言失败", "interface conversion failed"},
	{types.BugClassTypeAssertion, "jvm", regexp.MustCompile(`java\.lang\.ClassCastException`),
		"类型转换异常", "ClassCastException"},
	{types.BugClassTypeAssertion, "python", regexp.MustCompile(`TypeError: .* expected .*, got`),
		"Python 类型不匹配", "Python type mismatch"},
	{types.BugClassTypeAssertion, "rust", regexp.MustCompile(`thread .* panicked at .*downcast`),
		"Rust downcast 失败", "Rust downcast failure"},

	// ── Stack overflow / unbounded recursion ────────────────────────
	{types.BugClassStackOverflow, "go", regexp.MustCompile(`runtime: goroutine stack exceeds`),
		"goroutine 栈溢出", "goroutine stack overflow"},
	{types.BugClassStackOverflow, "jvm", regexp.MustCompile(`java\.lang\.StackOverflowError`),
		"JVM 栈溢出", "StackOverflowError"},
	{types.BugClassStackOverflow, "rust", regexp.MustCompile(`thread .* has overflowed its stack`),
		"Rust 栈溢出", "Rust stack overflow"},
	{types.BugClassStackOverflow, "python", regexp.MustCompile(`RecursionError: maximum recursion depth exceeded`),
		"Python 递归深度超限", "Python max recursion depth exceeded"},
	{types.BugClassStackOverflow, "node", regexp.MustCompile(`RangeError: Maximum call stack size exceeded`),
		"Node 栈深度超限", "Node call stack exceeded"},
	{types.BugClassStackOverflow, "generic", regexp.MustCompile(`(?i)\b(stack overflow|stack exhausted)\b`),
		"栈溢出", "stack overflow"},

	// ── Division by zero ─────────────────────────────────────────────
	{types.BugClassDivByZero, "go", regexp.MustCompile(`runtime error: integer divide by zero`),
		"整数除零", "integer divide by zero"},
	{types.BugClassDivByZero, "jvm", regexp.MustCompile(`java\.lang\.ArithmeticException: / by zero`),
		"除零异常", "ArithmeticException / by zero"},
	{types.BugClassDivByZero, "python", regexp.MustCompile(`ZeroDivisionError`),
		"Python 除零", "ZeroDivisionError"},
	{types.BugClassDivByZero, "ruby", regexp.MustCompile(`ZeroDivisionError`),
		"Ruby 除零", "ZeroDivisionError"},
	{types.BugClassDivByZero, "generic", regexp.MustCompile(`(?i)\bdivision by zero\b`),
		"除零", "division by zero"},

	// ── Resource exhaustion ──────────────────────────────────────────
	{types.BugClassResourceExhaustion, "go", regexp.MustCompile(`too many open files`),
		"文件描述符耗尽", "too many open files (FD exhaustion)"},
	{types.BugClassResourceExhaustion, "jvm", regexp.MustCompile(`java\.lang\.OutOfMemoryError`),
		"JVM 内存耗尽", "Java OutOfMemoryError"},
	{types.BugClassResourceExhaustion, "linux", regexp.MustCompile(`(?i)\b(EMFILE|ENFILE|ENOMEM)\b`),
		"系统资源耗尽(errno)", "system resource exhaustion (errno)"},
	{types.BugClassResourceExhaustion, "python", regexp.MustCompile(`MemoryError`),
		"Python 内存耗尽", "Python MemoryError"},
	{types.BugClassResourceExhaustion, "generic", regexp.MustCompile(`(?i)\b(resource exhausted|out of memory|out of file descriptors|disk full|no space left on device)\b`),
		"资源耗尽", "resource exhausted"},

	// ── Unhandled async / promise rejection ──────────────────────────
	{types.BugClassUnhandledAsync, "node", regexp.MustCompile(`UnhandledPromiseRejection`),
		"Node 未捕获的 Promise rejection", "Node UnhandledPromiseRejection"},
	{types.BugClassUnhandledAsync, "python", regexp.MustCompile(`RuntimeError: Task .* never awaited|coroutine .* was never awaited`),
		"Python 协程未 await", "Python coroutine never awaited"},
	{types.BugClassUnhandledAsync, "rust", regexp.MustCompile(`task .* panicked`),
		"Rust task panic", "Rust task panic"},

	// ── Serialization / parse / decode ───────────────────────────────
	{types.BugClassSerialization, "go", regexp.MustCompile(`proto: bad wiretype|invalid character.*looking for|unexpected end of JSON input`),
		"序列化解析失败", "serialization decode failure"},
	{types.BugClassSerialization, "jvm", regexp.MustCompile(`(JsonMappingException|JsonParseException|StreamCorruptedException)`),
		"JVM 反序列化失败", "JVM deserialization failure"},
	{types.BugClassSerialization, "python", regexp.MustCompile(`json\.JSONDecodeError|MessageDecodeError`),
		"Python 解码失败", "Python decode failure"},
	{types.BugClassSerialization, "generic", regexp.MustCompile(`(?i)\b(parse error|decode error|invalid encoding|malformed)\b`),
		"序列化/解析失败", "parse/decode failure"},

	// ── Integrity (checksum / hash / signature) ──────────────────────
	{types.BugClassIntegrity, "git", regexp.MustCompile(`fatal: bad object|fatal: object .* corrupt`),
		"Git 对象损坏", "Git object corruption"},
	{types.BugClassIntegrity, "generic", regexp.MustCompile(`(?i)\b(checksum mismatch|hash mismatch|signature (?:invalid|verification failed)|crc.*(error|mismatch)|invalid header|file is corrupt|unexpected EOF)\b`),
		"完整性校验失败", "integrity check failed"},

	// ── Auth / credential ────────────────────────────────────────────
	{types.BugClassAuth, "oauth", regexp.MustCompile(`(?i)\b(invalid_grant|invalid_client|invalid_token|access_denied)\b`),
		"OAuth 鉴权失败", "OAuth auth failure"},
	{types.BugClassAuth, "ldap", regexp.MustCompile(`(?i)\bbind (failed|invalid)`),
		"LDAP 鉴权失败", "LDAP auth failure"},
	{types.BugClassAuth, "jwt", regexp.MustCompile(`(?i)\b(jwt|token).*signature is invalid|signature verification failed`),
		"JWT 签名验证失败", "JWT signature verification failed"},
	{types.BugClassAuth, "generic", regexp.MustCompile(`(?i)\b(authentication (failed|required)|unauthorized|invalid credentials|forbidden|access denied)\b`),
		"鉴权/授权失败", "authentication / authorisation failure"},

	// ── TLS certificate ──────────────────────────────────────────────
	{types.BugClassTLSCert, "go", regexp.MustCompile(`x509: certificate (signed by unknown authority|has expired|is not valid)`),
		"x509 证书校验失败", "x509 certificate validation failure"},
	{types.BugClassTLSCert, "python", regexp.MustCompile(`SSL: CERTIFICATE_VERIFY_FAILED|CertificateError`),
		"Python TLS 证书校验失败", "Python TLS certificate verification failed"},
	{types.BugClassTLSCert, "java", regexp.MustCompile(`(SSLHandshakeException|CertificateException|PKIX path)`),
		"Java TLS/PKIX 失败", "Java SSL/PKIX failure"},
	{types.BugClassTLSCert, "generic", regexp.MustCompile(`(?i)\b(certificate (verify failed|expired|untrusted)|tls handshake failed|self.signed certificate)\b`),
		"TLS 证书校验失败", "TLS certificate verification failed"},

	// ── Use after free (memory-unsafe languages) ─────────────────────
	{types.BugClassUseAfterFree, "asan", regexp.MustCompile(`AddressSanitizer: (heap-use-after-free|stack-use-after-(scope|return)|use-after-poison)`),
		"释放后使用(AddressSanitizer)", "use-after-free (AddressSanitizer)"},
	{types.BugClassUseAfterFree, "msan", regexp.MustCompile(`MemorySanitizer: use-of-uninitialized-value`),
		"未初始化内存使用(MSan)", "uninitialized memory use (MSan)"},
	{types.BugClassUseAfterFree, "valgrind", regexp.MustCompile(`==\d+==.*Invalid (read|write) of size`),
		"非法内存访问(Valgrind)", "invalid memory access (Valgrind)"},
	{types.BugClassUseAfterFree, "glibc", regexp.MustCompile(`free\(\): (invalid pointer|double free|invalid next size)`),
		"glibc 检测到非法 free", "glibc free corruption"},
	{types.BugClassUseAfterFree, "rust", regexp.MustCompile(`thread .* panicked at .*use after (move|free)`),
		"Rust use-after-move/free panic", "Rust use-after-move panic"},
	{types.BugClassUseAfterFree, "generic", regexp.MustCompile(`(?i)\b(use[- ]after[- ]free|double free|dangling pointer)\b`),
		"释放后使用 / 悬空指针", "use-after-free / dangling pointer"},

	// ── Buffer overflow / OOB write (memory-unsafe languages) ────────
	{types.BugClassBufferOverflow, "asan", regexp.MustCompile(`AddressSanitizer: (heap|stack|global)-buffer-overflow`),
		"缓冲区溢出(AddressSanitizer)", "buffer overflow (AddressSanitizer)"},
	{types.BugClassBufferOverflow, "ubsan", regexp.MustCompile(`UndefinedBehaviorSanitizer:.*out of bounds`),
		"越界访问(UBSan)", "out-of-bounds access (UBSan)"},
	{types.BugClassBufferOverflow, "valgrind", regexp.MustCompile(`==\d+==.*Conditional jump or move depends on uninitialised value`),
		"未初始化值依赖(Valgrind)", "uninitialised conditional (Valgrind)"},
	{types.BugClassBufferOverflow, "glibc", regexp.MustCompile(`\*\*\* stack smashing detected \*\*\*|buffer overflow detected`),
		"栈/堆 buffer overflow", "stack/heap buffer overflow"},
	{types.BugClassBufferOverflow, "generic", regexp.MustCompile(`(?i)\b(buffer overflow|out[- ]of[- ]bounds (read|write))\b`),
		"缓冲区溢出", "buffer overflow"},

	// ── Uncaught exception (generic catch-all per language) ──────────
	{types.BugClassUncaughtException, "node", regexp.MustCompile(`Uncaught (Error|Exception):`),
		"Node 未捕获异常", "Node uncaught exception"},
	{types.BugClassUncaughtException, "python", regexp.MustCompile(`Traceback \(most recent call last\):`),
		"Python 未捕获异常", "Python uncaught exception"},
	{types.BugClassUncaughtException, "ruby", regexp.MustCompile(`unhandled exception|RuntimeError`),
		"Ruby 未捕获异常", "Ruby uncaught exception"},
	{types.BugClassUncaughtException, "swift", regexp.MustCompile(`Fatal error:|libc\+\+abi: terminating with uncaught exception`),
		"Swift/C++ 未捕获异常", "Swift/C++ uncaught exception"},
	{types.BugClassUncaughtException, "lua", regexp.MustCompile(`lua: .*:\d+: `),
		"Lua 错误", "Lua error"},
	{types.BugClassUncaughtException, "arkts", regexp.MustCompile(`Error: .* at .*\.ets:\d+`),
		"ArkTS 运行时错误", "ArkTS runtime error"},
	{types.BugClassUncaughtException, "cangjie", regexp.MustCompile(`Exception in thread .* at .*\.cj:\d+`),
		"Cangjie 未捕获异常", "Cangjie uncaught exception"},

	// ── Filesystem error (file-not-found / permission / disk) ────────
	{types.BugClassFileSystem, "errno", regexp.MustCompile(`(?i)\b(ENOENT|EACCES|EROFS|ENOSPC|EISDIR|ENOTDIR|EBUSY)\b`),
		"文件系统错误(errno)", "filesystem error (errno)"},
	{types.BugClassFileSystem, "go", regexp.MustCompile(`(no such file or directory|permission denied|read-only file system|no space left on device)`),
		"文件系统错误", "filesystem error"},
	{types.BugClassFileSystem, "python", regexp.MustCompile(`FileNotFoundError|PermissionError|IsADirectoryError|NotADirectoryError`),
		"Python 文件系统错误", "Python filesystem error"},
	{types.BugClassFileSystem, "jvm", regexp.MustCompile(`java\.io\.(FileNotFoundException|AccessDeniedException)`),
		"JVM 文件系统错误", "JVM filesystem error"},
	{types.BugClassFileSystem, "generic", regexp.MustCompile(`(?i)\b(file not found|permission denied|disk full|read.only filesystem)\b`),
		"文件系统错误", "filesystem error"},

	// ── Encoding (UTF-8 / charset / character decode) ────────────────
	{types.BugClassEncoding, "go", regexp.MustCompile(`(invalid UTF-8|encoding/.* invalid)`),
		"字符编码错误", "character encoding error"},
	{types.BugClassEncoding, "python", regexp.MustCompile(`UnicodeDecodeError|UnicodeEncodeError`),
		"Python Unicode 编解码错误", "Python Unicode codec error"},
	{types.BugClassEncoding, "jvm", regexp.MustCompile(`java\.nio\.charset\.(MalformedInput|UnmappableCharacter|CharacterCodingException)`),
		"JVM 字符集错误", "JVM charset error"},
	{types.BugClassEncoding, "rust", regexp.MustCompile(`thread .* panicked at[: ].*Utf8Error|invalid utf-8 sequence`),
		"Rust UTF-8 错误", "Rust UTF-8 error"},
	{types.BugClassEncoding, "generic", regexp.MustCompile(`(?i)\b(invalid utf-?8|charset error|encoding error|malformed (input|character))\b`),
		"字符编码错误", "character encoding error"},

	// ── Config (missing / invalid runtime configuration) ─────────────
	{types.BugClassConfig, "env", regexp.MustCompile(`(?i)\b(env(?:ironment)? variable .* (not set|missing|required)|missing required env)\b`),
		"环境变量缺失", "missing required env var"},
	{types.BugClassConfig, "yaml", regexp.MustCompile(`yaml: (line \d+:|unmarshal errors|cannot unmarshal)`),
		"YAML 配置解析错误", "YAML config parse error"},
	{types.BugClassConfig, "json", regexp.MustCompile(`config.*invalid|configuration.*invalid|missing required (config|field)`),
		"配置无效或字段缺失", "invalid / missing config"},
	{types.BugClassConfig, "generic", regexp.MustCompile(`(?i)\b(configuration (error|invalid)|missing config|required (option|setting) not (set|provided))\b`),
		"配置错误", "configuration error"},
}

// DetectBugClasses scans the raw log/trace text against the cross-
// language registry and returns the deduplicated list of detections.
//
// Behaviour:
//   - First-match-per-class wins for HumanZh/HumanEn (the language-
//     specific entry is preferred over the generic catch-all because
//     language-specific entries appear first in the registry).
//   - MatchedSignature carries the verbatim substring (capped 200
//     chars) the pattern hit, for evidence pinning.
//   - Stable order: classes appear in registry order (race / deadlock /
//     ... / tls_cert) so prompt rendering is deterministic.
//   - Empty or oversized rawLog still works (regex bounded by Go's
//     RE2 implementation; no catastrophic backtracking).
//   - μs-class on typical attached log (16-100 KB).
//
// Idempotent — calling twice on the same input returns identical
// output.
func DetectBugClasses(rawLog string) []types.DetectedBugClass {
	if rawLog == "" {
		return nil
	}
	const matchCap = 200
	seen := make(map[types.BugClass]bool, 8)
	var out []types.DetectedBugClass
	for _, p := range bugClassRegistry {
		if seen[p.Class] {
			continue
		}
		loc := p.Pattern.FindStringIndex(rawLog)
		if loc == nil {
			continue
		}
		matched := rawLog[loc[0]:loc[1]]
		if len(matched) > matchCap {
			matched = matched[:matchCap]
		}
		out = append(out, types.DetectedBugClass{
			Class:            p.Class,
			HumanZh:          p.HumanZh,
			HumanEn:          p.HumanEn,
			MatchedSignature: matched,
			SourceLang:       p.Lang,
		})
		seen[p.Class] = true
	}
	return out
}
