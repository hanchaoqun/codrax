package hitraceconv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExternalToolCommandSingleAuthorityStructurePinned(t *testing.T) {
	constructor := sourceGenerationFunctionBody(t, "external_tool_command.go", "newExternalToolCommand")
	for _, required := range []string{
		"exec.CommandContext(waitCtx, executable, args...)",
		"cmd.Cancel = nil",
		"cmd.WaitDelay = externalToolCommandWaitDelay",
		"newExternalToolProcessSupervisor(cmd)",
	} {
		if !strings.Contains(constructor, required) {
			t.Fatalf("external-tool command constructor lost %q:\n%s", required, constructor)
		}
	}
	runner := sourceGenerationFunctionBody(t, "external_tool_command.go", "runExternalToolCommandUntilExit")
	for _, required := range []string{
		"command.claim()",
		"command.cmd.Start()",
		"command.supervisor.afterStart(command.cmd)",
		"command.supervisor.terminate()",
		"command.cmd.Wait()",
		"command.ctx.Done()",
	} {
		if !strings.Contains(runner, required) {
			t.Fatalf("external-tool singleton runner lost %q:\n%s", required, runner)
		}
	}

	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve external-tool structure-test path")
	}
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	commandContextSites := 0
	rawCommandSites := 0
	startSites := 0
	waitSites := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		commandContextSites += strings.Count(text, "exec.CommandContext(")
		rawCommandSites += strings.Count(text, "exec.Command(")
		startSites += strings.Count(text, ".Start()")
		waitSites += strings.Count(text, ".Wait()")
	}
	if commandContextSites != 1 || rawCommandSites != 0 || startSites != 1 || waitSites != 2 {
		t.Fatalf("external-tool execution authority drifted: command_context=%d raw_command=%d start=%d wait=%d",
			commandContextSites, rawCommandSites, startSites, waitSites)
	}

	for _, provider := range []struct{ file, function string }{
		{file: "trace_streamer_provider.go", function: "runTraceStreamerExport"},
		{file: "simpleperf_text.go", function: "maybeConvertSimpleperfPerfData"},
		{file: "hiperf_proto.go", function: "maybeConvertHiperfPerfDataFromInput"},
		{file: "embedded_trace_streamer_runtime.go", function: "probeEmbeddedTraceStreamerRuntime"},
	} {
		body := sourceGenerationFunctionBody(t, provider.file, provider.function)
		if !strings.Contains(body, "runCommandWithProgressUntilExit(") {
			t.Fatalf("%s regained a process-supervisor bypass:\n%s", provider.function, body)
		}
	}

	unix := sourceGenerationFunctionBody(t, "external_tool_command_unix.go", "newExternalToolProcessSupervisor")
	if !strings.Contains(unix, "cmd.SysProcAttr.Setpgid = true") {
		t.Fatalf("Unix external-tool process-group isolation drifted:\n%s", unix)
	}
	windows := sourceGenerationFunctionBody(t, "external_tool_command_windows.go", "newExternalToolProcessSupervisor")
	for _, required := range []string{
		"windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE",
		"cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED",
	} {
		if !strings.Contains(windows, required) {
			t.Fatalf("Windows external-tool Job authority lost %q:\n%s", required, windows)
		}
	}
}
