package hitraceconv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExternalToolInputLeaseStructurePinned(t *testing.T) {
	constructor := sourceGenerationFunctionBody(t, "external_tool_input_lease.go", "createExternalToolInputSnapshot")
	for _, required := range []string{
		"expectedSize := source.Size()",
		"defer func()",
		"completeConversionInputStage(ctx, source, conversionInputStageExternalTool, resultErr)",
		"copyExternalToolInputSnapshot(",
		"writer.Sync()",
		"freezeExternalToolInputSnapshotFile",
	} {
		if !strings.Contains(constructor, required) {
			t.Fatalf("external tool snapshot transaction lost %q:\n%s", required, constructor)
		}
	}
	copyBody := sourceGenerationFunctionBody(t, "external_tool_input_lease.go", "copyExternalToolInputSnapshot")
	for _, required := range []string{"copyCancellableRange(ctx, dst, src", "progress(written, total)"} {
		if !strings.Contains(copyBody, required) {
			t.Fatalf("external tool snapshot copy lost %q:\n%s", required, copyBody)
		}
	}
	command := sourceGenerationFunctionBody(t, "external_tool_input_lease.go", "Command")
	for _, required := range []string{
		`inputArgument = "/proc/self/fd/3"`,
		"exec.CommandContext(ctx, executable, args...)",
		"cmd.ExtraFiles = extraFiles",
		"sameConversionCanonicalPath(argument, lease.source.DisplayPath())",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("external tool command authority lost %q:\n%s", required, command)
		}
	}
	resolver := sourceGenerationFunctionBody(t, "trace_tools.go", "resolveTraceStreamerToolResolution")
	returns := strings.Count(resolver, "return ")
	if snapshotReturns := strings.Count(resolver, "return traceStreamerSnapshotToolResolution("); returns == 0 || snapshotReturns != returns {
		t.Fatalf("trace_streamer resolver no longer routes every terminal arm through the snapshot-only typed helper: returns=%d snapshot_returns=%d\n%s", returns, snapshotReturns, resolver)
	}
	if strings.Contains(resolver, "externalToolInputVerifiedLinuxFD") {
		t.Fatalf("trace_streamer resolver enabled inherited-FD transport without an exact native-tool capability proof:\n%s", resolver)
	}

	linux := sourceGenerationFunctionBody(t, "external_tool_input_lease_linux.go", "tryExternalToolInheritedInputPlatform")
	for _, required := range []string{"externalToolInputVerifiedLinuxFD", "externalToolInputFileSource", "unix.F_DUPFD_CLOEXEC", "linuxExternalToolProcFDUsable"} {
		if !strings.Contains(linux, required) {
			t.Fatalf("Linux exact inherited-FD gate lost %q:\n%s", required, linux)
		}
	}
	proc := sourceGenerationFunctionBody(t, "external_tool_input_lease_linux.go", "linuxExternalToolProcFDUsable")
	for _, required := range []string{"unix.PROC_SUPER_MAGIC", `unix.Readlinkat(procFD, "self"`, `unix.Openat(procFD, "self/fd"`, "unix.Fstatat"} {
		if !strings.Contains(proc, required) {
			t.Fatalf("Linux procfd identity gate lost %q:\n%s", required, proc)
		}
	}

	windowsFreeze := sourceGenerationFunctionBody(t, "external_tool_input_lease_snapshot_windows.go", "freezeExternalToolInputSnapshotFilePlatform")
	if strings.Count(windowsFreeze, "reOpenExternalToolSnapshotWindows(") != 2 {
		t.Fatalf("Windows snapshot lost two-step handle access downgrade:\n%s", windowsFreeze)
	}
	assertSourceGenerationOrder(t, windowsFreeze,
		"bridgeHandle, err := reOpenExternalToolSnapshotWindows",
		"if err := writer.Close()",
		"finalHandle, err := reOpenExternalToolSnapshotWindows",
		"bridgeCloseErr := bridge.Close()",
	)
	for _, forbidden := range []string{"os.Open(", "os.OpenFile(", "windows.CreateFile("} {
		if strings.Contains(windowsFreeze, forbidden) {
			t.Fatalf("Windows snapshot access downgrade regained pathname reopen %q:\n%s", forbidden, windowsFreeze)
		}
	}
	windowsSourceGate := sourceGenerationFunctionBody(t, "external_tool_input_lease_snapshot_windows.go", "validateExternalToolInputSourcePlatform")
	windowsDirGate := sourceGenerationFunctionBody(t, "external_tool_input_lease_snapshot_windows.go", "validateExternalToolInputSnapshotDirPlatform")
	for name, body := range map[string]string{"source": windowsSourceGate, "snapshot": windowsDirGate} {
		if !strings.Contains(body, "validateWindowsExactGenerationFileSystem") {
			t.Fatalf("Windows %s exact-generation gate lost NTFS authority:\n%s", name, body)
		}
	}

	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve external tool lease structure test path")
	}
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	extraFilesWriters := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		extraFilesWriters += strings.Count(string(body), ".ExtraFiles =")
	}
	if extraFilesWriters != 1 {
		t.Fatalf("external-tool ExtraFiles authority is no longer single-point: writers=%d", extraFilesWriters)
	}
}
